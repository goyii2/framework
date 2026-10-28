package migrate

import (
	"fmt"
	"strings"
)

// MySQLDialect generates SQL for MySQL.
type MySQLDialect struct {
	baseDialect
}

// Name returns the dialect name.
func (d MySQLDialect) Name() string { return "mysql" }

// QuoteTable quotes a table name with backticks.
func (d MySQLDialect) QuoteTable(name string) string { return quoteName(name, "`") }

// QuoteColumn quotes a column name with backticks.
func (d MySQLDialect) QuoteColumn(name string) string { return quoteName(name, "`") }

// QuoteIndexColumn quotes an index column unless it is an expression.
func (d MySQLDialect) QuoteIndexColumn(name string) string { return quoteIndexColumn(d, name) }

// Placeholder returns a MySQL placeholder.
func (d MySQLDialect) Placeholder(index int) string { return "?" }

// BuildColumn builds one MySQL column definition.
func (d MySQLDialect) BuildColumn(c *ColumnBuilder) (string, error) {
	if c == nil {
		return "", fmt.Errorf("migrate: column builder is nil")
	}
	typeSQL, err := d.mysqlType(c)
	if err != nil {
		return "", err
	}
	parts := []string{typeSQL}
	if c.unsigned && supportsMySQLUnsigned(c.typeName) {
		parts = append(parts, "UNSIGNED")
	}
	if c.nullSet {
		if c.nullable {
			parts = append(parts, "NULL")
		} else {
			parts = append(parts, "NOT NULL")
		}
	}
	if c.autoIncrement {
		parts = append(parts, "AUTO_INCREMENT")
	}
	if c.primaryKey {
		parts = append(parts, "PRIMARY KEY")
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
	if c.charset != "" {
		parts = append(parts, "CHARACTER SET", c.charset)
	}
	if c.collation != "" {
		parts = append(parts, "COLLATE", c.collation)
	}
	if c.comment != "" {
		parts = append(parts, "COMMENT", sqlLiteral(c.comment))
	}
	if c.first {
		parts = append(parts, "FIRST")
	}
	if c.after != "" {
		parts = append(parts, "AFTER", d.QuoteColumn(c.after))
	}
	if c.appendSQL != "" {
		parts = append(parts, c.appendSQL)
	}
	return strings.Join(parts, " "), nil
}

// mysqlType maps a column builder onto its MySQL type name. A type without a
// mapping is rejected with an unsupported error instead of being spliced into
// the SQL verbatim.
func (d MySQLDialect) mysqlType(c *ColumnBuilder) (string, error) {
	switch c.typeName {
	case "tinyInteger":
		return sizedType("TINYINT", c.size), nil
	case "smallInteger":
		return sizedType("SMALLINT", c.size), nil
	case "integer":
		return sizedType("INT", c.size), nil
	case "bigInteger":
		return sizedType("BIGINT", c.size), nil
	case "string":
		return sizedType("VARCHAR", c.size, 255), nil
	case "char":
		return sizedType("CHAR", c.size), nil
	case "text":
		return "TEXT", nil
	case "tinyText":
		return "TINYTEXT", nil
	case "mediumText":
		return "MEDIUMTEXT", nil
	case "longText":
		return "LONGTEXT", nil
	case "binary":
		return sizedType("VARBINARY", c.size), nil
	case "tinyBlob":
		return "TINYBLOB", nil
	case "mediumBlob":
		return "MEDIUMBLOB", nil
	case "longBlob":
		return "LONGBLOB", nil
	case "boolean":
		return "TINYINT(1)", nil
	case "float":
		return sizedType("FLOAT", c.size), nil
	case "double":
		return sizedType("DOUBLE", c.size), nil
	case "decimal", "money":
		return sizedType("DECIMAL", c.size), nil
	case "date":
		return "DATE", nil
	case "dateTime":
		return precisionType("DATETIME", c.size), nil
	case "time":
		return precisionType("TIME", c.size), nil
	case "timestamp":
		return precisionType("TIMESTAMP", c.size), nil
	case "json":
		return "JSON", nil
	case "uuid":
		return "CHAR(36)", nil
	case "enum":
		return "ENUM(" + enumValues(c.values) + ")", nil
	case "set":
		return "SET(" + enumValues(c.values) + ")", nil
	default:
		return "", unsupported(d.Name(), c.typeName)
	}
}

func precisionType(name string, size []int) string {
	if len(size) == 0 {
		return name
	}
	return sizedType(name, size)
}

func supportsMySQLUnsigned(typeName string) bool {
	switch typeName {
	case "tinyInteger", "smallInteger", "integer", "bigInteger", "float", "double", "decimal", "money":
		return true
	default:
		return false
	}
}

// TableExistsSQL returns SQL checking for a table.
func (d MySQLDialect) TableExistsSQL(table string) SQLStatement {
	return SQLStatement{Query: "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = ?", Args: []any{table}}
}

// ColumnExistsSQL returns SQL checking for a column.
func (d MySQLDialect) ColumnExistsSQL(table, column string) SQLStatement {
	return SQLStatement{Query: "SELECT COUNT(*) FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = ? AND column_name = ?", Args: []any{table, column}}
}

// IndexExistsSQL returns SQL checking for an index.
func (d MySQLDialect) IndexExistsSQL(table, index string) SQLStatement {
	return SQLStatement{Query: "SELECT COUNT(*) FROM information_schema.statistics WHERE table_schema = DATABASE() AND table_name = ? AND index_name = ?", Args: []any{table, index}}
}

// ForeignKeyExistsSQL returns SQL checking for a foreign key.
func (d MySQLDialect) ForeignKeyExistsSQL(table, name string) SQLStatement {
	return SQLStatement{Query: "SELECT COUNT(*) FROM information_schema.table_constraints WHERE table_schema = DATABASE() AND table_name = ? AND constraint_name = ? AND constraint_type = 'FOREIGN KEY'", Args: []any{table, name}}
}

// ConstraintExistsSQL returns SQL checking for a constraint.
func (d MySQLDialect) ConstraintExistsSQL(table, name string) SQLStatement {
	return SQLStatement{Query: "SELECT COUNT(*) FROM information_schema.table_constraints WHERE table_schema = DATABASE() AND table_name = ? AND constraint_name = ?", Args: []any{table, name}}
}

// BuildRowExistsSQL returns SQL for checking if a row exists.
func (d MySQLDialect) BuildRowExistsSQL(table string, condition string) string {
	return d.buildRowExistsSQL(d, table, condition)
}

// BuildCountRowsSQL returns SQL for counting rows.
func (d MySQLDialect) BuildCountRowsSQL(table string, condition string) string {
	return d.buildCountRowsSQL(d, table, condition)
}

// CreateTable builds CREATE TABLE SQL with table options.
func (d MySQLDialect) CreateTable(table string, columns *ColumnList, options string) (string, error) {
	return d.buildCreateTable(d, table, columns, options, true)
}

// CreateMigrationTableSQL builds idempotent SQL that creates the migration
// table (version varchar(180) primary key, apply_time int) when it is absent.
func (d MySQLDialect) CreateMigrationTableSQL(table string) (string, error) {
	return d.buildCreateMigrationTableSQL(d, table)
}

// DropTable builds a strict DROP TABLE; idempotent drops belong to the plan
// layer's existence probes.
func (d MySQLDialect) DropTable(table string) (string, error) {
	return d.buildDropTable(d, table)
}

// RenameTable builds RENAME TABLE SQL.
func (d MySQLDialect) RenameTable(oldName, newName string) (string, error) {
	return fmt.Sprintf("RENAME TABLE %s TO %s", d.QuoteTable(oldName), d.QuoteTable(newName)), nil
}

// TruncateTable builds TRUNCATE TABLE SQL.
func (d MySQLDialect) TruncateTable(table string) (string, error) {
	return d.buildTruncateTable(d, table)
}

// AddColumn builds ADD COLUMN SQL.
func (d MySQLDialect) AddColumn(table, column string, builder *ColumnBuilder) (string, error) {
	return d.buildAddColumn(d, table, column, builder)
}

// AlterColumn builds ALTER COLUMN SQL.
func (d MySQLDialect) AlterColumn(table, column string, builder *ColumnBuilder) (string, error) {
	columnSQL, err := d.BuildColumn(builder)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("ALTER TABLE %s MODIFY COLUMN %s %s", d.QuoteTable(table), d.QuoteColumn(column), columnSQL), nil
}

// DropColumn builds DROP COLUMN SQL.
func (d MySQLDialect) DropColumn(table, column string) (string, error) {
	return d.buildDropColumn(d, table, column)
}

// RenameColumn builds RENAME COLUMN SQL.
func (d MySQLDialect) RenameColumn(table, oldName, newName string) (string, error) {
	return d.buildRenameColumn(d, table, oldName, newName)
}

// CreateIndex builds CREATE INDEX SQL.
func (d MySQLDialect) CreateIndex(name, table string, columns []string, unique bool) (string, error) {
	return d.buildCreateIndex(d, name, table, columns, unique)
}

// DropIndex builds a strict DROP INDEX that keeps the table clause, because
// MySQL requires ON table to locate the index.
func (d MySQLDialect) DropIndex(name, table string) (string, error) {
	return d.buildDropIndex(d, name, table)
}

// AddPrimaryKey builds ADD PRIMARY KEY SQL.
func (d MySQLDialect) AddPrimaryKey(name, table string, columns []string) (string, error) {
	return d.buildAddPrimaryKey(d, name, table, columns)
}

// DropPrimaryKey builds DROP PRIMARY KEY SQL.
func (d MySQLDialect) DropPrimaryKey(name, table string) (string, error) {
	return "ALTER TABLE " + d.QuoteTable(table) + " DROP PRIMARY KEY", nil
}

// AddForeignKey builds ADD FOREIGN KEY SQL.
func (d MySQLDialect) AddForeignKey(name, table string, columns []string, refTable string, refColumns []string, onDelete ForeignKeyAction, onUpdate ForeignKeyAction) (string, error) {
	return d.buildAddForeignKey(d, name, table, columns, refTable, refColumns, onDelete, onUpdate)
}

// DropForeignKey builds DROP FOREIGN KEY SQL.
func (d MySQLDialect) DropForeignKey(name, table string) (string, error) {
	return fmt.Sprintf("ALTER TABLE %s DROP FOREIGN KEY %s", d.QuoteTable(table), d.QuoteTable(name)), nil
}

// AddCommentOnColumn builds column comment SQL.
func (d MySQLDialect) AddCommentOnColumn(table, column, comment string) (string, error) {
	return fmt.Sprintf("ALTER TABLE %s MODIFY COLUMN %s COMMENT %s", d.QuoteTable(table), d.QuoteColumn(column), sqlLiteral(comment)), nil
}

// DropCommentFromColumn builds SQL for dropping a column comment.
func (d MySQLDialect) DropCommentFromColumn(table, column string) (string, error) {
	return d.AddCommentOnColumn(table, column, "")
}

// AddCommentOnTable builds table comment SQL.
func (d MySQLDialect) AddCommentOnTable(table, comment string) (string, error) {
	return fmt.Sprintf("ALTER TABLE %s COMMENT = %s", d.QuoteTable(table), sqlLiteral(comment)), nil
}

// DropCommentFromTable builds SQL for dropping a table comment.
func (d MySQLDialect) DropCommentFromTable(table string) (string, error) {
	return d.AddCommentOnTable(table, "")
}

// Insert builds INSERT SQL.
func (d MySQLDialect) Insert(table string, row Row) (SQLStatement, error) {
	return d.buildInsert(d, table, row)
}

// BatchInsert builds multi-row INSERT SQL.
func (d MySQLDialect) BatchInsert(table string, columns []string, rows [][]any) (SQLStatement, error) {
	return d.buildBatchInsert(d, table, columns, rows)
}

// Update builds UPDATE SQL.
func (d MySQLDialect) Update(table string, row Row, condition string, args ...any) (SQLStatement, error) {
	return d.buildUpdate(d, table, row, condition, args...)
}

// Delete builds DELETE SQL.
func (d MySQLDialect) Delete(table string, condition string, args ...any) (SQLStatement, error) {
	return d.buildDelete(d, table, condition, args...)
}

// AcquireLockSQL builds SQL for acquiring an advisory migration lock.
func (d MySQLDialect) AcquireLockSQL(lockName string, timeoutSeconds int) (SQLStatement, error) {
	return SQLStatement{Query: "SELECT GET_LOCK(?, ?)", Args: []any{lockName, timeoutSeconds}}, nil
}

// ReleaseLockSQL builds SQL for releasing an advisory migration lock.
func (d MySQLDialect) ReleaseLockSQL(lockName string) (SQLStatement, error) {
	return SQLStatement{Query: "SELECT RELEASE_LOCK(?)", Args: []any{lockName}}, nil
}
