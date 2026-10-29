package ar

import (
	"context"
	"fmt"
	"reflect"

	"gorm.io/gorm"
	"gorm.io/gorm/schema"

	"github.com/goyii2/framework/validators"
)

// Model gives Active Record model structs their instance persistence
// methods. Models embed it; it contributes no columns (both fields are
// unexported and therefore invisible to GORM):
//
//	type User struct {
//	    ar.Model
//	    ID   int64  `gorm:"column:id;primaryKey"`
//	    Name string `gorm:"column:name"`
//	}
//
// Instances become bound — able to Save, Delete, Reload and Validate —
// through the package entry points that hand them out: New[T](ctx).New() for
// a fresh record, FindByID and query results for existing rows. The bound
// context is the one the record was created with, so instances obtained
// inside a Transaction closure keep participating in that transaction
// afterwards.
type Model struct {
	ctx    context.Context
	record any
}

// bind attaches the record to ctx. It is promoted into every *T whose T
// embeds Model, which is what query results and New() detect through the
// binder interface — no reflection on the model type itself.
func (m *Model) bind(ctx context.Context, record any) {
	m.ctx, m.record = ctx, record
}

// binder matches any record whose type embeds Model.
type binder interface {
	bind(context.Context, any)
}

// bindModels binds every record in records that embeds Model to ctx.
func bindModels[T any](ctx context.Context, records ...*T) {
	for _, record := range records {
		if b, ok := any(record).(binder); ok {
			b.bind(ctx, record)
		}
	}
}

// handle resolves the bound context to a GORM handle. A model that never
// went through this package has no context and therefore no handle.
func (m *Model) handle() (*gorm.DB, error) {
	if m.ctx == nil {
		return nil, fmt.Errorf("ar: model %T is not bound to a context; obtain records via ar.New[T](ctx).New(), ar.FindByID or a query result", m.record)
	}
	return session(m.ctx)
}

// Context returns the context the record is bound to — the one it was
// created or loaded with, carrying the application and any open transaction.
// Validation rules that need the database (a uniqueness check, say) use it
// to query through this package instead of reaching for a global handle.
func (m *Model) Context() context.Context { return m.ctx }

// Validate checks the record against the schemas its Rules method declares
// for scenario and returns the per-field message list; nil means the record
// is valid, as does a model without Rules or a scenario without a schema.
// Messages translate into the language of the context the record is bound
// to.
func (m *Model) Validate(scenario validators.Scenario) map[string][]string {
	if err := validators.Validate(m.ctx, m.record, scenario); err != nil {
		return err.Fields
	}
	return nil
}

// Save writes the record back to the database: an insert when its primary
// key is zero, an update of the full row otherwise. When the model declares
// validation rules, the matching scenario is validated first and an invalid
// record is refused before any statement runs. Lifecycle hooks the model
// implements (BeforeSave, BeforeCreate, ...) run as part of the write.
func (m *Model) Save() error {
	handle, err := m.handle()
	if err != nil {
		return err
	}
	if err := m.validateBeforeSave(handle); err != nil {
		return err
	}
	if err := handle.Save(m.record).Error; err != nil {
		return fmt.Errorf("ar: save %T: %w", m.record, err)
	}
	return nil
}

// validateBeforeSave runs the model's rules for the scenario this save
// constitutes: insert when the primary key is zero, update otherwise — the
// same split the write itself applies. Models without Rules skip validation
// entirely, and a RuleSet without a schema for the scenario passes.
func (m *Model) validateBeforeSave(handle *gorm.DB) error {
	if _, ok := m.record.(validators.Ruled); !ok {
		return nil
	}
	scenario, err := saveScenario(handle, m.record)
	if err != nil {
		return err
	}
	// The verdict must reach Save as a plain nil on success: returning the
	// *Errors value directly would wrap a nil pointer in a non-nil error.
	if verrs := validators.Validate(m.ctx, m.record, scenario); verrs != nil {
		return verrs
	}
	return nil
}

// saveScenario reports whether the upcoming save of record is an insert or
// an update, judged by the primary key the same way the write is.
func saveScenario(handle *gorm.DB, record any) (validators.Scenario, error) {
	field, err := primaryKey(handle, record)
	if err != nil {
		return "", err
	}
	_, zero := field.ValueOf(context.Background(), structValue(record))
	if zero {
		return validators.ScenarioInsert, nil
	}
	return validators.ScenarioUpdate, nil
}

// Delete removes the row carrying the record's primary key, running the
// model's BeforeDelete and AfterDelete hooks around the statement.
func (m *Model) Delete() error {
	handle, err := m.handle()
	if err != nil {
		return err
	}
	if err := handle.Delete(m.record).Error; err != nil {
		return fmt.Errorf("ar: delete %T: %w", m.record, err)
	}
	return nil
}

// Reload refreshes the record's fields from the database row with the same
// primary key, discarding any in-memory modification.
func (m *Model) Reload() error {
	handle, err := m.handle()
	if err != nil {
		return err
	}
	field, err := primaryKey(handle, m.record)
	if err != nil {
		return err
	}
	value, zero := field.ValueOf(context.Background(), structValue(m.record))
	if zero {
		return fmt.Errorf("ar: reload %T: the primary key is not set; the record was never saved or loaded", m.record)
	}
	if err := handle.Where(field.DBName+" = ?", value).Take(m.record).Error; err != nil {
		return fmt.Errorf("ar: reload %T: %w", m.record, err)
	}
	return nil
}

// structValue dereferences record down to its struct value, the form GORM
// schema field accessors expect.
func structValue(record any) reflect.Value {
	v := reflect.ValueOf(record)
	for v.Kind() == reflect.Pointer {
		v = v.Elem()
	}
	return v
}

// primaryKey parses the record's schema and returns its single primary key
// field. Records without a primary key, with a composite one, or whose key
// maps to no database column are rejected — those cases need explicit Where
// conditions instead of the convenience helpers.
func primaryKey(handle *gorm.DB, record any) (*schema.Field, error) {
	stmt := &gorm.Statement{DB: handle}
	if err := stmt.Parse(record); err != nil {
		return nil, fmt.Errorf("ar: parse schema of %T: %w", record, err)
	}
	field := stmt.Schema.PrioritizedPrimaryField
	if field == nil || len(stmt.Schema.PrimaryFields) > 1 || field.DBName == "" {
		return nil, fmt.Errorf("ar: %T needs exactly one primary key column for this operation", record)
	}
	return field, nil
}
