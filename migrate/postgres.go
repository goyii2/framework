package migrate

import (
	"fmt"
	"strings"
)

// PostgreSQLDialect generates SQL for PostgreSQL.
type PostgreSQLDialect struct {
	baseDialect
}

// Name returns the dialect name.
func (d PostgreSQLDialect) Name() string { return "postgres" }

// QuoteTable quotes a table name with double quotes.
func (d PostgreSQLDialect) QuoteTable(name string) string { return quoteName(name, `"`) }

// QuoteColumn quotes a column name with double quotes.
func (d PostgreSQLDialect) QuoteColumn(name string) string { return quoteName(name, `"`) }

// QuoteIndexColumn quotes an index column unless it is an expression.
func (d PostgreSQLDialect) QuoteIndexColumn(name string) string { return quoteIndexColumn(d, name) }

// Placeholder returns a PostgreSQL positional placeholder.
func (d PostgreSQLDialect) Placeholder(index int) string {
	return "$" + fmt.Sprintf("%d", index)
}

// BuildColumn builds one PostgreSQL column definition.
func (d PostgreSQLDialect) BuildColumn(c *ColumnBuilder) (string, error) {
	if c == nil {
		return "", fmt.Errorf("migrate: column builder is nil")
	}
	if c.unsigned {
		return "", unsupported(d.Name(), "UNSIGNED")
	}
	if c.charset != "" {
		return "", unsupported(d.Name(), "CHARACTER SET")
	}
	if c.collation != "" {
		return "", unsupported(d.Name(), "COLLATE")
	}
	if c.after != "" {
		return "", unsupported(d.Name(), "AFTER")
	}
	if c.first {
		return "", unsupported(d.Name(), "FIRST")
	}

	serial := pgSerialType(c)
	parts := []string{serial}
	if serial == "" {
		typeSQL, err := d.pgType(c)
		if err != nil {
			return "", err
		}
		parts = []string{typeSQL}
		if c.nullSet {
			if c.nullable {
				parts = append(parts, "NULL")
			} else {
				parts = append(parts, "NOT NULL")
			}
		}
	} else {
		if c.primaryKey {
			parts = append(parts, "PRIMARY KEY")
		}
	}
	if serial == "" && c.defaultExpr != "" {
		parts = append(parts, "DEFAULT", c.defaultExpr)
	}
	if serial == "" && c.defaultSet {
		parts = append(parts, "DEFAULT", sqlLiteral(c.defaultValue))
	}
	if c.check != "" {
		parts = append(parts, "CHECK ("+c.check+")")
	}
	if c.unique {
		parts = append(parts, "UNIQUE")
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

func pgSerialType(c *ColumnBuilder) string {
	if c.primaryKey && c.autoIncrement {
		switch c.typeName {
		case "integer":
			return "SERIAL"
		case "bigInteger":
			return "BIGSERIAL"
		}
	}
	return ""
}

// pgType maps a column builder onto its PostgreSQL type name. A type without
// a mapping is rejected with an unsupported error instead of being spliced
// into the SQL verbatim.
func (d PostgreSQLDialect) pgType(c *ColumnBuilder) (string, error) {
	switch c.typeName {
	case "tinyInteger":
		return "SMALLINT", nil
	case "smallInteger":
		return "SMALLINT", nil
	case "integer":
		return "INTEGER", nil
	case "bigInteger":
		return "BIGINT", nil
	case "string":
		return sizedType("VARCHAR", c.size, 255), nil
	case "char":
		return sizedType("CHAR", c.size), nil
	case "text":
		return "TEXT", nil
	case "tinyText":
		return "TEXT", nil
	case "mediumText":
		return "TEXT", nil
	case "longText":
		return "TEXT", nil
	case "binary":
		return "BYTEA", nil
	case "tinyBlob":
		return "BYTEA", nil
	case "mediumBlob":
		return "BYTEA", nil
	case "longBlob":
		return "BYTEA", nil
	case "boolean":
		return "BOOLEAN", nil
	case "float":
		return "REAL", nil
	case "double":
		return "DOUBLE PRECISION", nil
	case "decimal":
		return sizedType("DECIMAL", c.size), nil
	case "money":
		return "MONEY", nil
	case "date":
		return "DATE", nil
	case "dateTime":
		return precisionType("TIMESTAMP", c.size), nil
	case "time":
		return precisionType("TIME", c.size), nil
	case "timestamp":
		return precisionType("TIMESTAMP", c.size), nil
	case "json":
		return "JSONB", nil
	case "uuid":
		return "UUID", nil
	case "enum":
		return "TEXT", nil
	case "set":
		return "TEXT", nil
	default:
		return "", unsupported(d.Name(), c.typeName)
	}
}

// TableExistsSQL returns SQL checking for a table.
func (d PostgreSQLDialect) TableExistsSQL(table string) SQLStatement {
	return SQLStatement{
		Query: "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = current_schema() AND table_name = $1",
		Args:  []any{table},
	}
}

// ColumnExistsSQL returns SQL checking for a column.
func (d PostgreSQLDialect) ColumnExistsSQL(table, column string) SQLStatement {
	return SQLStatement{
		Query: "SELECT COUNT(*) FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = $1 AND column_name = $2",
		Args:  []any{table, column},
	}
}

// IndexExistsSQL returns SQL checking for an index.
func (d PostgreSQLDialect) IndexExistsSQL(table, index string) SQLStatement {
	return SQLStatement{
		Query: "SELECT COUNT(*) FROM pg_indexes WHERE schemaname = current_schema() AND tablename = $1 AND indexname = $2",
		Args:  []any{table, index},
	}
}

// ForeignKeyExistsSQL returns SQL checking for a foreign key.
func (d PostgreSQLDialect) ForeignKeyExistsSQL(table, name string) SQLStatement {
	return SQLStatement{
		Query: "SELECT COUNT(*) FROM information_schema.table_constraints WHERE table_schema = current_schema() AND table_name = $1 AND constraint_name = $2 AND constraint_type = 'FOREIGN KEY'",
		Args:  []any{table, name},
	}
}

// ConstraintExistsSQL returns SQL checking for a constraint.
func (d PostgreSQLDialect) ConstraintExistsSQL(table, name string) SQLStatement {
	return SQLStatement{
		Query: "SELECT COUNT(*) FROM information_schema.table_constraints WHERE table_schema = current_schema() AND table_name = $1 AND constraint_name = $2",
		Args:  []any{table, name},
	}
}

// BuildRowExistsSQL returns SQL for checking if a row exists.
func (d PostgreSQLDialect) BuildRowExistsSQL(table string, condition string) string {
	return d.buildRowExistsSQL(d, table, condition)
}

// BuildCountRowsSQL returns SQL for counting rows.
func (d PostgreSQLDialect) BuildCountRowsSQL(table string, condition string) string {
	return d.buildCountRowsSQL(d, table, condition)
}

// CreateTable builds CREATE TABLE SQL with table options.
func (d PostgreSQLDialect) CreateTable(table string, columns *ColumnList, options string) (string, error) {
	return d.buildCreateTable(d, table, columns, options, true)
}

// CreateMigrationTableSQL builds idempotent SQL that creates the migration
// table (version varchar(180) primary key, apply_time int) when it is absent.
func (d PostgreSQLDialect) CreateMigrationTableSQL(table string) (string, error) {
	return d.buildCreateMigrationTableSQL(d, table)
}

// DropTable builds a strict DROP TABLE; idempotent drops belong to the plan
// layer's existence probes.
func (d PostgreSQLDialect) DropTable(table string) (string, error) {
	return d.buildDropTable(d, table)
}

// RenameTable builds ALTER TABLE RENAME SQL.
func (d PostgreSQLDialect) RenameTable(oldName, newName string) (string, error) {
	return d.buildRenameTable(d, oldName, newName)
}

// TruncateTable builds TRUNCATE TABLE SQL.
func (d PostgreSQLDialect) TruncateTable(table string) (string, error) {
	return d.buildTruncateTable(d, table)
}

// AddColumn builds ADD COLUMN SQL.
func (d PostgreSQLDialect) AddColumn(table, column string, builder *ColumnBuilder) (string, error) {
	return d.buildAddColumn(d, table, column, builder)
}

// AlterColumn builds ALTER COLUMN TYPE SQL.
func (d PostgreSQLDialect) AlterColumn(table, column string, builder *ColumnBuilder) (string, error) {
	typeSQL, err := d.pgType(builder)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s TYPE %s", d.QuoteTable(table), d.QuoteColumn(column), typeSQL), nil
}

// DropColumn builds DROP COLUMN SQL.
func (d PostgreSQLDialect) DropColumn(table, column string) (string, error) {
	return d.buildDropColumn(d, table, column)
}

// RenameColumn builds RENAME COLUMN SQL.
func (d PostgreSQLDialect) RenameColumn(table, oldName, newName string) (string, error) {
	return d.buildRenameColumn(d, table, oldName, newName)
}

// CreateIndex builds CREATE INDEX SQL.
func (d PostgreSQLDialect) CreateIndex(name, table string, columns []string, unique bool) (string, error) {
	return d.buildCreateIndex(d, name, table, columns, unique)
}

// DropIndex builds a strict DROP INDEX; the table argument is unused because
// PostgreSQL drops an index by name alone. Idempotent drops belong to the
// plan layer's existence probes.
func (d PostgreSQLDialect) DropIndex(name, table string) (string, error) {
	return "DROP INDEX " + d.QuoteTable(name), nil
}

// AddPrimaryKey builds ADD PRIMARY KEY SQL.
func (d PostgreSQLDialect) AddPrimaryKey(name, table string, columns []string) (string, error) {
	return d.buildAddPrimaryKey(d, name, table, columns)
}

// DropPrimaryKey builds DROP CONSTRAINT SQL for a primary key.
func (d PostgreSQLDialect) DropPrimaryKey(name, table string) (string, error) {
	return d.buildDropConstraint(d, name, table)
}

// AddForeignKey builds ADD FOREIGN KEY SQL.
func (d PostgreSQLDialect) AddForeignKey(name, table string, columns []string, refTable string, refColumns []string, onDelete ForeignKeyAction, onUpdate ForeignKeyAction) (string, error) {
	return d.buildAddForeignKey(d, name, table, columns, refTable, refColumns, onDelete, onUpdate)
}

// DropForeignKey builds DROP CONSTRAINT SQL for a foreign key.
func (d PostgreSQLDialect) DropForeignKey(name, table string) (string, error) {
	return d.buildDropConstraint(d, name, table)
}

// AddCommentOnColumn builds COMMENT ON COLUMN SQL.
func (d PostgreSQLDialect) AddCommentOnColumn(table, column, comment string) (string, error) {
	return fmt.Sprintf("COMMENT ON COLUMN %s.%s IS %s", d.QuoteTable(table), d.QuoteColumn(column), sqlLiteral(comment)), nil
}

// DropCommentFromColumn builds SQL for removing a column comment.
func (d PostgreSQLDialect) DropCommentFromColumn(table, column string) (string, error) {
	return fmt.Sprintf("COMMENT ON COLUMN %s.%s IS NULL", d.QuoteTable(table), d.QuoteColumn(column)), nil
}

// AddCommentOnTable builds COMMENT ON TABLE SQL.
func (d PostgreSQLDialect) AddCommentOnTable(table, comment string) (string, error) {
	return fmt.Sprintf("COMMENT ON TABLE %s IS %s", d.QuoteTable(table), sqlLiteral(comment)), nil
}

// DropCommentFromTable builds SQL for removing a table comment.
func (d PostgreSQLDialect) DropCommentFromTable(table string) (string, error) {
	return fmt.Sprintf("COMMENT ON TABLE %s IS NULL", d.QuoteTable(table)), nil
}

// Insert builds INSERT SQL.
func (d PostgreSQLDialect) Insert(table string, row Row) (SQLStatement, error) {
	return d.buildInsert(d, table, row)
}

// BatchInsert builds multi-row INSERT SQL.
func (d PostgreSQLDialect) BatchInsert(table string, columns []string, rows [][]any) (SQLStatement, error) {
	return d.buildBatchInsert(d, table, columns, rows)
}

// Update builds UPDATE SQL.
func (d PostgreSQLDialect) Update(table string, row Row, condition string, args ...any) (SQLStatement, error) {
	return d.buildUpdate(d, table, row, condition, args...)
}

// Delete builds DELETE SQL.
func (d PostgreSQLDialect) Delete(table string, condition string, args ...any) (SQLStatement, error) {
	return d.buildDelete(d, table, condition, args...)
}

// AcquireLockSQL builds SQL for acquiring an advisory lock.
func (d PostgreSQLDialect) AcquireLockSQL(lockName string, timeoutSeconds int) (SQLStatement, error) {
	return SQLStatement{
		Query: "SELECT pg_advisory_lock(hashtext($1))",
		Args:  []any{lockName},
	}, nil
}

// ReleaseLockSQL builds SQL for releasing an advisory lock.
func (d PostgreSQLDialect) ReleaseLockSQL(lockName string) (SQLStatement, error) {
	return SQLStatement{
		Query: "SELECT pg_advisory_unlock(hashtext($1))",
		Args:  []any{lockName},
	}, nil
}
