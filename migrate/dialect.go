package migrate

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Dialect generates SQL for a specific database engine.
type Dialect interface {
	Name() string
	QuoteTable(name string) string
	QuoteColumn(name string) string
	QuoteIndexColumn(name string) string
	Placeholder(index int) string
	BuildColumn(column *ColumnBuilder) (string, error)
	TableExistsSQL(table string) SQLStatement
	ColumnExistsSQL(table, column string) SQLStatement
	IndexExistsSQL(table, index string) SQLStatement
	ForeignKeyExistsSQL(table, name string) SQLStatement
	ConstraintExistsSQL(table, name string) SQLStatement
	BuildRowExistsSQL(table string, condition string) string
	BuildCountRowsSQL(table string, condition string) string
	CreateTable(table string, columns *ColumnList, options string) (string, error)
	CreateMigrationTableSQL(table string) (string, error)
	DropTable(table string) (string, error)
	RenameTable(oldName, newName string) (string, error)
	TruncateTable(table string) (string, error)
	AddColumn(table, column string, builder *ColumnBuilder) (string, error)
	AlterColumn(table, column string, builder *ColumnBuilder) (string, error)
	DropColumn(table, column string) (string, error)
	RenameColumn(table, oldName, newName string) (string, error)
	CreateIndex(name, table string, columns []string, unique bool) (string, error)
	DropIndex(name, table string) (string, error)
	AddPrimaryKey(name, table string, columns []string) (string, error)
	DropPrimaryKey(name, table string) (string, error)
	AddForeignKey(name, table string, columns []string, refTable string, refColumns []string, onDelete ForeignKeyAction, onUpdate ForeignKeyAction) (string, error)
	DropForeignKey(name, table string) (string, error)
	AddCommentOnColumn(table, column, comment string) (string, error)
	DropCommentFromColumn(table, column string) (string, error)
	AddCommentOnTable(table, comment string) (string, error)
	DropCommentFromTable(table string) (string, error)
	Insert(table string, row Row) (SQLStatement, error)
	BatchInsert(table string, columns []string, rows [][]any) (SQLStatement, error)
	Update(table string, row Row, condition string, args ...any) (SQLStatement, error)
	Delete(table string, condition string, args ...any) (SQLStatement, error)
	AcquireLockSQL(lockName string, timeoutSeconds int) (SQLStatement, error)
	ReleaseLockSQL(lockName string) (SQLStatement, error)
}

// ForeignKeyAction is an ON DELETE or ON UPDATE action.
type ForeignKeyAction string

const (
	// Cascade maps to CASCADE.
	Cascade ForeignKeyAction = "CASCADE"
	// Restrict maps to RESTRICT.
	Restrict ForeignKeyAction = "RESTRICT"
	// SetNull maps to SET NULL.
	SetNull ForeignKeyAction = "SET NULL"
	// NoAction maps to NO ACTION.
	NoAction ForeignKeyAction = "NO ACTION"
)

func quoteName(name, quote string) string {
	if name == "*" {
		return name
	}
	parts := strings.Split(name, ".")
	for i, part := range parts {
		parts[i] = quote + strings.ReplaceAll(part, quote, quote+quote) + quote
	}
	return strings.Join(parts, ".")
}

func quoteIndexColumn(d Dialect, name string) string {
	if strings.ContainsAny(name, "() ") || strings.Contains(name, "->") {
		return name
	}
	return d.QuoteColumn(name)
}

func columnList(d Dialect, columns []string) string {
	quoted := make([]string, 0, len(columns))
	for _, column := range columns {
		quoted = append(quoted, d.QuoteIndexColumn(column))
	}
	return strings.Join(quoted, ", ")
}

func joinOptions(options string) string {
	options = strings.TrimSpace(options)
	if options == "" {
		return ""
	}
	return " " + options
}

func sqlLiteral(v any) string {
	switch value := v.(type) {
	case nil:
		return "NULL"
	case Expression:
		return string(value)
	case string:
		return "'" + escapeSQLString(value) + "'"
	case bool:
		if value {
			return "TRUE"
		}
		return "FALSE"
	default:
		return fmt.Sprint(value)
	}
}

// escapeSQLString doubles every single quote so the value can be embedded
// inside a SQL string literal without terminating it early.
func escapeSQLString(value string) string {
	return strings.ReplaceAll(value, "'", "''")
}

func sizedType(name string, size []int, defaults ...int) string {
	if len(size) == 0 {
		if len(defaults) == 0 {
			return name
		}
		return name + "(" + strconv.Itoa(defaults[0]) + ")"
	}
	if len(size) == 1 {
		return name + "(" + strconv.Itoa(size[0]) + ")"
	}
	return name + "(" + strconv.Itoa(size[0]) + ", " + strconv.Itoa(size[1]) + ")"
}

func enumValues(values []string) string {
	quoted := make([]string, 0, len(values))
	for _, value := range values {
		quoted = append(quoted, sqlLiteral(value))
	}
	return strings.Join(quoted, ", ")
}

func sortedRowKeys(row Row) []string {
	keys := make([]string, 0, len(row))
	for key := range row {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
