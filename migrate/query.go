package migrate

import (
	"context"
	"database/sql"
	"fmt"
)

// TableExists reports whether a table exists.
func (m *MigrationContext) TableExists(ctx context.Context, table string) (bool, error) {
	return m.queryCountStatement(ctx, m.dialect.TableExistsSQL(table))
}

// ColumnExists reports whether a column exists.
func (m *MigrationContext) ColumnExists(ctx context.Context, table, column string) (bool, error) {
	return m.queryCountStatement(ctx, m.dialect.ColumnExistsSQL(table, column))
}

// IndexExists reports whether an index exists.
func (m *MigrationContext) IndexExists(ctx context.Context, table, index string) (bool, error) {
	return m.queryCountStatement(ctx, m.dialect.IndexExistsSQL(table, index))
}

// ForeignKeyExists reports whether a foreign key exists.
func (m *MigrationContext) ForeignKeyExists(ctx context.Context, table, name string) (bool, error) {
	return m.queryCountStatement(ctx, m.dialect.ForeignKeyExistsSQL(table, name))
}

// ConstraintExists reports whether a constraint exists.
func (m *MigrationContext) ConstraintExists(ctx context.Context, table, name string) (bool, error) {
	return m.queryCountStatement(ctx, m.dialect.ConstraintExistsSQL(table, name))
}

// QueryValue returns the first column of the first row, or nil when no row exists.
func (m *MigrationContext) QueryValue(ctx context.Context, query string, args ...any) (any, error) {
	if m.db == nil {
		return nil, fmt.Errorf("migrate: database handle is nil")
	}
	rows, err := m.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, rows.Err()
	}
	var value any
	if err := rows.Scan(&value); err != nil {
		return nil, err
	}
	return normalizeValue(value), rows.Err()
}

// QueryOne returns the first row as a Row, or nil when no row exists.
func (m *MigrationContext) QueryOne(ctx context.Context, query string, args ...any) (Row, error) {
	rows, err := m.queryRows(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, rows.Err()
	}
	return scanRow(rows)
}

// QueryAll returns all rows as Row values.
func (m *MigrationContext) QueryAll(ctx context.Context, query string, args ...any) ([]Row, error) {
	rows, err := m.queryRows(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Row{}
	for rows.Next() {
		row, err := scanRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// RowExists reports whether a row matching condition exists.
func (m *MigrationContext) RowExists(ctx context.Context, table string, condition string, args ...any) (bool, error) {
	value, err := m.QueryValue(ctx, m.dialect.BuildRowExistsSQL(table, condition), args...)
	if err != nil {
		return false, err
	}
	return truthy(value), nil
}

// CountRows returns the number of rows matching condition.
func (m *MigrationContext) CountRows(ctx context.Context, table string, condition string, args ...any) (int64, error) {
	value, err := m.QueryValue(ctx, m.dialect.BuildCountRowsSQL(table, condition), args...)
	if err != nil {
		return 0, err
	}
	return toInt64(value), nil
}

func (m *MigrationContext) queryRows(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	if m.db == nil {
		return nil, fmt.Errorf("migrate: database handle is nil")
	}
	return m.db.QueryContext(ctx, query, args...)
}

func (m *MigrationContext) queryCountStatement(ctx context.Context, statement SQLStatement) (bool, error) {
	value, err := m.QueryValue(ctx, statement.Query, statement.Args...)
	if err != nil {
		return false, err
	}
	return toInt64(value) > 0, nil
}

func scanRow(rows *sql.Rows) (Row, error) {
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	values := make([]any, len(columns))
	dest := make([]any, len(columns))
	for i := range values {
		dest[i] = &values[i]
	}
	if err := rows.Scan(dest...); err != nil {
		return nil, err
	}
	row := Row{}
	for i, column := range columns {
		row[column] = normalizeValue(values[i])
	}
	return row, nil
}

func normalizeValue(value any) any {
	if bytes, ok := value.([]byte); ok {
		return string(bytes)
	}
	return value
}

func truthy(value any) bool {
	return toInt64(value) != 0
}

func toInt64(value any) int64 {
	switch v := value.(type) {
	case nil:
		return 0
	case int:
		return int64(v)
	case int8:
		return int64(v)
	case int16:
		return int64(v)
	case int32:
		return int64(v)
	case int64:
		return v
	case uint:
		return int64(v)
	case uint8:
		return int64(v)
	case uint16:
		return int64(v)
	case uint32:
		return int64(v)
	case uint64:
		return int64(v)
	case bool:
		if v {
			return 1
		}
		return 0
	default:
		return 0
	}
}
