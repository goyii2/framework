package behaviors

import (
	"errors"
	"fmt"
	"reflect"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ErrStaleVersion marks a save refused because the row's version moved on
// since the record was loaded — the lost-update the optimistic lock
// exists to catch. Distinguish it with errors.Is:
//
//	if err := doc.Save(); errors.Is(err, behaviors.ErrStaleVersion) {
//	    // reload, reapply, save again
//	}
var ErrStaleVersion = errors.New("behaviors: optimistic lock conflict, the row was changed by someone else")

// OptimisticLock guards the embedding record against lost updates through
// a version column — the counterpart of Yii2's optimistic locking. A new
// row starts at version 1. Every update increments the version and runs
// against a WHERE version = <loaded version>; when the row was changed in
// between, the statement matches nothing, the save fails with
// ErrStaleVersion and the record keeps the version it was loaded with.
//
// The guard covers the save path (update of a loaded record). Deletes run
// unconditioned: removing a row someone else changed is not a lost update.
type OptimisticLock struct {
	Version int64 `gorm:"column:version"`

	// loaded holds the version the record was loaded or last saved with —
	// the expected value of the next update and the value restored on
	// conflict. Unexported, so it maps to no column.
	loaded int64
}

// onCreate starts new rows at version 1 so the first update has a concrete
// previous value to expect. A caller seeding version explicitly keeps it.
func (o *OptimisticLock) onCreate(model reflect.Value, tx *gorm.DB) error {
	if o.Version == 0 {
		o.Version = 1
	}
	return nil
}

// onUpdate adds the version condition to the statement and moves the
// record to the next version, which the update writes.
func (o *OptimisticLock) onUpdate(model reflect.Value, tx *gorm.DB) error {
	if tx.Statement == nil || tx.Statement.Schema == nil {
		return fmt.Errorf("behaviors: OptimisticLock needs a parsed schema; save through the record's own methods")
	}
	field := tx.Statement.Schema.LookUpField("Version")
	if field == nil || field.DBName == "" {
		return fmt.Errorf("behaviors: OptimisticLock needs its version column mapped on %s", model.Type())
	}
	previous := o.Version
	o.Version = previous + 1
	o.loaded = previous
	tx.Statement.AddClause(clause.Where{Exprs: []clause.Expression{
		clause.Eq{Column: clause.Column{Name: field.DBName}, Value: previous},
	}})
	return nil
}

// onUpdated inspects the executed update: zero rows affected means the
// expected version was gone, a conflict. The record rolls back to its
// loaded version so a reload-and-retry cycle starts from a truthful state.
func (o *OptimisticLock) onUpdated(model reflect.Value, tx *gorm.DB) error {
	if rowsAffected(tx) != 0 {
		return nil
	}
	o.Version = o.loaded
	return fmt.Errorf("behaviors: %s stale at version %d: %w", model.Type(), o.loaded, ErrStaleVersion)
}

// rowsAffected reports how many rows the statement behind tx touched. GORM
// hands hooks a fresh session whose own RowsAffected is always zero; the
// live count sits on the database instance that owns the shared statement.
func rowsAffected(tx *gorm.DB) int64 {
	if tx.Statement != nil && tx.Statement.DB != nil {
		return tx.Statement.DB.RowsAffected
	}
	return tx.RowsAffected
}
