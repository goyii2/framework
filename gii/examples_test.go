package gii

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	// The sqlite driver registration the migrate layer opens connections
	// through — the same one the migrate commands use.
	_ "github.com/glebarez/go-sqlite"

	"github.com/goyii2/framework/db"
	"github.com/goyii2/framework/examples/basic/migrations"
	"github.com/goyii2/framework/migrate"
)

// exampleDatabase applies the basic example's migrations to a throwaway
// sqlite file and returns a GORM connection over it: the generation
// acceptance runs against the real table shapes the example application
// uses (user, post with the timestamp columns), without touching the
// example's runtime directory or committing anything to its database.
func exampleDatabase(t *testing.T) (*db.Conn, func()) {
	t.Helper()
	dsn := filepath.Join(t.TempDir(), "example.db")
	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("opening the migration connection: %v", err)
	}
	dialect, ok := migrate.DialectByName("sqlite")
	if !ok {
		t.Fatal("sqlite dialect not registered")
	}
	migrator := migrate.NewMigrator(conn, dialect, migrations.All())
	// SQLite cannot acquire the migrate package's advisory lock; the
	// migrator runs without it, as the migrate commands do for sqlite.
	migrator.UseLock = false
	if _, err := migrator.Up(context.Background(), 0); err != nil {
		t.Fatalf("applying the example migrations: %v", err)
	}

	gormConn, err := db.New(db.Config{Driver: "sqlite", DSN: dsn})
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	return gormConn, func() {
		_ = gormConn.Close()
		_ = conn.Close()
	}
}

// TestGenerateModelAgainstExampleUserTable generates the model of the
// example's user table and checks the guessed mapping: required rules for
// the NOT NULL columns, Max from the varchar lengths, and no Required for
// the defaulted role column.
func TestGenerateModelAgainstExampleUserTable(t *testing.T) {
	conn, closeDB := exampleDatabase(t)
	defer closeDB()

	out := filepath.Join(t.TempDir(), "models")
	if _, err := GenerateModel(conn.DB, "user", Options{Out: out, Package: "models"}); err != nil {
		t.Fatalf("GenerateModel() error = %v", err)
	}
	source := readGenerated(t, filepath.Join(out, "user.go"))
	assertParsable(t, filepath.Join(out, "user.go"))
	// SQLite maps every integer kind — the migration's BigPrimaryKey
	// included — onto INTEGER, so the key reads back as a plain int.
	assertField(t, source, "ID", "int")
	assertField(t, source, "Username", "string")
	assertField(t, source, "PasswordHash", "string")
	assertField(t, source, "Role", "string")
	assertContains(t, source, `Required(z.Message("username cannot be blank")).Max(64`, "the username rule")
	assertContains(t, source, `Required(z.Message("password_hash cannot be blank")).Max(128`, "the password hash rule")
	// role carries a database default, so it stays optional: no Required,
	// just the Max of its varchar length (gofmt aligns the shape's keys,
	// so the rule is matched without the key).
	assertContains(t, source, `z.String().Max(16, z.Message("role must be at most 16 characters"))`, "the role rule without Required")
	if strings.Contains(source, `"role": z.String().Required`) {
		t.Error("a defaulted column should not become Required")
	}
}

// TestGenerateCrudAgainstExamplePostTable generates the whole CRUD set of
// the example's post table — the shape with a bigint author reference, a
// text body, a defaulted status and the nullable timestamp pair — and
// parses every produced Go file: the acceptance bar the ticket sets.
// Compiling the views with the real templ toolchain needs the surrounding
// project and is left to the user.
func TestGenerateCrudAgainstExamplePostTable(t *testing.T) {
	conn, closeDB := exampleDatabase(t)
	defer closeDB()

	out := filepath.Join(t.TempDir(), "generated")
	result, err := GenerateCrud(conn.DB, "post", Options{Out: out, Package: "generated"})
	if err != nil {
		t.Fatalf("GenerateCrud() error = %v", err)
	}
	for _, path := range result.Files {
		if strings.HasSuffix(path, ".go") {
			assertParsable(t, path)
			assertGofmtClean(t, path)
		}
	}

	model := readGenerated(t, filepath.Join(out, "post.go"))
	assertField(t, model, "ID", "int")
	assertField(t, model, "Title", "string")
	assertField(t, model, "Body", "*string")
	assertField(t, model, "Status", "string")
	assertField(t, model, "AuthorID", "int")
	// The timestamp columns the later migration added: nullable datetime
	// columns generate pointer time fields, the timestamps included.
	assertField(t, model, "CreatedAt", "*time.Time")
	assertField(t, model, "UpdatedAt", "*time.Time")
	// The binding whitelist of the example's shape: the editable columns
	// carry form tags, the key and the author reference stay server-side.
	for _, column := range []string{"title", "body", "status"} {
		assertContains(t, model, "form:\""+column+"\"", "the "+column+" binding tag")
	}
	for _, column := range []string{"id", "author_id", "created_at", "updated_at"} {
		if strings.Contains(model, "form:\""+column+"\"") {
			t.Errorf("the %s column should stay out of the binding whitelist", column)
		}
	}

	controller := readGenerated(t, filepath.Join(out, "post_controller.go"))
	// The bigint primary key goes through the typed FindByID path.
	assertContains(t, controller, "ar.FindByID[Post](c.Context(), id)", "the typed find")
	// The write actions bind straight into the record; the text body
	// renders as a textarea.
	assertContains(t, controller, "if errs := c.LoadForm(post, scenario); errs != nil {", "the direct binding")
	assertContains(t, controller, "Values: form.Values(post),", "the form refill")
	assertContains(t, controller, `{Name: "body", Label: "Body", Type: view.FieldTextarea},`, "the body textarea")
	// The listing filter skeleton: the filter struct reads the query and
	// narrows the provider's query through Apply.
	assertContains(t, controller, "form.BindQuery(c.Request(), &filters)", "the query binding")
	assertContains(t, controller, `query = query.Where("title LIKE ?"+likeEscapeClause, likePattern(f.Title))`, "the title LIKE filter")

	views := readGenerated(t, filepath.Join(out, "post_views.templ"))
	assertContains(t, views, "templ postListPage(d postListData) {", "the list page")
	assertContains(t, views, "templ postFormPage(d postFormData) {", "the form page")

	if len(result.Files) != 3 {
		t.Fatalf("GenerateCrud() files = %v, want model, controller and views", result.Files)
	}
}

// TestIntrospectReportsExampleTables is the small sanity check behind the
// acceptance run: the migrator sees the example tables with the facts the
// generator decisions build on.
func TestIntrospectReportsExampleTables(t *testing.T) {
	conn, closeDB := exampleDatabase(t)
	defer closeDB()

	user, err := Introspect(conn.DB, "user")
	if err != nil {
		t.Fatalf("Introspect(user) error = %v", err)
	}
	if pk := user.PrimaryKey(); pk == nil || pk.Name != "id" {
		t.Errorf("user primary key = %+v, want id", pk)
	}
	post, err := Introspect(conn.DB, "post")
	if err != nil {
		t.Fatalf("Introspect(post) error = %v", err)
	}
	for _, c := range post.Columns {
		if c.Name == "title" && (c.Nullable || c.Length != 200) {
			t.Errorf("title column = %+v, want NOT NULL varchar(200)", c)
		}
		if c.Name == "status" && !c.HasDefault {
			t.Errorf("status column = %+v, want the migration's default", c)
		}
	}
}
