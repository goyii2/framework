package migrate

import (
	"fmt"
	"strconv"
	"strings"
)

// SQLServerDialect generates SQL for Microsoft SQL Server.
type SQLServerDialect struct {
	baseDialect
}

// Name returns the dialect name.
func (d SQLServerDialect) Name() string { return "sqlserver" }

func sqlserverQuoteName(name string) string {
	if name == "*" {
		return name
	}
	parts := strings.Split(name, ".")
	for i, part := range parts {
		parts[i] = "[" + strings.ReplaceAll(part, "]", "]]") + "]"
	}
	return strings.Join(parts, ".")
}

// QuoteTable quotes a table name with square brackets.
func (d SQLServerDialect) QuoteTable(name string) string { return sqlserverQuoteName(name) }

// QuoteColumn quotes a column name with square brackets.
func (d SQLServerDialect) QuoteColumn(name string) string { return sqlserverQuoteName(name) }

// QuoteIndexColumn quotes an index column unless it is an expression.
func (d SQLServerDialect) QuoteIndexColumn(name string) string {
	return quoteIndexColumn(d, name)
}

// Placeholder returns a SQL Server parameter placeholder.
func (d SQLServerDialect) Placeholder(index int) string {
	return "@p" + strconv.Itoa(index)
}

// BuildColumn builds one SQL Server column definition.
func (d SQLServerDialect) BuildColumn(c *ColumnBuilder) (string, error) {
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
	if c.generatedAs != "" {
		return "", unsupported(d.Name(), "GENERATED AS")
	}

	var typeSQL string
	if identity := mssqlIdentityType(c); identity != "" {
		typeSQL = identity
	} else {
		var err error
		typeSQL, err = d.mssqlType(c)
		if err != nil {
			return "", err
		}
	}

	parts := []string{typeSQL}
	if c.nullSet {
		if c.nullable {
			parts = append(parts, "NULL")
		} else {
			parts = append(parts, "NOT NULL")
		}
	}
	if c.primaryKey && !isIdentityColumn(c) {
		parts = append(parts, "PRIMARY KEY")
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
	if c.unique {
		parts = append(parts, "UNIQUE")
	}
	if c.appendSQL != "" {
		parts = append(parts, c.appendSQL)
	}
	return strings.Join(parts, " "), nil
}

func isIdentityColumn(c *ColumnBuilder) bool {
	return c.primaryKey && c.autoIncrement && (c.typeName == "integer" || c.typeName == "bigInteger")
}

func mssqlIdentityType(c *ColumnBuilder) string {
	if c.primaryKey && c.autoIncrement {
		switch c.typeName {
		case "integer":
			return "INT IDENTITY(1,1) PRIMARY KEY"
		case "bigInteger":
			return "BIGINT IDENTITY(1,1) PRIMARY KEY"
		}
	}
	return ""
}

// mssqlType maps a column builder onto its SQL Server type name. A type
// without a mapping - including enum and set, which SQL Server cannot
// represent - is rejected with an unsupported error instead of producing an
// empty definition.
func (d SQLServerDialect) mssqlType(c *ColumnBuilder) (string, error) {
	switch c.typeName {
	case "tinyInteger":
		return "TINYINT", nil
	case "smallInteger":
		return "SMALLINT", nil
	case "integer":
		return "INT", nil
	case "bigInteger":
		return "BIGINT", nil
	case "string":
		return sizedType("NVARCHAR", c.size, 255), nil
	case "char":
		return sizedType("NCHAR", c.size), nil
	case "text", "tinyText", "mediumText", "longText":
		return "NVARCHAR(MAX)", nil
	case "binary":
		return sizedType("VARBINARY", c.size), nil
	case "tinyBlob", "mediumBlob", "longBlob":
		return "VARBINARY(MAX)", nil
	case "boolean":
		return "BIT", nil
	case "float":
		return sizedType("FLOAT", c.size), nil
	case "double":
		return "DOUBLE PRECISION", nil
	case "decimal":
		return sizedType("DECIMAL", c.size), nil
	case "money":
		return "MONEY", nil
	case "date":
		return "DATE", nil
	case "dateTime":
		return mssqlPrecisionType("DATETIME2", c.size), nil
	case "time":
		return mssqlPrecisionType("TIME", c.size), nil
	case "timestamp":
		return mssqlPrecisionType("DATETIME2", c.size), nil
	case "json":
		return "NVARCHAR(MAX)", nil
	case "uuid":
		return "UNIQUEIDENTIFIER", nil
	default:
		return "", unsupported(d.Name(), c.typeName)
	}
}

func mssqlPrecisionType(name string, size []int) string {
	if len(size) == 0 {
		return name
	}
	return sizedType(name, size)
}

// TableExistsSQL returns SQL checking for a table.
func (d SQLServerDialect) TableExistsSQL(table string) SQLStatement {
	return SQLStatement{
		Query: "SELECT COUNT(*) FROM sys.tables WHERE SCHEMA_NAME(schema_id) = SCHEMA_NAME() AND name = @p1",
		Args:  []any{table},
	}
}

// ColumnExistsSQL returns SQL checking for a column.
func (d SQLServerDialect) ColumnExistsSQL(table, column string) SQLStatement {
	return SQLStatement{
		Query: "SELECT COUNT(*) FROM sys.columns c JOIN sys.tables t ON c.object_id = t.object_id WHERE SCHEMA_NAME(t.schema_id) = SCHEMA_NAME() AND t.name = @p1 AND c.name = @p2",
		Args:  []any{table, column},
	}
}

// IndexExistsSQL returns SQL checking for an index.
func (d SQLServerDialect) IndexExistsSQL(table, index string) SQLStatement {
	return SQLStatement{
		Query: "SELECT COUNT(*) FROM sys.indexes i JOIN sys.tables t ON i.object_id = t.object_id WHERE SCHEMA_NAME(t.schema_id) = SCHEMA_NAME() AND t.name = @p1 AND i.name = @p2",
		Args:  []any{table, index},
	}
}

// ForeignKeyExistsSQL returns SQL checking for a foreign key.
func (d SQLServerDialect) ForeignKeyExistsSQL(table, name string) SQLStatement {
	return SQLStatement{
		Query: "SELECT COUNT(*) FROM sys.foreign_keys WHERE SCHEMA_NAME(schema_id) = SCHEMA_NAME() AND OBJECT_NAME(parent_object_id) = @p1 AND name = @p2",
		Args:  []any{table, name},
	}
}

// ConstraintExistsSQL returns SQL checking for a constraint.
func (d SQLServerDialect) ConstraintExistsSQL(table, name string) SQLStatement {
	return SQLStatement{
		Query: "SELECT COUNT(*) FROM sys.check_constraints cc JOIN sys.tables t ON cc.parent_object_id = t.object_id WHERE SCHEMA_NAME(t.schema_id) = SCHEMA_NAME() AND t.name = @p1 AND cc.name = @p2",
		Args:  []any{table, name},
	}
}

// BuildRowExistsSQL returns SQL for checking if a row exists as a CASE
// expression, because SQL Server has no bare EXISTS select.
func (d SQLServerDialect) BuildRowExistsSQL(table string, condition string) string {
	query := "SELECT CASE WHEN EXISTS(SELECT 1 FROM " + d.QuoteTable(table)
	if strings.TrimSpace(condition) != "" {
		query += " WHERE " + condition
	}
	return query + ") THEN 1 ELSE 0 END"
}

// BuildCountRowsSQL returns SQL for counting rows.
func (d SQLServerDialect) BuildCountRowsSQL(table string, condition string) string {
	return d.buildCountRowsSQL(d, table, condition)
}

// CreateTable builds CREATE TABLE SQL; SQL Server cannot take table options.
func (d SQLServerDialect) CreateTable(table string, columns *ColumnList, options string) (string, error) {
	return d.buildCreateTable(d, table, columns, options, false)
}

// CreateMigrationTableSQL builds idempotent SQL that creates the migration
// table (version varchar(180) primary key, apply_time int) when it is absent.
// SQL Server has no CREATE TABLE IF NOT EXISTS, so existence is checked via
// OBJECT_ID before creating the table; the quoted name embedded in that
// string literal has its single quotes doubled.
func (d SQLServerDialect) CreateMigrationTableSQL(table string) (string, error) {
	quoted := d.QuoteTable(table)
	return fmt.Sprintf(
		"IF OBJECT_ID(N'%s', 'U') IS NULL BEGIN CREATE TABLE %s (%s varchar(180) NOT NULL PRIMARY KEY, %s int NOT NULL) END",
		escapeSQLString(quoted), quoted, d.QuoteColumn("version"), d.QuoteColumn("apply_time"),
	), nil
}

// DropTable builds a strict DROP TABLE; SQL Server has no IF EXISTS form, and
// idempotent drops belong to the plan layer's existence probes anyway.
func (d SQLServerDialect) DropTable(table string) (string, error) {
	return d.buildDropTable(d, table)
}

// RenameTable builds sp_rename SQL. Object names are embedded in a string
// literal, so single quotes are doubled.
func (d SQLServerDialect) RenameTable(oldName, newName string) (string, error) {
	return fmt.Sprintf("EXEC sp_rename '%s', '%s'", escapeSQLString(oldName), escapeSQLString(newName)), nil
}

// TruncateTable builds TRUNCATE TABLE SQL.
func (d SQLServerDialect) TruncateTable(table string) (string, error) {
	return d.buildTruncateTable(d, table)
}

// AddColumn builds ADD SQL without the COLUMN keyword.
func (d SQLServerDialect) AddColumn(table, column string, builder *ColumnBuilder) (string, error) {
	columnSQL, err := d.BuildColumn(builder)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("ALTER TABLE %s ADD %s %s", d.QuoteTable(table), d.QuoteColumn(column), columnSQL), nil
}

// AlterColumn builds ALTER COLUMN SQL.
func (d SQLServerDialect) AlterColumn(table, column string, builder *ColumnBuilder) (string, error) {
	columnSQL, err := d.BuildColumn(builder)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s %s", d.QuoteTable(table), d.QuoteColumn(column), columnSQL), nil
}

// DropColumn builds DROP COLUMN SQL.
func (d SQLServerDialect) DropColumn(table, column string) (string, error) {
	return d.buildDropColumn(d, table, column)
}

// RenameColumn builds sp_rename SQL for a column. The qualified name is
// embedded in a string literal, so single quotes are doubled.
func (d SQLServerDialect) RenameColumn(table, oldName, newName string) (string, error) {
	return fmt.Sprintf("EXEC sp_rename '%s.%s', '%s', 'COLUMN'", escapeSQLString(table), escapeSQLString(oldName), escapeSQLString(newName)), nil
}

// CreateIndex builds CREATE INDEX SQL.
func (d SQLServerDialect) CreateIndex(name, table string, columns []string, unique bool) (string, error) {
	return d.buildCreateIndex(d, name, table, columns, unique)
}

// DropIndex builds DROP INDEX SQL with ON table clause.
func (d SQLServerDialect) DropIndex(name, table string) (string, error) {
	return d.buildDropIndex(d, name, table)
}

// AddPrimaryKey builds ADD CONSTRAINT PRIMARY KEY SQL.
func (d SQLServerDialect) AddPrimaryKey(name, table string, columns []string) (string, error) {
	return d.buildAddPrimaryKey(d, name, table, columns)
}

// DropPrimaryKey builds DROP CONSTRAINT SQL.
func (d SQLServerDialect) DropPrimaryKey(name, table string) (string, error) {
	return d.buildDropConstraint(d, name, table)
}

// AddForeignKey builds ADD CONSTRAINT FOREIGN KEY SQL.
func (d SQLServerDialect) AddForeignKey(name, table string, columns []string, refTable string, refColumns []string, onDelete ForeignKeyAction, onUpdate ForeignKeyAction) (string, error) {
	return d.buildAddForeignKey(d, name, table, columns, refTable, refColumns, onDelete, onUpdate)
}

// DropForeignKey builds DROP CONSTRAINT SQL for a foreign key.
func (d SQLServerDialect) DropForeignKey(name, table string) (string, error) {
	return d.buildDropConstraint(d, name, table)
}

// AddCommentOnColumn builds sp_addextendedproperty SQL for a column comment.
// Object names are embedded in string literals, so single quotes are doubled.
func (d SQLServerDialect) AddCommentOnColumn(table, column, comment string) (string, error) {
	return fmt.Sprintf("EXEC sp_addextendedproperty 'MS_Description', %s, 'SCHEMA', SCHEMA_NAME(), 'TABLE', '%s', 'COLUMN', '%s'", sqlLiteral(comment), escapeSQLString(table), escapeSQLString(column)), nil
}

// DropCommentFromColumn builds sp_dropextendedproperty SQL for a column
// comment; single quotes in object names are doubled.
func (d SQLServerDialect) DropCommentFromColumn(table, column string) (string, error) {
	return fmt.Sprintf("EXEC sp_dropextendedproperty 'MS_Description', 'SCHEMA', SCHEMA_NAME(), 'TABLE', '%s', 'COLUMN', '%s'", escapeSQLString(table), escapeSQLString(column)), nil
}

// AddCommentOnTable builds sp_addextendedproperty SQL for a table comment;
// single quotes in the table name are doubled.
func (d SQLServerDialect) AddCommentOnTable(table, comment string) (string, error) {
	return fmt.Sprintf("EXEC sp_addextendedproperty 'MS_Description', %s, 'SCHEMA', SCHEMA_NAME(), 'TABLE', '%s'", sqlLiteral(comment), escapeSQLString(table)), nil
}

// DropCommentFromTable builds sp_dropextendedproperty SQL for a table
// comment; single quotes in the table name are doubled.
func (d SQLServerDialect) DropCommentFromTable(table string) (string, error) {
	return fmt.Sprintf("EXEC sp_dropextendedproperty 'MS_Description', 'SCHEMA', SCHEMA_NAME(), 'TABLE', '%s'", escapeSQLString(table)), nil
}

// Insert builds INSERT SQL.
func (d SQLServerDialect) Insert(table string, row Row) (SQLStatement, error) {
	return d.buildInsert(d, table, row)
}

// BatchInsert builds multi-row INSERT SQL.
func (d SQLServerDialect) BatchInsert(table string, columns []string, rows [][]any) (SQLStatement, error) {
	return d.buildBatchInsert(d, table, columns, rows)
}

// Update builds UPDATE SQL.
func (d SQLServerDialect) Update(table string, row Row, condition string, args ...any) (SQLStatement, error) {
	return d.buildUpdate(d, table, row, condition, args...)
}

// Delete builds DELETE SQL.
func (d SQLServerDialect) Delete(table string, condition string, args ...any) (SQLStatement, error) {
	return d.buildDelete(d, table, condition, args...)
}

// AcquireLockSQL builds SQL for acquiring an application lock.
func (d SQLServerDialect) AcquireLockSQL(lockName string, timeoutSeconds int) (SQLStatement, error) {
	return SQLStatement{
		Query: "DECLARE @result INT; EXEC @result = sp_getapplock @Resource = @p1, @LockMode = 'Exclusive', @LockTimeout = @p2; SELECT @result",
		Args:  []any{lockName, timeoutSeconds},
	}, nil
}

// ReleaseLockSQL builds SQL for releasing an application lock.
func (d SQLServerDialect) ReleaseLockSQL(lockName string) (SQLStatement, error) {
	return SQLStatement{
		Query: "DECLARE @result INT; EXEC @result = sp_releaseapplock @Resource = @p1; SELECT @result",
		Args:  []any{lockName},
	}, nil
}
