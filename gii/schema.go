package gii

import (
	"fmt"
	"strings"

	"gorm.io/gorm"
)

// Column is one introspected table column: the facts GORM's migrator
// reported plus the Go type the generator maps them to. Everything the
// templates render derives from these fields, so the mapping decisions live
// here once instead of inside every template.
type Column struct {
	// Name is the column name as declared in the table.
	Name string
	// DBType is the lowercased database type name ("varchar", "bigint",
	// "datetime", ...).
	DBType string
	// GoType is the Go type the column maps to ("string", "int64",
	// "time.Time", "[]byte"), without the pointer nullable columns add.
	GoType string
	// Unmapped marks a database type with no known mapping; the Go field
	// falls back to string and carries a TODO comment.
	Unmapped bool
	// Nullable reports whether the column accepts NULL; nullable columns
	// become pointer fields.
	Nullable bool
	// Length is the declared length of variable-length types; HasLength
	// reports whether the database reported one.
	Length    int64
	HasLength bool
	// HasDefault reports a declared default value; DefaultValue is its text.
	HasDefault   bool
	DefaultValue string
	// PrimaryKey marks the table's primary key column.
	PrimaryKey bool
	// AutoIncrement marks an auto-incrementing column.
	AutoIncrement bool
	// Unique marks a column under a single-column unique index or
	// constraint. Best effort: drivers differ in what they report, so the
	// Rules skeleton treats it as a hint, never a guarantee.
	Unique bool
}

// Kind classifies a column for code generation: which input type a form
// field renders and how submitted values parse back into the model.
type Kind string

const (
	KindString Kind = "string"
	KindInt    Kind = "int"
	KindFloat  Kind = "float"
	KindBool   Kind = "bool"
	KindTime   Kind = "time"
	KindBytes  Kind = "bytes"
)

// Kind reports the generation class of the column. Unmapped columns
// classify as strings — string is the type generated for them.
func (c Column) Kind() Kind {
	switch c.GoType {
	case "int", "int8", "int16", "int32", "int64":
		return KindInt
	case "float32", "float64":
		return KindFloat
	case "bool":
		return KindBool
	case "time.Time":
		return KindTime
	case "[]byte":
		return KindBytes
	default:
		return KindString
	}
}

// Pointer reports whether generated Go fields for the column are pointers:
// nullable columns only; binary columns never are ([]byte's nil already
// means NULL) and neither are primary keys — the Active Record helpers
// judge insert vs update by the key's zero value, which a pointer would
// obscure.
func (c Column) Pointer() bool {
	return c.Nullable && !c.PrimaryKey && c.Kind() != KindBytes
}

// goType is the column's Go type with the pointer applied.
func (c Column) goType() string {
	if c.Pointer() {
		return "*" + c.GoType
	}
	return c.GoType
}

// Table is one introspected table.
type Table struct {
	// Name is the table name.
	Name string
	// Columns lists the table's columns in declaration order.
	Columns []Column
}

// PrimaryKey returns the table's primary key column, or nil when the table
// has none. The CRUD generator refuses tables without a single-column key
// (the Active Record helpers FindByID and Save's insert/update split are
// built around exactly one); the model generator works without.
func (t Table) PrimaryKey() *Column {
	for i := range t.Columns {
		if t.Columns[i].PrimaryKey {
			return &t.Columns[i]
		}
	}
	return nil
}

// Introspect reads table's structure through GORM's migrator: ColumnTypes
// for the columns, GetIndexes for unique single-column indexes (unique
// constraints report through the column itself). conn is any GORM handle the
// table lives in — typically the application's db component.
func Introspect(conn *gorm.DB, table string) (*Table, error) {
	migrator := conn.Migrator()
	if !migrator.HasTable(table) {
		return nil, fmt.Errorf("gii: table %q does not exist", table)
	}
	types, err := migrator.ColumnTypes(table)
	if err != nil {
		return nil, fmt.Errorf("gii: read columns of %q: %w", table, err)
	}
	unique := uniqueIndexedColumns(migrator, table)

	t := &Table{Name: table}
	for _, ct := range types {
		column := Column{Name: ct.Name(), DBType: strings.ToLower(ct.DatabaseTypeName())}
		if nullable, ok := ct.Nullable(); ok {
			column.Nullable = nullable
		}
		if length, ok := ct.Length(); ok {
			column.Length, column.HasLength = length, true
		}
		if value, ok := ct.DefaultValue(); ok {
			column.DefaultValue, column.HasDefault = value, true
		}
		if pk, ok := ct.PrimaryKey(); ok {
			column.PrimaryKey = pk
		}
		if auto, ok := ct.AutoIncrement(); ok {
			column.AutoIncrement = auto
		}
		if unique, ok := ct.Unique(); ok {
			column.Unique = unique
		}
		column.Unique = column.Unique || unique[column.Name]
		column.GoType, column.Unmapped = mapGoType(column.DBType, column.Length, column.HasLength)
		t.Columns = append(t.Columns, column)
	}
	return t, nil
}

// uniqueIndexedColumns collects the columns covered by a single-column
// unique index. Failing to read indexes is not fatal — drivers support
// GetIndexes unevenly ("not support" errors), and uniqueness is only a hint
// for the Rules skeleton.
func uniqueIndexedColumns(migrator gorm.Migrator, table string) map[string]bool {
	indexes, err := migrator.GetIndexes(table)
	if err != nil {
		return nil
	}
	unique := make(map[string]bool)
	for _, index := range indexes {
		if len(index.Columns()) != 1 {
			continue
		}
		if isUnique, ok := index.Unique(); ok && isUnique {
			unique[index.Columns()[0]] = true
		}
	}
	return unique
}

// goTypeByDBType maps lowercased database type names to Go types. Length
// context only matters where the same name means different things (a
// one-bit tinyint is a boolean in MySQL convention).
var goTypeByDBType = map[string]string{
	"bigint":      "int64",
	"bigserial":   "int64",
	"int8":        "int64",
	"int":         "int",
	"integer":     "int",
	"int4":        "int",
	"serial":      "int",
	"mediumint":   "int",
	"smallint":    "int16",
	"smallserial": "int16",
	"int2":        "int16",
	"bool":        "bool",
	"boolean":     "bool",
	// The float family maps to float64 across the board: SQLite's REAL is
	// 64-bit, MySQL's FLOAT and Postgres' real scan losslessly into it, and
	// no driver names a type that is unambiguously 32-bit.
	"float":                       "float64",
	"real":                        "float64",
	"float4":                      "float64",
	"double":                      "float64",
	"float8":                      "float64",
	"double precision":            "float64",
	"decimal":                     "float64",
	"numeric":                     "float64",
	"dec":                         "float64",
	"money":                       "float64",
	"varchar":                     "string",
	"character varying":           "string",
	"nvarchar":                    "string",
	"char":                        "string",
	"character":                   "string",
	"nchar":                       "string",
	"string":                      "string",
	"text":                        "string",
	"tinytext":                    "string",
	"mediumtext":                  "string",
	"longtext":                    "string",
	"clob":                        "string",
	"date":                        "time.Time",
	"datetime":                    "time.Time",
	"timestamp":                   "time.Time",
	"timestamptz":                 "time.Time",
	"timestamp with time zone":    "time.Time",
	"timestamp without time zone": "time.Time",
	"time":                        "time.Time",
	"year":                        "time.Time",
	"blob":                        "[]byte",
	"binary":                      "[]byte",
	"varbinary":                   "[]byte",
	"bytea":                       "[]byte",
}

// mapGoType resolves a column's Go type from its database type. Unknown
// types map to string with unmapped set, so the generated field compiles
// and the TODO comment flags the spot to adjust by hand.
func mapGoType(dbType string, length int64, hasLength bool) (string, bool) {
	if dbType == "tinyint" || dbType == "bit" {
		// A one-bit/tinyint(1) column is a boolean by MySQL convention;
		// anything wider stays an integer.
		if hasLength && length == 1 {
			return "bool", false
		}
		if dbType == "bit" {
			return "string", true
		}
		return "int8", false
	}
	if got, ok := goTypeByDBType[dbType]; ok {
		return got, false
	}
	return "string", true
}
