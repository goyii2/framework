package generator

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/goyii2/framework/migrate"
)

func TestParseFieldsBuildsColumnCode(t *testing.T) {
	fields, err := ParseFields("user_id:unsignedBigInteger:notNull,title:string(128):notNull,content:longText,metadata:json,status:unsignedTinyInteger:notNull:default(10)")
	if err != nil {
		t.Fatalf("ParseFields() error = %v", err)
	}

	got := make([]string, 0, len(fields))
	for _, field := range fields {
		got = append(got, field.ColumnCode())
	}
	want := []string{
		`Add("user_id", m.UnsignedBigInteger().NotNull())`,
		`Add("title", m.String(128).NotNull())`,
		`Add("content", m.LongText().Null())`,
		`Add("metadata", m.Json().Null())`,
		`Add("status", m.UnsignedTinyInteger().NotNull().DefaultValue(10))`,
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("ColumnCode() = %#v, want %#v", got, want)
	}
}

func TestGenerateMigrationWritesFileAndRefusesOverwrite(t *testing.T) {
	dir := t.TempDir()
	opts := Options{
		Name:          "create_article_table",
		Dir:           dir,
		PackageName:   "migrations",
		MigrateImport: "github.com/goyii2/framework/migrate",
		Now:           func() time.Time { return time.Date(2026, 6, 13, 10, 30, 0, 0, time.UTC) },
		Fields:        "title:string(128):notNull,content:longText",
	}

	path, err := Generate(opts)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if filepath.Base(path) != "m20260613_103000_create_article_table.go" {
		t.Fatalf("file name = %q", filepath.Base(path))
	}
	contentBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	content := string(contentBytes)
	for _, want := range []string{
		"type M20260613_103000CreateArticleTable struct{}",
		`return "m20260613_103000_create_article_table"`,
		`CreateTable("article", migrate.Columns().`,
		`Add("id", m.UnsignedBigPrimaryKey()).`,
		`Add("title", m.String(128).NotNull()).`,
		`Add("content", m.LongText().Null()),`,
		`DropTable("article").`,
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("generated content missing %q:\n%s", want, content)
		}
	}

	if _, err := Generate(opts); err == nil {
		t.Fatalf("Generate() overwrote existing file")
	}
}

// primaryKeyLine matches the implicit id column the template emits and
// captures the primary-key builder call it uses.
var primaryKeyLine = regexp.MustCompile(`Add\("id", (m\.\w+\(\))\)\.`)

// optionsLine matches the CREATE TABLE options argument line, which is the
// only template line at that indentation starting with a string literal.
var optionsLine = regexp.MustCompile(`(?m)^\t\t\t("[^"]*"),$`)

// primaryKeyBuilders maps a primary-key builder call as written in templates
// to the real migrate builder it names, so generated templates can be checked
// against dialect expectations instead of trusting the code text.
var primaryKeyBuilders = map[string]func(*migrate.MigrationContext) *migrate.ColumnBuilder{
	"m.PrimaryKey()":            func(m *migrate.MigrationContext) *migrate.ColumnBuilder { return m.PrimaryKey() },
	"m.BigPrimaryKey()":         func(m *migrate.MigrationContext) *migrate.ColumnBuilder { return m.BigPrimaryKey() },
	"m.UnsignedBigPrimaryKey()": func(m *migrate.MigrationContext) *migrate.ColumnBuilder { return m.UnsignedBigPrimaryKey() },
}

func TestBuildContentTemplateShapePerDialect(t *testing.T) {
	fields := "title:string(128):notNull,content:longText"
	wantPrimaryKey := map[string]string{
		"mysql":     `Add("id", m.UnsignedBigPrimaryKey()).`,
		"sqlite":    `Add("id", m.PrimaryKey()).`,
		"postgres":  `Add("id", m.BigPrimaryKey()).`,
		"sqlserver": `Add("id", m.BigPrimaryKey()).`,
	}
	for _, name := range migrate.DialectNames() {
		t.Run(name, func(t *testing.T) {
			wantPK, ok := wantPrimaryKey[name]
			if !ok {
				t.Fatalf("no primary key expectation for dialect %q", name)
			}
			content, err := BuildContent(Options{
				Name:        "create_article_table",
				PackageName: "migrations",
				Fields:      fields,
				Dialect:     name,
			}, "m20260613_103000_create_article_table")
			if err != nil {
				t.Fatalf("BuildContent() error = %v", err)
			}
			text := string(content)
			if !strings.Contains(text, wantPK) {
				t.Fatalf("%s template missing primary key %q:\n%s", name, wantPK, text)
			}
			if name == "mysql" {
				if !strings.Contains(text, `"ENGINE=InnoDB DEFAULT CHARSET=utf8mb4",`) {
					t.Fatalf("mysql template missing ENGINE options:\n%s", text)
				}
			} else if strings.Contains(text, "ENGINE=") {
				t.Fatalf("%s template must not carry MySQL table options:\n%s", name, text)
			}
			if !strings.Contains(text, `Add("title", m.String(128).NotNull()).`) {
				t.Fatalf("%s template missing field column:\n%s", name, text)
			}
			if !strings.Contains(text, `DropTable("article").`) {
				t.Fatalf("%s template missing Down drop:\n%s", name, text)
			}
		})
	}
}

// The generated Up body must render, through the dialect it targets, exactly
// the CREATE TABLE SQL the conformance matrix expects for that dialect.
func TestGeneratedUpSQLMatchesDialectExpectations(t *testing.T) {
	fields := "title:string(128):notNull,content:longText"
	wantCreate := map[string]string{
		"mysql":     "CREATE TABLE `article` (`id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY, `title` VARCHAR(128) NOT NULL, `content` LONGTEXT NULL) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4",
		"sqlite":    `CREATE TABLE "article" ("id" INTEGER PRIMARY KEY AUTOINCREMENT, "title" VARCHAR(128) NOT NULL, "content" TEXT NULL)`,
		"postgres":  `CREATE TABLE "article" ("id" BIGSERIAL PRIMARY KEY, "title" VARCHAR(128) NOT NULL, "content" TEXT NULL)`,
		"sqlserver": "CREATE TABLE [article] ([id] BIGINT IDENTITY(1,1) PRIMARY KEY NOT NULL, [title] NVARCHAR(128) NOT NULL, [content] NVARCHAR(MAX) NULL)",
	}
	for _, name := range migrate.DialectNames() {
		t.Run(name, func(t *testing.T) {
			wantSQL, ok := wantCreate[name]
			if !ok {
				t.Fatalf("no CREATE TABLE expectation for dialect %q", name)
			}
			content, err := BuildContent(Options{
				Name:    "create_article_table",
				Fields:  fields,
				Dialect: name,
			}, "m20260613_103000_create_article_table")
			if err != nil {
				t.Fatalf("BuildContent() error = %v", err)
			}
			text := string(content)

			pkMatch := primaryKeyLine.FindStringSubmatch(text)
			if pkMatch == nil {
				t.Fatalf("template has no id primary key line:\n%s", text)
			}
			build, ok := primaryKeyBuilders[pkMatch[1]]
			if !ok {
				t.Fatalf("template primary key %q names no known builder", pkMatch[1])
			}
			optMatch := optionsLine.FindStringSubmatch(text)
			if optMatch == nil {
				t.Fatalf("template has no table options line:\n%s", text)
			}
			options, err := strconv.Unquote(optMatch[1])
			if err != nil {
				t.Fatalf("template table options %q are not a quoted literal: %v", optMatch[1], err)
			}

			dialect, _ := migrate.DialectByName(name)
			m := migrate.NewMigrationContext(nil, dialect)
			gotSQL, err := dialect.CreateTable("article", migrate.Columns().
				Add("id", build(m)).
				Add("title", m.String(128).NotNull()).
				Add("content", m.LongText().Null()), options)
			if err != nil {
				t.Fatalf("generated Up columns rejected by %s: %v", name, err)
			}
			if gotSQL != wantSQL {
				t.Fatalf("CREATE TABLE SQL:\n got: %s\nwant: %s", gotSQL, wantSQL)
			}
		})
	}
}

func TestBuildContentDefaultsToMySQLDialect(t *testing.T) {
	content, err := BuildContent(Options{
		Name:   "create_article_table",
		Fields: "title:string(128):notNull",
	}, "m20260613_103000_create_article_table")
	if err != nil {
		t.Fatalf("BuildContent() error = %v", err)
	}
	text := string(content)
	if !strings.Contains(text, `Add("id", m.UnsignedBigPrimaryKey()).`) {
		t.Fatalf("default template missing mysql primary key:\n%s", text)
	}
	if !strings.Contains(text, `"ENGINE=InnoDB DEFAULT CHARSET=utf8mb4",`) {
		t.Fatalf("default template missing mysql table options:\n%s", text)
	}
}

func TestBuildContentRejectsUnknownDialect(t *testing.T) {
	_, err := BuildContent(Options{
		Name:    "create_article_table",
		Fields:  "title:string(128):notNull",
		Dialect: "oracle",
	}, "m20260613_103000_create_article_table")
	if err == nil {
		t.Fatal("BuildContent() returned nil error for unknown dialect")
	}
	if !strings.Contains(err.Error(), `unknown dialect "oracle"`) {
		t.Fatalf("error = %v", err)
	}
	for _, name := range migrate.DialectNames() {
		if !strings.Contains(err.Error(), name) {
			t.Fatalf("error does not list supported dialect %q: %v", name, err)
		}
	}
}
