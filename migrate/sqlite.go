package migrate

import (
	"fmt"
	"strings"
)

// SQLiteDialect generates SQL for SQLite.
type SQLiteDialect struct {
	baseDialect
}

// Name returns the dialect name.
func (d SQLiteDialect) Name() string { return "sqlite" }

// QuoteTable quotes a table name with double quotes.
func (d SQLiteDialect) QuoteTable(name string) string { return quoteName(name, `"`) }

// QuoteColumn quotes a column name with double quotes.
func (d SQLiteDialect) QuoteColumn(name string) string { return quoteName(name, `"`) }

// QuoteIndexColumn quotes an index column unless it is an expression.
func (d SQLiteDialect) QuoteIndexColumn(name string) string { return quoteIndexColumn(d, name) }

// Placeholder returns a SQLite placeholder.
func (d SQLiteDialect) Placeholder(index int) string { return "?" }

// BuildColumn builds one SQLite column definition.
func (d SQLiteDialect) BuildColumn(c *ColumnBuilder) (string, error) {
	if c == nil {
		return "", fmt.Errorf("migrate: column builder is nil")
	}
	if c.comment != "" {
		return "", unsupported(d.Name(), "COLUMN COMMENT")
	}
	if c.after != "" {
		return "", unsupported(d.Name(), "AFTER")
	}
	if c.first {
		return "", unsupported(d.Name(), "FIRST")
	}
	if c.charset != "" {
		return "", unsupported(d.Name(), "CHARACTER SET")
	}
	if c.collation != "" {
		return "", unsupported(d.Name(), "COLLATE")
	}
	if c.primaryKey {
		parts := []string{"INTEGER", "PRIMARY KEY"}
		if c.autoIncrement {
			parts = append(parts, "AUTOINCREMENT")
		}
		if c.appendSQL != "" {
			parts = append(parts, c.appendSQL)
		}
		return strings.Join(parts, " "), nil
	}
	typeSQL, err := d.sqliteType(c)
	if err != nil {
		return "", err
	}
	parts := []string{typeSQL}
	if c.nullSet {
		if c.nullable {
			parts = append(parts, "NULL")
		} else {
			parts = append(parts, "NOT NULL")
		}
	}
	if c.unique {
		parts = append(parts, "UNIQUE")
	}
	if c.defaultExpr != "" {
		parts = append(parts, "DEFAULT", c.defaultExpr)
	}
	if c.defaultSet {
		parts = append(parts, "DEFAULT", sqlLiteral(c.defaultValue))
	}
	if c.check != "" {
		parts = append(parts, "CHECK ("+c.check+")")
	}
	if c.generatedAs != "" {
		parts = append(parts, "GENERATED ALWAYS AS ("+c.generatedAs+")")
		if c.generatedKind != "" {
			parts = append(parts, c.generatedKind)
		}
	}
	if c.appendSQL != "" {
		parts = append(parts, c.appendSQL)
	}
	return strings.Join(parts, " "), nil
}

// sqliteType maps a column builder onto its SQLite type name. A type without a
// mapping is rejected with an unsupported error instead of being spliced into
// the SQL verbatim.
func (d SQLiteDialect) sqliteType(c *ColumnBuilder) (string, error) {
	switch c.typeName {
	case "tinyInteger", "smallInteger", "integer", "bigInteger", "boolean":
		return "INTEGER", nil
	case "string":
		return sizedType("VARCHAR", c.size, 255), nil
	case "char":
		return sizedType("CHAR", c.size), nil
	case "text", "tinyText", "mediumText", "longText", "date", "dateTime", "time", "timestamp", "json", "uuid", "enum", "set":
		return "TEXT", nil
	case "binary", "tinyBlob", "mediumBlob", "longBlob":
		return "BLOB", nil
	case "float", "double":
		return "REAL", nil
	case "decimal", "money":
		return sizedType("NUMERIC", c.size), nil
	default:
		return "", unsupported(d.Name(), c.typeName)
	}
}

// TableExistsSQL returns SQL checking for a table or view.
func (d SQLiteDialect) TableExistsSQL(table string) SQLStatement {
	return SQLStatement{Query: "SELECT COUNT(*) FROM sqlite_master WHERE type IN ('table', 'view') AND name = ?", Args: []any{table}}
}

// ColumnExistsSQL returns SQL checking for a column.
func (d SQLiteDialect) ColumnExistsSQL(table, column string) SQLStatement {
	return SQLStatement{Query: "SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?", Args: []any{table, column}}
}

// IndexExistsSQL returns SQL checking for an index.
func (d SQLiteDialect) IndexExistsSQL(table, index string) SQLStatement {
	return SQLStatement{Query: "SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND tbl_name = ? AND name = ?", Args: []any{table, index}}
}

// ForeignKeyExistsSQL returns SQL that reports no named foreign keys.
func (d SQLiteDialect) ForeignKeyExistsSQL(table, name string) SQLStatement {
	return SQLStatement{Query: "SELECT 0", Args: nil}
}

// ConstraintExistsSQL returns SQL that reports no named constraints.
func (d SQLiteDialect) ConstraintExistsSQL(table, name string) SQLStatement {
	return SQLStatement{Query: "SELECT 0", Args: nil}
}

// BuildRowExistsSQL returns SQL for checking if a row exists.
func (d SQLiteDialect) BuildRowExistsSQL(table string, condition string) string {
	return d.buildRowExistsSQL(d, table, condition)
}

// BuildCountRowsSQL returns SQL for counting rows.
func (d SQLiteDialect) BuildCountRowsSQL(table string, condition string) string {
	return d.buildCountRowsSQL(d, table, condition)
}

// CreateTable builds CREATE TABLE SQL; SQLite cannot take table options.
func (d SQLiteDialect) CreateTable(table string, columns *ColumnList, options string) (string, error) {
	return d.buildCreateTable(d, table, columns, options, false)
}

// CreateMigrationTableSQL builds idempotent SQL that creates the migration
// table (version varchar(180) primary key, apply_time int) when it is absent.
func (d SQLiteDialect) CreateMigrationTableSQL(table string) (string, error) {
	return d.buildCreateMigrationTableSQL(d, table)
}

// DropTable builds a strict DROP TABLE; idempotent drops belong to the plan
// layer's existence probes.
func (d SQLiteDialect) DropTable(table string) (string, error) {
	return d.buildDropTable(d, table)
}

// RenameTable builds ALTER TABLE RENAME SQL.
func (d SQLiteDialect) RenameTable(oldName, newName string) (string, error) {
	return d.buildRenameTable(d, oldName, newName)
}

// TruncateTable returns an unsupported-operation error.
func (d SQLiteDialect) TruncateTable(table string) (string, error) {
	return "", unsupported(d.Name(), "TRUNCATE TABLE")
}

// AddColumn builds ADD COLUMN SQL.
func (d SQLiteDialect) AddColumn(table, column string, builder *ColumnBuilder) (string, error) {
	return d.buildAddColumn(d, table, column, builder)
}

// AlterColumn returns an unsupported-operation error.
func (d SQLiteDialect) AlterColumn(table, column string, builder *ColumnBuilder) (string, error) {
	return "", unsupported(d.Name(), "ALTER COLUMN")
}

// DropColumn returns an unsupported-operation error.
func (d SQLiteDialect) DropColumn(table, column string) (string, error) {
	return "", unsupported(d.Name(), "DROP COLUMN")
}

// RenameColumn returns an unsupported-operation error.
func (d SQLiteDialect) RenameColumn(table, oldName, newName string) (string, error) {
	return "", unsupported(d.Name(), "RENAME COLUMN")
}

// CreateIndex builds CREATE INDEX SQL.
func (d SQLiteDialect) CreateIndex(name, table string, columns []string, unique bool) (string, error) {
	return d.buildCreateIndex(d, name, table, columns, unique)
}

// DropIndex builds a strict DROP INDEX; the table argument is unused because
// SQLite drops an index by name alone. Idempotent drops belong to the plan
// layer's existence probes.
func (d SQLiteDialect) DropIndex(name, table string) (string, error) {
	return "DROP INDEX " + d.QuoteTable(name), nil
}

// AddPrimaryKey returns an unsupported-operation error.
func (d SQLiteDialect) AddPrimaryKey(name, table string, columns []string) (string, error) {
	return "", unsupported(d.Name(), "ADD PRIMARY KEY")
}

// DropPrimaryKey returns an unsupported-operation error.
func (d SQLiteDialect) DropPrimaryKey(name, table string) (string, error) {
	return "", unsupported(d.Name(), "DROP PRIMARY KEY")
}

// AddForeignKey returns an unsupported-operation error.
func (d SQLiteDialect) AddForeignKey(name, table string, columns []string, refTable string, refColumns []string, onDelete ForeignKeyAction, onUpdate ForeignKeyAction) (string, error) {
	return "", unsupported(d.Name(), "ADD FOREIGN KEY")
}

// DropForeignKey returns an unsupported-operation error.
func (d SQLiteDialect) DropForeignKey(name, table string) (string, error) {
	return "", unsupported(d.Name(), "DROP FOREIGN KEY")
}

// AddCommentOnColumn returns an unsupported-operation error.
func (d SQLiteDialect) AddCommentOnColumn(table, column, comment string) (string, error) {
	return "", unsupported(d.Name(), "COLUMN COMMENT")
}

// DropCommentFromColumn returns an unsupported-operation error.
func (d SQLiteDialect) DropCommentFromColumn(table, column string) (string, error) {
	return "", unsupported(d.Name(), "COLUMN COMMENT")
}

// AddCommentOnTable returns an unsupported-operation error.
func (d SQLiteDialect) AddCommentOnTable(table, comment string) (string, error) {
	return "", unsupported(d.Name(), "TABLE COMMENT")
}

// DropCommentFromTable returns an unsupported-operation error.
func (d SQLiteDialect) DropCommentFromTable(table string) (string, error) {
	return "", unsupported(d.Name(), "TABLE COMMENT")
}

// Insert builds INSERT SQL.
func (d SQLiteDialect) Insert(table string, row Row) (SQLStatement, error) {
	return d.buildInsert(d, table, row)
}

// BatchInsert builds multi-row INSERT SQL.
func (d SQLiteDialect) BatchInsert(table string, columns []string, rows [][]any) (SQLStatement, error) {
	return d.buildBatchInsert(d, table, columns, rows)
}

// Update builds UPDATE SQL.
func (d SQLiteDialect) Update(table string, row Row, condition string, args ...any) (SQLStatement, error) {
	return d.buildUpdate(d, table, row, condition, args...)
}

// Delete builds DELETE SQL.
func (d SQLiteDialect) Delete(table string, condition string, args ...any) (SQLStatement, error) {
	return d.buildDelete(d, table, condition, args...)
}

// AcquireLockSQL returns an unsupported-operation error.
func (d SQLiteDialect) AcquireLockSQL(lockName string, timeoutSeconds int) (SQLStatement, error) {
	return SQLStatement{}, unsupported(d.Name(), "ADVISORY LOCK")
}

// ReleaseLockSQL returns an unsupported-operation error.
func (d SQLiteDialect) ReleaseLockSQL(lockName string) (SQLStatement, error) {
	return SQLStatement{}, unsupported(d.Name(), "ADVISORY LOCK")
}
