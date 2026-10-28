package migrate

import (
	"fmt"
	"strings"
)

// baseDialect carries the statement shapes every adapter shares. Each adapter
// embeds it, so the shared builders are reachable through method promotion,
// and passes itself as the Dialect argument so quoting, placeholders and
// column rendering dispatch through the embedding adapter's implementations.
// Go embedding is composition, not inheritance: a method on baseDialect never
// calls the outer adapter's overrides on its own, hence the explicit self
// parameter on every shared shape.
type baseDialect struct{}

// buildRowExistsSQL renders a SELECT EXISTS(...) probe over one table.
func (baseDialect) buildRowExistsSQL(d Dialect, table string, condition string) string {
	query := "SELECT EXISTS(SELECT 1 FROM " + d.QuoteTable(table)
	if strings.TrimSpace(condition) != "" {
		query += " WHERE " + condition
	}
	return query + ")"
}

// buildCountRowsSQL renders a SELECT COUNT(*) over one table.
func (baseDialect) buildCountRowsSQL(d Dialect, table string, condition string) string {
	query := "SELECT COUNT(*) FROM " + d.QuoteTable(table)
	if strings.TrimSpace(condition) != "" {
		query += " WHERE " + condition
	}
	return query
}

// buildCreateTable assembles the shared CREATE TABLE skeleton: each column is
// rendered through the adapter's BuildColumn. Adapters that accept table
// options pass allowOptions=true and get the options appended verbatim; the
// others get an unsupported-operation error instead of silent option loss.
func (baseDialect) buildCreateTable(d Dialect, table string, columns *ColumnList, options string, allowOptions bool) (string, error) {
	if !allowOptions && strings.TrimSpace(options) != "" {
		return "", unsupported(d.Name(), "TABLE OPTIONS")
	}
	defs := make([]string, 0, len(columns.Items()))
	for _, item := range columns.Items() {
		columnSQL, err := d.BuildColumn(item.Column)
		if err != nil {
			return "", err
		}
		defs = append(defs, d.QuoteColumn(item.Name)+" "+columnSQL)
	}
	query := fmt.Sprintf("CREATE TABLE %s (%s)", d.QuoteTable(table), strings.Join(defs, ", "))
	if allowOptions {
		query += joinOptions(options)
	}
	return query, nil
}

// buildCreateMigrationTableSQL renders the idempotent CREATE TABLE IF NOT
// EXISTS statement for the migration history table.
func (baseDialect) buildCreateMigrationTableSQL(d Dialect, table string) (string, error) {
	return fmt.Sprintf(
		"CREATE TABLE IF NOT EXISTS %s (%s varchar(180) NOT NULL PRIMARY KEY, %s int NOT NULL)",
		d.QuoteTable(table), d.QuoteColumn("version"), d.QuoteColumn("apply_time"),
	), nil
}

// buildDropTable renders a strict DROP TABLE: a missing object is left to the
// database to report as its natural error. Idempotent drops belong to the
// plan layer's existence probes, so no adapter emits IF EXISTS from here.
func (baseDialect) buildDropTable(d Dialect, table string) (string, error) {
	return "DROP TABLE " + d.QuoteTable(table), nil
}

// buildRenameTable renders ALTER TABLE ... RENAME TO.
func (baseDialect) buildRenameTable(d Dialect, oldName, newName string) (string, error) {
	return fmt.Sprintf("ALTER TABLE %s RENAME TO %s", d.QuoteTable(oldName), d.QuoteTable(newName)), nil
}

// buildTruncateTable renders TRUNCATE TABLE.
func (baseDialect) buildTruncateTable(d Dialect, table string) (string, error) {
	return "TRUNCATE TABLE " + d.QuoteTable(table), nil
}

// buildAddColumn renders ALTER TABLE ... ADD COLUMN with the full column
// definition from the adapter's BuildColumn.
func (baseDialect) buildAddColumn(d Dialect, table, column string, builder *ColumnBuilder) (string, error) {
	columnSQL, err := d.BuildColumn(builder)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", d.QuoteTable(table), d.QuoteColumn(column), columnSQL), nil
}

// buildDropColumn renders ALTER TABLE ... DROP COLUMN.
func (baseDialect) buildDropColumn(d Dialect, table, column string) (string, error) {
	return fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s", d.QuoteTable(table), d.QuoteColumn(column)), nil
}

// buildRenameColumn renders ALTER TABLE ... RENAME COLUMN ... TO.
func (baseDialect) buildRenameColumn(d Dialect, table, oldName, newName string) (string, error) {
	return fmt.Sprintf("ALTER TABLE %s RENAME COLUMN %s TO %s", d.QuoteTable(table), d.QuoteColumn(oldName), d.QuoteColumn(newName)), nil
}

// buildCreateIndex renders CREATE [UNIQUE] INDEX over the quoted column list.
func (baseDialect) buildCreateIndex(d Dialect, name, table string, columns []string, unique bool) (string, error) {
	prefix := "CREATE INDEX"
	if unique {
		prefix = "CREATE UNIQUE INDEX"
	}
	return fmt.Sprintf("%s %s ON %s (%s)", prefix, d.QuoteTable(name), d.QuoteTable(table), columnList(d, columns)), nil
}

// buildDropIndex renders DROP INDEX ... ON table for engines that require the
// table clause.
func (baseDialect) buildDropIndex(d Dialect, name, table string) (string, error) {
	return fmt.Sprintf("DROP INDEX %s ON %s", d.QuoteTable(name), d.QuoteTable(table)), nil
}

// buildAddPrimaryKey renders ALTER TABLE ... ADD CONSTRAINT ... PRIMARY KEY.
func (baseDialect) buildAddPrimaryKey(d Dialect, name, table string, columns []string) (string, error) {
	return fmt.Sprintf("ALTER TABLE %s ADD CONSTRAINT %s PRIMARY KEY (%s)", d.QuoteTable(table), d.QuoteTable(name), columnList(d, columns)), nil
}

// buildDropConstraint renders ALTER TABLE ... DROP CONSTRAINT for engines
// that expose primary keys and foreign keys as named constraints.
func (baseDialect) buildDropConstraint(d Dialect, name, table string) (string, error) {
	return fmt.Sprintf("ALTER TABLE %s DROP CONSTRAINT %s", d.QuoteTable(table), d.QuoteTable(name)), nil
}

// buildAddForeignKey renders ALTER TABLE ... ADD CONSTRAINT ... FOREIGN KEY
// with the optional ON DELETE and ON UPDATE actions.
func (baseDialect) buildAddForeignKey(d Dialect, name, table string, columns []string, refTable string, refColumns []string, onDelete ForeignKeyAction, onUpdate ForeignKeyAction) (string, error) {
	query := fmt.Sprintf("ALTER TABLE %s ADD CONSTRAINT %s FOREIGN KEY (%s) REFERENCES %s (%s)", d.QuoteTable(table), d.QuoteTable(name), columnList(d, columns), d.QuoteTable(refTable), columnList(d, refColumns))
	if onDelete != "" {
		query += " ON DELETE " + string(onDelete)
	}
	if onUpdate != "" {
		query += " ON UPDATE " + string(onUpdate)
	}
	return query, nil
}

// buildInsert renders an INSERT statement for one row. Column order is sorted
// so the generated SQL is deterministic.
func (baseDialect) buildInsert(d Dialect, table string, row Row) (SQLStatement, error) {
	if len(row) == 0 {
		return SQLStatement{}, fmt.Errorf("migrate: insert row is empty")
	}
	keys := sortedRowKeys(row)
	columns := make([]string, 0, len(keys))
	values := make([]string, 0, len(keys))
	args := make([]any, 0, len(keys))
	for _, key := range keys {
		columns = append(columns, d.QuoteColumn(key))
		switch value := row[key].(type) {
		case Expression:
			values = append(values, string(value))
		default:
			args = append(args, value)
			values = append(values, d.Placeholder(len(args)))
		}
	}
	return SQLStatement{
		Query: fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", d.QuoteTable(table), strings.Join(columns, ", "), strings.Join(values, ", ")),
		Args:  args,
	}, nil
}

// buildBatchInsert renders one INSERT statement for many rows of values.
func (baseDialect) buildBatchInsert(d Dialect, table string, columns []string, rows [][]any) (SQLStatement, error) {
	if len(columns) == 0 {
		return SQLStatement{}, fmt.Errorf("migrate: batch insert columns are empty")
	}
	if len(rows) == 0 {
		return SQLStatement{}, fmt.Errorf("migrate: batch insert rows are empty")
	}
	quotedColumns := make([]string, 0, len(columns))
	for _, column := range columns {
		quotedColumns = append(quotedColumns, d.QuoteColumn(column))
	}
	args := make([]any, 0, len(columns)*len(rows))
	valueGroups := make([]string, 0, len(rows))
	for _, row := range rows {
		if len(row) != len(columns) {
			return SQLStatement{}, fmt.Errorf("migrate: batch insert row has %d values for %d columns", len(row), len(columns))
		}
		values := make([]string, 0, len(row))
		for _, value := range row {
			if expr, ok := value.(Expression); ok {
				values = append(values, string(expr))
				continue
			}
			args = append(args, value)
			values = append(values, d.Placeholder(len(args)))
		}
		valueGroups = append(valueGroups, "("+strings.Join(values, ", ")+")")
	}
	return SQLStatement{
		Query: fmt.Sprintf("INSERT INTO %s (%s) VALUES %s", d.QuoteTable(table), strings.Join(quotedColumns, ", "), strings.Join(valueGroups, ", ")),
		Args:  args,
	}, nil
}

// buildUpdate renders an UPDATE statement for the columns in row; Expression
// values are inlined, plain values become bound arguments.
func (baseDialect) buildUpdate(d Dialect, table string, row Row, condition string, condArgs ...any) (SQLStatement, error) {
	if len(row) == 0 {
		return SQLStatement{}, fmt.Errorf("migrate: update row is empty")
	}
	keys := sortedRowKeys(row)
	args := make([]any, 0, len(row)+len(condArgs))
	sets := make([]string, 0, len(keys))
	for _, key := range keys {
		if expr, ok := row[key].(Expression); ok {
			sets = append(sets, d.QuoteColumn(key)+" = "+string(expr))
			continue
		}
		args = append(args, row[key])
		sets = append(sets, d.QuoteColumn(key)+" = "+d.Placeholder(len(args)))
	}
	query := fmt.Sprintf("UPDATE %s SET %s", d.QuoteTable(table), strings.Join(sets, ", "))
	if strings.TrimSpace(condition) != "" {
		query += " WHERE " + condition
		args = append(args, condArgs...)
	}
	return SQLStatement{Query: query, Args: args}, nil
}

// buildDelete renders a DELETE statement for the rows matching condition.
func (baseDialect) buildDelete(d Dialect, table string, condition string, args ...any) (SQLStatement, error) {
	query := "DELETE FROM " + d.QuoteTable(table)
	if strings.TrimSpace(condition) != "" {
		query += " WHERE " + condition
	}
	return SQLStatement{Query: query, Args: append([]any(nil), args...)}, nil
}
