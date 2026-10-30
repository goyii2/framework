package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goyii2/framework/base"
	"github.com/goyii2/framework/db"
)

// testApp assembles an application whose database is a throwaway sqlite
// file holding one table; the commands resolve their connection from it,
// the way a real goyii binary would from its configuration.
func testApp(t *testing.T, ddl ...string) *base.Application {
	t.Helper()
	dsn := filepath.Join(t.TempDir(), "console.db")
	conn, err := db.New(db.Config{Driver: "sqlite", DSN: dsn})
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	for _, stmt := range ddl {
		if err := conn.Exec(stmt).Error; err != nil {
			t.Fatalf("creating the test table: %v", err)
		}
	}
	app := base.New()
	app.SetDB(conn)
	return app
}

// run executes the gii command tree with args, returning its output and
// error.
func run(t *testing.T, app *base.Application, args ...string) (string, error) {
	t.Helper()
	root := New(app)
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), err
}

func TestModelCommandGeneratesFromConfiguredDatabase(t *testing.T) {
	app := testApp(t, `CREATE TABLE tag (id INTEGER PRIMARY KEY AUTOINCREMENT, label VARCHAR(30) NOT NULL)`)
	out := filepath.Join(t.TempDir(), "models")

	output, err := run(t, app, "model", "--table", "tag", "--out", out, "--package", "models")
	if err != nil {
		t.Fatalf("gii model error = %v\n%s", err, output)
	}
	if !strings.Contains(output, "created "+filepath.Join(out, "tag.go")) {
		t.Errorf("output misses the created file:\n%s", output)
	}
	if _, err := os.Stat(filepath.Join(out, "tag.go")); err != nil {
		t.Errorf("generated model missing: %v", err)
	}
}

func TestModelCommandOverridesDSN(t *testing.T) {
	// The application's database holds one table; --dsn points at another
	// file holding a different one — the flag decides which is read.
	app := testApp(t, `CREATE TABLE tag (id INTEGER PRIMARY KEY AUTOINCREMENT, label VARCHAR(30) NOT NULL)`)
	other := filepath.Join(t.TempDir(), "other.db")
	conn, err := db.New(db.Config{Driver: "sqlite", DSN: other})
	if err != nil {
		t.Fatal(err)
	}
	if err := conn.Exec(`CREATE TABLE category (id INTEGER PRIMARY KEY AUTOINCREMENT, name VARCHAR(40) NOT NULL)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}

	out := filepath.Join(t.TempDir(), "models")
	output, err := run(t, app, "model", "--table", "category", "--out", out, "--package", "models", "--dsn", other)
	if err != nil {
		t.Fatalf("gii model --dsn error = %v\n%s", err, output)
	}
	if !strings.Contains(output, filepath.Join(out, "category.go")) {
		t.Errorf("output misses the created file:\n%s", output)
	}
}

func TestModelCommandMissingTableFails(t *testing.T) {
	app := testApp(t, `CREATE TABLE tag (id INTEGER PRIMARY KEY AUTOINCREMENT, label VARCHAR(30) NOT NULL)`)
	if _, err := run(t, app, "model", "--table", "absent", "--out", t.TempDir()); err == nil {
		t.Fatal("generating a model of a missing table should fail")
	}
}

func TestControllerCommandPrintsRouteSnippet(t *testing.T) {
	app := testApp(t)
	out := filepath.Join(t.TempDir(), "controllers")

	output, err := run(t, app, "controller", "--id", "post", "--out", out, "--package", "main")
	if err != nil {
		t.Fatalf("gii controller error = %v\n%s", err, output)
	}
	if !strings.Contains(output, "created "+filepath.Join(out, "post_controller.go")) {
		t.Errorf("output misses the created file:\n%s", output)
	}
	if !strings.Contains(output, `c.Get("/post", ctl.ActionIndex).Name("post/index")`) {
		t.Errorf("output misses the route snippet:\n%s", output)
	}
}

func TestCrudCommandGeneratesSetAndPrintsSnippets(t *testing.T) {
	app := testApp(t, `CREATE TABLE post (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		title VARCHAR(200) NOT NULL,
		body TEXT,
		status VARCHAR(16) NOT NULL DEFAULT 'draft'
	)`)
	out := filepath.Join(t.TempDir(), "app")

	output, err := run(t, app, "crud", "--table", "post", "--out", out, "--package", "app")
	if err != nil {
		t.Fatalf("gii crud error = %v\n%s", err, output)
	}
	for _, name := range []string{"post.go", "post_controller.go", "post_views.templ"} {
		if !strings.Contains(output, filepath.Join(out, name)) {
			t.Errorf("output misses the created %s:\n%s", name, output)
		}
	}
	if !strings.Contains(output, `r.Register("post/list", view.Typed(postListPage))`) {
		t.Errorf("output misses the view registration snippet:\n%s", output)
	}
	if !strings.Contains(output, `Run "templ generate"`) {
		t.Errorf("output misses the templ generate hint:\n%s", output)
	}
}
