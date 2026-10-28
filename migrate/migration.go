package migrate

import (
	"context"
	"errors"
)

// DefaultMigrationTable is the default table used to record applied migrations.
const DefaultMigrationTable = "migration"

// ErrIrreversibleMigration is returned by migrations that cannot be reverted.
var ErrIrreversibleMigration = errors.New("irreversible migration")

// ErrMigrationNotFound is returned when an applied migration has no registered implementation.
var ErrMigrationNotFound = errors.New("migration not found")

// ErrMigrationLockTimeout is returned when the migration lock cannot be acquired.
var ErrMigrationLockTimeout = errors.New("migration lock timeout")

// Migration is implemented by concrete database migrations.
type Migration interface {
	// Name returns the unique migration version name.
	Name() string
	// Up applies the migration.
	Up(ctx context.Context, m *MigrationContext) error
	// Down reverts the migration.
	Down(ctx context.Context, m *MigrationContext) error
}

// AppliedMigration describes a migration recorded in the migration table.
type AppliedMigration struct {
	// Version is the migration version name.
	Version string
	// ApplyTime is the Unix timestamp recorded when the migration was applied.
	ApplyTime int64
}
