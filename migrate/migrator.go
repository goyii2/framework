package migrate

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"time"
)

// Migrator applies, reverts, and inspects registered migrations.
type Migrator struct {
	db         *sql.DB
	dialect    Dialect
	migrations []Migration
	now        func() time.Time
	logger     Logger

	// MigrationTable is the table used to record applied migrations.
	MigrationTable string
	// DryRun prints migration SQL without executing it.
	DryRun bool
	// UseLock controls whether mutating migration commands acquire a database lock.
	UseLock bool
	// LockName is the advisory lock name used for migration commands.
	LockName string
	// LockTimeoutSeconds is the number of seconds to wait for the migration lock.
	LockTimeoutSeconds int
}

// NewMigrator creates a migrator for the provided database and migrations.
func NewMigrator(db *sql.DB, dialect Dialect, migrations []Migration) *Migrator {
	if dialect == nil {
		dialect = MySQLDialect{}
	}
	copied := append([]Migration(nil), migrations...)
	sort.Slice(copied, func(i, j int) bool {
		return copied[i].Name() < copied[j].Name()
	})
	return &Migrator{
		db:                 db,
		dialect:            dialect,
		migrations:         copied,
		now:                time.Now,
		logger:             defaultLogger(),
		MigrationTable:     DefaultMigrationTable,
		UseLock:            true,
		LockName:           "yiimigrate",
		LockTimeoutSeconds: 30,
	}
}

// EnsureMigrationTable creates the migration table when it does not exist.
func (m *Migrator) EnsureMigrationTable(ctx context.Context) error {
	statement, err := m.dialect.CreateMigrationTableSQL(m.migrationTable())
	if err != nil {
		return err
	}
	if m.DryRun {
		m.logSQL(statement)
		return nil
	}
	_, err = m.db.ExecContext(ctx, statement)
	return err
}

// Applied returns applied migrations ordered by apply time from oldest to newest.
func (m *Migrator) Applied(ctx context.Context) ([]AppliedMigration, error) {
	if m.DryRun {
		return nil, nil
	}
	if err := m.EnsureMigrationTable(ctx); err != nil {
		return nil, err
	}
	return m.loadApplied(ctx, false)
}

// Pending returns registered migrations that have not been applied.
func (m *Migrator) Pending(ctx context.Context) ([]Migration, error) {
	applied, err := m.Applied(ctx)
	if err != nil {
		return nil, err
	}
	appliedSet := make(map[string]struct{}, len(applied))
	for _, migration := range applied {
		appliedSet[migration.Version] = struct{}{}
	}

	pending := make([]Migration, 0, len(m.migrations))
	for _, migration := range m.migrations {
		if _, ok := appliedSet[migration.Name()]; !ok {
			pending = append(pending, migration)
		}
	}
	return pending, nil
}

// History returns applied migrations ordered from newest to oldest.
func (m *Migrator) History(ctx context.Context, limit int) ([]AppliedMigration, error) {
	if err := m.EnsureMigrationTable(ctx); err != nil {
		return nil, err
	}
	applied, err := m.loadApplied(ctx, true)
	if err != nil {
		return nil, err
	}
	return limitApplied(applied, limit), nil
}

// Up applies pending migrations. A limit of zero or less applies all pending migrations.
func (m *Migrator) Up(ctx context.Context, limit int) ([]string, error) {
	var applied []string
	err := m.withLock(ctx, func() error {
		var err error
		applied, err = m.up(ctx, limit)
		return err
	})
	return applied, err
}

// New returns pending migrations. A limit of zero or less returns all pending migrations.
func (m *Migrator) New(ctx context.Context, limit int) ([]Migration, error) {
	pending, err := m.Pending(ctx)
	if err != nil {
		return nil, err
	}
	return limitMigrations(pending, limit), nil
}

func (m *Migrator) up(ctx context.Context, limit int) ([]string, error) {
	pending, err := m.Pending(ctx)
	if err != nil {
		return nil, err
	}
	pending = limitMigrations(pending, limit)

	applied := make([]string, 0, len(pending))
	for _, migration := range pending {
		if err := m.apply(ctx, migration); err != nil {
			return applied, err
		}
		applied = append(applied, migration.Name())
	}
	return applied, nil
}

// Down reverts applied migrations from newest to oldest. A limit of zero or less reverts all applied migrations.
func (m *Migrator) Down(ctx context.Context, limit int) ([]string, error) {
	var reverted []string
	err := m.withLock(ctx, func() error {
		var err error
		reverted, err = m.down(ctx, limit)
		return err
	})
	return reverted, err
}

func (m *Migrator) down(ctx context.Context, limit int) ([]string, error) {
	history, err := m.History(ctx, limit)
	if err != nil {
		return nil, err
	}
	registered := m.registeredByName()

	reverted := make([]string, 0, len(history))
	for _, applied := range history {
		migration, ok := registered[applied.Version]
		if !ok {
			return reverted, fmt.Errorf("%w: %s", ErrMigrationNotFound, applied.Version)
		}
		if err := m.revert(ctx, migration); err != nil {
			return reverted, err
		}
		reverted = append(reverted, migration.Name())
	}
	return reverted, nil
}

// Redo reverts and reapplies migrations.
func (m *Migrator) Redo(ctx context.Context, limit int) ([]string, []string, error) {
	var reverted []string
	var applied []string
	err := m.withLock(ctx, func() error {
		var err error
		reverted, err = m.down(ctx, limit)
		if err != nil {
			return err
		}
		applied, err = m.up(ctx, len(reverted))
		return err
	})
	return reverted, applied, err
}

// To migrates up or down until version is the latest applied migration.
func (m *Migrator) To(ctx context.Context, version string) error {
	return m.withLock(ctx, func() error {
		if version == "0" {
			_, err := m.down(ctx, 0)
			return err
		}
		if _, ok := m.registeredByName()[version]; !ok {
			return fmt.Errorf("%w: %s", ErrMigrationNotFound, version)
		}

		applied, err := m.Applied(ctx)
		if err != nil {
			return err
		}
		appliedSet := make(map[string]struct{}, len(applied))
		for _, migration := range applied {
			appliedSet[migration.Version] = struct{}{}
		}
		if _, ok := appliedSet[version]; ok {
			history, err := m.History(ctx, 0)
			if err != nil {
				return err
			}
			for _, migration := range history {
				if migration.Version == version {
					return nil
				}
				registered := m.registeredByName()
				if err := m.revert(ctx, registered[migration.Version]); err != nil {
					return err
				}
			}
			return nil
		}

		for _, migration := range m.migrations {
			if _, ok := appliedSet[migration.Name()]; ok {
				continue
			}
			if err := m.apply(ctx, migration); err != nil {
				return err
			}
			if migration.Name() == version {
				return nil
			}
		}
		return fmt.Errorf("%w: %s", ErrMigrationNotFound, version)
	})
}

// Mark changes migration history without running migration code.
func (m *Migrator) Mark(ctx context.Context, version string) error {
	return m.withLock(ctx, func() error {
		if !m.DryRun {
			if err := m.EnsureMigrationTable(ctx); err != nil {
				return err
			}
		}
		if version != "0" {
			if _, ok := m.registeredByName()[version]; !ok {
				return fmt.Errorf("%w: %s", ErrMigrationNotFound, version)
			}
		}
		applied, err := m.Applied(ctx)
		if err != nil {
			return err
		}
		for _, migration := range applied {
			if m.DryRun {
				m.logSQL(m.deleteMigrationSQL())
				continue
			}
			if _, err := m.db.ExecContext(ctx, m.deleteMigrationSQL(), migration.Version); err != nil {
				return err
			}
		}
		if version == "0" {
			return nil
		}
		for _, migration := range m.migrations {
			if m.DryRun {
				m.logSQL(m.insertMigrationSQL())
			} else if _, err := m.db.ExecContext(ctx, m.insertMigrationSQL(), migration.Name(), m.now().Unix()); err != nil {
				return err
			}
			if migration.Name() == version {
				return nil
			}
		}
		return nil
	})
}

func (m *Migrator) loadApplied(ctx context.Context, descending bool) ([]AppliedMigration, error) {
	rows, err := m.db.QueryContext(ctx, m.selectAppliedMigrationsSQL(descending))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var applied []AppliedMigration
	for rows.Next() {
		var migration AppliedMigration
		if err := rows.Scan(&migration.Version, &migration.ApplyTime); err != nil {
			return nil, err
		}
		applied = append(applied, migration)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return applied, nil
}

func (m *Migrator) apply(ctx context.Context, migration Migration) error {
	m.logMigration(">>> applying %s", migration.Name())
	if m.DryRun {
		migrationContext := NewMigrationContext(m.db, m.dialect, WithDryRun(true))
		if err := migration.Up(ctx, migrationContext); err != nil {
			return err
		}
		m.logMigration("<<< applied %s", migration.Name())
		return nil
	}

	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	migrationContext := NewMigrationContext(tx, m.dialect)
	if err := migration.Up(ctx, migrationContext); err != nil {
		return rollbackWithCause(tx, err)
	}
	if _, err := tx.ExecContext(ctx, m.insertMigrationSQL(), migration.Name(), m.now().Unix()); err != nil {
		return rollbackWithCause(tx, err)
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	m.logMigration("<<< applied %s", migration.Name())
	return nil
}

func (m *Migrator) revert(ctx context.Context, migration Migration) error {
	m.logMigration(">>> reverting %s", migration.Name())
	if m.DryRun {
		migrationContext := NewMigrationContext(m.db, m.dialect, WithDryRun(true))
		if err := migration.Down(ctx, migrationContext); err != nil {
			return err
		}
		m.logMigration("<<< reverted %s", migration.Name())
		return nil
	}

	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	migrationContext := NewMigrationContext(tx, m.dialect)
	if err := migration.Down(ctx, migrationContext); err != nil {
		return rollbackWithCause(tx, err)
	}
	if _, err := tx.ExecContext(ctx, m.deleteMigrationSQL(), migration.Name()); err != nil {
		return rollbackWithCause(tx, err)
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	m.logMigration("<<< reverted %s", migration.Name())
	return nil
}

func (m *Migrator) registeredByName() map[string]Migration {
	registered := make(map[string]Migration, len(m.migrations))
	for _, migration := range m.migrations {
		registered[migration.Name()] = migration
	}
	return registered
}

func (m *Migrator) migrationTable() string {
	if m.MigrationTable == "" {
		return DefaultMigrationTable
	}
	return m.MigrationTable
}

func (m *Migrator) selectAppliedMigrationsSQL(descending bool) string {
	direction := "ASC"
	if descending {
		direction = "DESC"
	}
	return "SELECT " + m.dialect.QuoteColumn("version") + ", " + m.dialect.QuoteColumn("apply_time") +
		" FROM " + m.dialect.QuoteTable(m.migrationTable()) +
		" ORDER BY " + m.dialect.QuoteColumn("apply_time") + " " + direction +
		", " + m.dialect.QuoteColumn("version") + " " + direction
}

func (m *Migrator) insertMigrationSQL() string {
	return "INSERT INTO " + m.dialect.QuoteTable(m.migrationTable()) +
		" (" + m.dialect.QuoteColumn("version") + ", " + m.dialect.QuoteColumn("apply_time") +
		") VALUES (" + m.dialect.Placeholder(1) + ", " + m.dialect.Placeholder(2) + ")"
}

func (m *Migrator) deleteMigrationSQL() string {
	return "DELETE FROM " + m.dialect.QuoteTable(m.migrationTable()) +
		" WHERE " + m.dialect.QuoteColumn("version") + " = " + m.dialect.Placeholder(1)
}

func (m *Migrator) withLock(ctx context.Context, fn func() error) error {
	if m.DryRun || !m.UseLock {
		return fn()
	}
	if err := m.acquireLock(ctx); err != nil {
		return err
	}
	err := fn()
	releaseErr := m.releaseLock(ctx)
	if err != nil {
		return err
	}
	return releaseErr
}

func (m *Migrator) acquireLock(ctx context.Context) error {
	statement, err := m.dialect.AcquireLockSQL(m.LockName, m.LockTimeoutSeconds)
	if err != nil {
		return err
	}
	var got int64
	if err := m.db.QueryRowContext(ctx, statement.Query, statement.Args...).Scan(&got); err != nil {
		return err
	}
	if got != 1 {
		return ErrMigrationLockTimeout
	}
	return nil
}

func (m *Migrator) releaseLock(ctx context.Context) error {
	statement, err := m.dialect.ReleaseLockSQL(m.LockName)
	if err != nil {
		return err
	}
	var released any
	return m.db.QueryRowContext(ctx, statement.Query, statement.Args...).Scan(&released)
}

func (m *Migrator) logMigration(format string, args ...any) {
	if m.logger != nil {
		m.logger.Printf(format, args...)
	}
}

func (m *Migrator) logSQL(query string) {
	if m.logger != nil {
		m.logger.Printf("    > %s", query)
	}
}

func rollbackWithCause(tx *sql.Tx, cause error) error {
	if err := tx.Rollback(); err != nil {
		return errors.Join(cause, err)
	}
	return cause
}

func limitMigrations(migrations []Migration, limit int) []Migration {
	if limit <= 0 || limit >= len(migrations) {
		return migrations
	}
	return migrations[:limit]
}

func limitApplied(applied []AppliedMigration, limit int) []AppliedMigration {
	if limit <= 0 || limit >= len(applied) {
		return applied
	}
	return applied[:limit]
}
