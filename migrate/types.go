package migrate

import (
	"context"
	"database/sql"
	"fmt"
)

// DBTX is the common database interface implemented by *sql.DB and *sql.Tx.
type DBTX interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// Row represents a row for DML helpers.
type Row map[string]any

// Expression marks raw SQL that should be inserted into generated SQL directly.
type Expression string

// Expr returns a raw SQL expression for DML values or column defaults.
func Expr(sql string) Expression {
	return Expression(sql)
}

// SQLStatement contains a SQL query and its bound arguments.
type SQLStatement struct {
	Query string
	Args  []any
}

// MigrationContext exposes Yii2-style helpers bound to a database and dialect.
type MigrationContext struct {
	db      DBTX
	dialect Dialect
	dryRun  bool
}

// ContextOption configures a MigrationContext.
type ContextOption func(*MigrationContext)

// WithDryRun configures whether schema plans should print but not execute SQL.
func WithDryRun(dryRun bool) ContextOption {
	return func(m *MigrationContext) {
		m.dryRun = dryRun
	}
}

// NewMigrationContext creates a migration context for db and dialect.
func NewMigrationContext(db DBTX, dialect Dialect, opts ...ContextOption) *MigrationContext {
	if dialect == nil {
		dialect = MySQLDialect{}
	}
	m := &MigrationContext{db: db, dialect: dialect}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// DB returns the database handle used by the context.
func (m *MigrationContext) DB() DBTX {
	return m.db
}

// Dialect returns the SQL dialect used by the context.
func (m *MigrationContext) Dialect() Dialect {
	return m.dialect
}

// DryRun reports whether schema plans skip execution.
func (m *MigrationContext) DryRun() bool {
	return m.dryRun
}

// Schema creates a new chainable schema plan.
func (m *MigrationContext) Schema() *SchemaPlan {
	return &SchemaPlan{ctx: m}
}

// Execute runs one SQL statement immediately.
func (m *MigrationContext) Execute(ctx context.Context, query string, args ...any) error {
	if m.db == nil {
		return fmt.Errorf("migrate: database handle is nil")
	}
	if m.dryRun {
		return nil
	}
	_, err := m.db.ExecContext(ctx, query, args...)
	return err
}

// UnsupportedOperationError describes an operation a dialect cannot generate.
type UnsupportedOperationError struct {
	Dialect   string
	Operation string
}

// Error returns the unsupported operation message.
func (e *UnsupportedOperationError) Error() string {
	return fmt.Sprintf("%s does not support %s", e.Dialect, e.Operation)
}

func unsupported(dialect, operation string) error {
	return &UnsupportedOperationError{Dialect: dialect, Operation: operation}
}
