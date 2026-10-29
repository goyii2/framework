package behaviors

import (
	"fmt"
	"reflect"

	"gorm.io/gorm"
)

// Hooks is the wiring between embedded behaviors and GORM's lifecycle: the
// model embeds it once, next to the behavior structs, and GORM's
// BeforeCreate / BeforeUpdate / AfterUpdate callbacks reach the behaviors
// through it. It declares no fields and therefore no columns.
//
// Embedding it is what makes the behaviors run. Go cannot promote two
// same-named hook methods from two embedded structs — the promotion is
// ambiguous and GORM would call neither — so the behaviors themselves
// define no GORM hooks and Hooks is the only promoted implementation. It
// dispatches by walking the model's embedded fields and invoking each
// behavior's internal fill method for the event at hand.
//
// A model that declares its own BeforeCreate or BeforeUpdate shadows the
// promoted methods (the shallower definition wins) and takes over the
// wiring; such a hook keeps its behaviors alive by delegating to
// DispatchCreate, DispatchUpdate or DispatchUpdated.
type Hooks struct{}

// createFiller matches behaviors that fill the record on the insert path.
type createFiller interface {
	onCreate(model reflect.Value, tx *gorm.DB) error
}

// updateFiller matches behaviors that act before the update statement runs.
type updateFiller interface {
	onUpdate(model reflect.Value, tx *gorm.DB) error
}

// updatedChecker matches behaviors that inspect the outcome after the
// update statement ran (rows affected, conflicts).
type updatedChecker interface {
	onUpdated(model reflect.Value, tx *gorm.DB) error
}

// BeforeCreate fills the record for insertion: every embedded behavior
// gets its create turn, in field declaration order.
func (Hooks) BeforeCreate(tx *gorm.DB) error {
	if tx.Statement == nil || tx.Statement.ReflectValue.Kind() != reflect.Struct {
		return nil
	}
	return dispatch(tx.Statement.ReflectValue, tx, func(b any) error {
		if filler, ok := b.(createFiller); ok {
			return filler.onCreate(tx.Statement.ReflectValue, tx)
		}
		return nil
	})
}

// BeforeUpdate prepares the record for the update statement: every
// embedded behavior gets its update turn (refresh marks, add lock
// conditions) before the SQL is built.
func (Hooks) BeforeUpdate(tx *gorm.DB) error {
	if tx.Statement == nil || tx.Statement.ReflectValue.Kind() != reflect.Struct {
		return nil
	}
	return dispatch(tx.Statement.ReflectValue, tx, func(b any) error {
		if filler, ok := b.(updateFiller); ok {
			return filler.onUpdate(tx.Statement.ReflectValue, tx)
		}
		return nil
	})
}

// AfterUpdate lets behaviors inspect the finished update — the optimistic
// lock turns zero rows affected into a conflict error here.
func (Hooks) AfterUpdate(tx *gorm.DB) error {
	if tx.Statement == nil || tx.Statement.ReflectValue.Kind() != reflect.Struct {
		return nil
	}
	return dispatch(tx.Statement.ReflectValue, tx, func(b any) error {
		if checker, ok := b.(updatedChecker); ok {
			return checker.onUpdated(tx.Statement.ReflectValue, tx)
		}
		return nil
	})
}

// DispatchCreate runs the create-phase fills of every behavior embedded in
// model. Models whose own BeforeCreate shadows the Hooks method delegate
// here to keep their behaviors running.
func DispatchCreate(model any, tx *gorm.DB) error {
	v, err := modelValue(model)
	if err != nil {
		return err
	}
	return dispatch(v, tx, func(b any) error {
		if filler, ok := b.(createFiller); ok {
			return filler.onCreate(v, tx)
		}
		return nil
	})
}

// DispatchUpdate runs the update-phase fills of every behavior embedded in
// model. Models whose own BeforeUpdate shadows the Hooks method delegate
// here to keep their behaviors running.
func DispatchUpdate(model any, tx *gorm.DB) error {
	v, err := modelValue(model)
	if err != nil {
		return err
	}
	return dispatch(v, tx, func(b any) error {
		if filler, ok := b.(updateFiller); ok {
			return filler.onUpdate(v, tx)
		}
		return nil
	})
}

// DispatchUpdated runs the post-update checks of every behavior embedded in
// model. Models whose own AfterUpdate shadows the Hooks method delegate
// here to keep their behaviors running.
func DispatchUpdated(model any, tx *gorm.DB) error {
	v, err := modelValue(model)
	if err != nil {
		return err
	}
	return dispatch(v, tx, func(b any) error {
		if checker, ok := b.(updatedChecker); ok {
			return checker.onUpdated(v, tx)
		}
		return nil
	})
}

// modelValue normalizes model to an addressable struct value — the form the
// behaviors receive as their model argument. A non-addressable value cannot
// carry fills, so it is an error instead of a silent skip.
func modelValue(model any) (reflect.Value, error) {
	v := reflectStruct(model)
	if v.Kind() != reflect.Struct || !v.CanAddr() {
		return reflect.Value{}, fmt.Errorf("behaviors: dispatch needs a pointer to a struct model, got %T", model)
	}
	return v, nil
}

// reflectStruct dereferences model down to its struct value.
func reflectStruct(model any) reflect.Value {
	v := reflect.ValueOf(model)
	for v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface {
		v = v.Elem()
	}
	return v
}

// dispatch walks the embedded fields of the model struct and calls call on
// every behavior it finds. The walk follows anonymous (embedded) fields,
// so a shared base struct carrying behaviors works too; pointer-valued
// embedded fields dispatch only when non-nil. Behaviors hold the record's
// own state, so every field on the path must be addressable — the record
// itself decides that, and a non-addressable one is an error rather than a
// silent skip.
func dispatch(model reflect.Value, tx *gorm.DB, call func(any) error) error {
	return walk(model, map[uintptr]struct{}{}, call)
}

// walk visits every reachable embedded field, depth-first in declaration
// order, calling call with each field as any — value fields as their
// address (*T), embedded pointers as the pointer itself, which is where
// the behavior interfaces live either way. Pointer-valued embedded fields
// dispatch only when non-nil, with a cycle guard on pointer identity.
func walk(v reflect.Value, visited map[uintptr]struct{}, call func(any) error) error {
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		if !t.Field(i).Anonymous {
			// Named (non-embedded) fields are the record's own composition
			// detail, not behaviors; only promoted positions dispatch,
			// matching how GORM and Go method promotion work.
			continue
		}
		field := v.Field(i)
		if !field.CanAddr() {
			return fmt.Errorf("behaviors: model %s is not addressable; pass a pointer to the record", t)
		}

		var target reflect.Value
		if field.Kind() == reflect.Pointer {
			if field.IsNil() {
				continue
			}
			ptr := field.Pointer()
			if _, seen := visited[ptr]; seen {
				continue
			}
			visited[ptr] = struct{}{}
			target = field
		} else {
			target = field.Addr()
		}

		// An unexported embedded type (type model struct { base }, base
		// lowercase) cannot be handed out through Interface, but its
		// exported fields are fully reachable — the recursion below
		// dispatches the behaviors it carries.
		if target.CanInterface() {
			if err := call(target.Interface()); err != nil {
				return err
			}
		}
		switch field.Kind() {
		case reflect.Struct:
			if err := walk(field, visited, call); err != nil {
				return err
			}
		case reflect.Pointer:
			if err := walk(field.Elem(), visited, call); err != nil {
				return err
			}
		}
	}
	return nil
}
