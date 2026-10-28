package generator

import (
	"bytes"
	"errors"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/goyii2/framework/migrate"
)

// Options controls migration file generation.
type Options struct {
	Name          string
	Dir           string
	PackageName   string
	MigrateImport string
	Now           func() time.Time
	Fields        string
	// Dialect names the target dialect (mysql, sqlite, postgres, sqlserver)
	// through the migrate package registry; it shapes the primary key and
	// table options emitted in the template. Empty defaults to mysql.
	Dialect string
}

// Field describes one parsed --fields column.
type Field struct {
	Name      string
	Type      string
	Args      []string
	Modifiers []string
}

// ParseFields parses a --fields value.
func ParseFields(input string) ([]Field, error) {
	if strings.TrimSpace(input) == "" {
		return nil, nil
	}
	parts := splitTopLevel(input, ',')
	fields := make([]Field, 0, len(parts))
	for _, part := range parts {
		tokens := splitTopLevel(part, ':')
		if len(tokens) < 2 {
			return nil, fmt.Errorf("invalid field %q", part)
		}
		field := Field{Name: strings.TrimSpace(tokens[0])}
		field.Type, field.Args = parseType(strings.TrimSpace(tokens[1]))
		for _, modifier := range tokens[2:] {
			field.Modifiers = append(field.Modifiers, strings.TrimSpace(modifier))
		}
		fields = append(fields, field)
	}
	return fields, nil
}

// ColumnCode returns Go code for adding the field to a ColumnList.
func (f Field) ColumnCode() string {
	builder := "m." + builderName(f.Type) + "(" + strings.Join(f.Args, ", ") + ")"
	hasNullability := false
	for _, modifier := range f.Modifiers {
		switch {
		case modifier == "notNull":
			builder += ".NotNull()"
			hasNullability = true
		case modifier == "null":
			builder += ".Null()"
			hasNullability = true
		case modifier == "unsigned":
			builder += ".Unsigned()"
		case strings.HasPrefix(modifier, "default(") && strings.HasSuffix(modifier, ")"):
			builder += ".DefaultValue(" + defaultValueCode(modifier[len("default("):len(modifier)-1]) + ")"
		case strings.HasPrefix(modifier, "defaultExpression(") && strings.HasSuffix(modifier, ")"):
			builder += ".DefaultExpression(" + strconv.Quote(modifier[len("defaultExpression("):len(modifier)-1]) + ")"
		}
	}
	if !hasNullability {
		builder += ".Null()"
	}
	return "Add(" + strconv.Quote(f.Name) + ", " + builder + ")"
}

// dialectProfile holds the template fragments that genuinely differ between
// dialects: the primary-key builder call emitted for the implicit id column
// and the CREATE TABLE options argument. Only MySQL has a default options
// string; sqlite and sqlserver reject table options outright and postgres has
// no meaningful cross-dialect default.
type dialectProfile struct {
	primaryKey string
	options    string
}

// dialectProfiles keeps dialect-specific template knowledge next to the
// dialect name reported by the registry, so the shared template below stays
// free of MySQL-specific fragments.
var dialectProfiles = map[string]dialectProfile{
	"mysql":     {primaryKey: "m.UnsignedBigPrimaryKey()", options: `"ENGINE=InnoDB DEFAULT CHARSET=utf8mb4"`},
	"sqlite":    {primaryKey: "m.PrimaryKey()", options: `""`},
	"postgres":  {primaryKey: "m.BigPrimaryKey()", options: `""`},
	"sqlserver": {primaryKey: "m.BigPrimaryKey()", options: `""`},
}

// profileForDialect returns the template profile for dialect. Registered
// dialects without an explicit profile fall back to a portable template:
// plain auto-increment primary key and no table options.
func profileForDialect(dialect migrate.Dialect) dialectProfile {
	if profile, ok := dialectProfiles[dialect.Name()]; ok {
		return profile
	}
	return dialectProfile{primaryKey: "m.PrimaryKey()", options: `""`}
}

// normalizeOptions fills in the default values for optional fields.
func normalizeOptions(options Options) Options {
	if options.Dir == "" {
		options.Dir = "migrations"
	}
	if options.PackageName == "" {
		options.PackageName = "migrations"
	}
	if options.MigrateImport == "" {
		options.MigrateImport = "github.com/goyii2/framework/migrate"
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	if strings.TrimSpace(options.Dialect) == "" {
		options.Dialect = "mysql"
	}
	return options
}

// Generate writes a migration file and refuses to overwrite existing files.
func Generate(options Options) (string, error) {
	if options.Name == "" {
		return "", errors.New("migration name is required")
	}
	options = normalizeOptions(options)

	version := "m" + options.Now().Format("20060102_150405") + "_" + snakeName(options.Name)
	path := filepath.Join(options.Dir, version+".go")
	content, err := BuildContent(options, version)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(options.Dir, 0o755); err != nil {
		return "", err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return "", err
	}
	defer file.Close()
	if _, err := file.Write(content); err != nil {
		return "", err
	}
	return path, nil
}

// BuildContent returns formatted Go source for a migration.
func BuildContent(options Options, version string) ([]byte, error) {
	options = normalizeOptions(options)
	dialect, ok := migrate.DialectByName(options.Dialect)
	if !ok {
		return nil, fmt.Errorf("unknown dialect %q; supported values: %s", options.Dialect, strings.Join(migrate.DialectNames(), ", "))
	}
	profile := profileForDialect(dialect)
	fields, err := ParseFields(options.Fields)
	if err != nil {
		return nil, err
	}
	typeName := migrationTypeName(version)
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "package %s\n\n", options.PackageName)
	fmt.Fprintf(&buf, "import (\n\t\"context\"\n\n\t%q\n)\n\n", options.MigrateImport)
	fmt.Fprintf(&buf, "type %s struct{}\n\n", typeName)
	fmt.Fprintf(&buf, "func (%s) Name() string {\n\treturn %q\n}\n\n", typeName, version)
	fmt.Fprintf(&buf, "func (%s) Up(ctx context.Context, m *migrate.MigrationContext) error {\n", typeName)
	if len(fields) == 0 {
		fmt.Fprintf(&buf, "\treturn m.Schema().\n\t\tRaw(\"-- TODO: add migration SQL\").\n\t\tExec(ctx)\n")
	} else {
		table := tableNameFromMigration(options.Name)
		fmt.Fprintf(&buf, "\treturn m.Schema().\n\t\tCreateTable(%q, migrate.Columns().\n", table)
		fmt.Fprintf(&buf, "\t\t\tAdd(\"id\", %s).\n", profile.primaryKey)
		for i, field := range fields {
			suffix := "."
			if i == len(fields)-1 {
				suffix = ","
			}
			fmt.Fprintf(&buf, "\t\t\t%s%s\n", field.ColumnCode(), suffix)
		}
		fmt.Fprintf(&buf, "\t\t\t%s,\n\t\t).\n\t\tExec(ctx)\n", profile.options)
	}
	fmt.Fprintf(&buf, "}\n\n")
	fmt.Fprintf(&buf, "func (%s) Down(ctx context.Context, m *migrate.MigrationContext) error {\n", typeName)
	if len(fields) == 0 {
		fmt.Fprintf(&buf, "\treturn migrate.ErrIrreversibleMigration\n")
	} else {
		fmt.Fprintf(&buf, "\treturn m.Schema().\n\t\tDropTable(%q).\n\t\tExec(ctx)\n", tableNameFromMigration(options.Name))
	}
	fmt.Fprintf(&buf, "}\n")
	return format.Source(buf.Bytes())
}

func parseType(input string) (string, []string) {
	open := strings.Index(input, "(")
	if open < 0 || !strings.HasSuffix(input, ")") {
		return input, nil
	}
	args := strings.TrimSuffix(input[open+1:], ")")
	return input[:open], splitTopLevel(args, ',')
}

func splitTopLevel(input string, sep rune) []string {
	var out []string
	depth := 0
	start := 0
	for i, r := range input {
		switch r {
		case '(':
			depth++
		case ')':
			depth--
		default:
			if r == sep && depth == 0 {
				out = append(out, strings.TrimSpace(input[start:i]))
				start = i + len(string(r))
			}
		}
	}
	out = append(out, strings.TrimSpace(input[start:]))
	return out
}

func builderName(name string) string {
	special := map[string]string{"json": "Json", "uuid": "UUID"}
	if value, ok := special[name]; ok {
		return value
	}
	var out []rune
	upperNext := true
	for _, r := range name {
		if r == '_' || r == '-' {
			upperNext = true
			continue
		}
		if upperNext {
			out = append(out, unicode.ToUpper(r))
			upperNext = false
			continue
		}
		out = append(out, r)
	}
	return string(out)
}

func defaultValueCode(value string) string {
	value = strings.TrimSpace(value)
	if _, err := strconv.ParseFloat(value, 64); err == nil {
		return value
	}
	if value == "true" || value == "false" {
		return value
	}
	return strconv.Quote(strings.Trim(value, `"'`))
}

func snakeName(name string) string {
	name = strings.TrimSpace(strings.ToLower(name))
	name = strings.ReplaceAll(name, "-", "_")
	return name
}

func migrationTypeName(version string) string {
	parts := strings.Split(version, "_")
	if len(parts) >= 3 && strings.HasPrefix(parts[0], "m") {
		var out strings.Builder
		out.WriteString(builderName(parts[0]))
		out.WriteString("_")
		out.WriteString(parts[1])
		for _, part := range parts[2:] {
			out.WriteString(builderName(part))
		}
		return out.String()
	}
	var out strings.Builder
	for _, part := range parts {
		if part == "" {
			continue
		}
		out.WriteString(builderName(part))
	}
	return out.String()
}

func tableNameFromMigration(name string) string {
	name = snakeName(name)
	name = strings.TrimPrefix(name, "create_")
	name = strings.TrimSuffix(name, "_table")
	return name
}
