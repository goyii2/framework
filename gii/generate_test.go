package gii

import (
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"gorm.io/gorm"

	"github.com/goyii2/framework/config"
	"github.com/goyii2/framework/db"
)

// testConn opens a file-backed sqlite connection (file-backed so every
// pooled connection sees the same database) and creates the product table
// the generator tests run against: the full spread of column kinds —
// primary key, required and nullable strings, a text column, a decimal, an
// integer with a default, a boolean, a datetime and the timestamp pair.
func testConn(t *testing.T) *gorm.DB {
	t.Helper()
	conn, err := db.Connect(config.DB{Driver: "sqlite", DSN: filepath.Join(t.TempDir(), "gii.db")})
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	conn.Exec(`CREATE TABLE product (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		title VARCHAR(120) NOT NULL,
		slug VARCHAR(80) NOT NULL,
		description TEXT,
		price DECIMAL(10,2) NOT NULL,
		stock INTEGER NOT NULL DEFAULT 0,
		active BOOLEAN NOT NULL DEFAULT 1,
		published_at DATETIME,
		created_at DATETIME,
		updated_at DATETIME
	)`)
	conn.Exec(`CREATE UNIQUE INDEX idx_product_slug ON product(slug)`)
	if conn.Error != nil {
		t.Fatalf("creating the product table: %v", conn.Error)
	}
	return conn.DB
}

// readGenerated reads a generated file, failing the test when missing.
func readGenerated(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading generated %s: %v", path, err)
	}
	return string(content)
}

// assertParsable checks a generated .go file parses as Go source — the
// syntax half of compilability; the type check needs the surrounding
// project.
func assertParsable(t *testing.T, path string) {
	t.Helper()
	fset := token.NewFileSet()
	if _, err := parser.ParseFile(fset, path, nil, parser.AllErrors); err != nil {
		t.Fatalf("generated %s does not parse: %v", path, err)
	}
}

// assertGofmtClean checks the file is exactly what gofmt would print — the
// generator formats its Go output, so anything else is a template bug.
func assertGofmtClean(t *testing.T, path string) {
	t.Helper()
	content := readGenerated(t, path)
	formatted, err := format.Source([]byte(content))
	if err != nil {
		t.Fatalf("gofmt %s: %v", path, err)
	}
	if string(formatted) != content {
		t.Errorf("%s is not gofmt-clean", path)
	}
}

// assertContains fails with the missing excerpt.
func assertContains(t *testing.T, content, excerpt, what string) {
	t.Helper()
	if !strings.Contains(content, excerpt) {
		t.Errorf("generated code misses %s: %q not found in:\n%s", what, excerpt, content)
	}
}

// assertField checks a struct field maps to its Go type; the whitespace
// between name and type is gofmt's alignment, never a single literal
// space.
func assertField(t *testing.T, content, field, goType string) {
	t.Helper()
	pattern := `(?m)^\s*` + regexp.QuoteMeta(field) + `\s+` + regexp.QuoteMeta(goType) + `\s`
	if !regexp.MustCompile(pattern).MatchString(content) {
		t.Errorf("generated code misses field %s %s in:\n%s", field, goType, content)
	}
}

func TestGenerateModelMapsColumnsAndRules(t *testing.T) {
	out := filepath.Join(t.TempDir(), "models")
	result, err := GenerateModel(testConn(t), "product", Options{Out: out, Package: "models"})
	if err != nil {
		t.Fatalf("GenerateModel() error = %v", err)
	}
	if len(result.Files) != 1 || !strings.HasSuffix(result.Files[0], "product.go") {
		t.Fatalf("GenerateModel() files = %v, want [product.go]", result.Files)
	}

	assertParsable(t, result.Files[0])
	assertGofmtClean(t, result.Files[0])
	source := readGenerated(t, result.Files[0])

	assertContains(t, source, "package models", "the package clause")
	assertContains(t, source, "type Product struct {", "the model struct")
	assertContains(t, source, "ar.Model", "the Active Record embed")
	assertContains(t, source, "func (Product) TableName() string { return \"product\" }", "TableName")

	for field, want := range map[string]string{
		"ID":          "int",
		"Title":       "string",
		"Slug":        "string",
		"Description": "*string",
		"Price":       "float64",
		"Stock":       "int",
		"Active":      "bool",
		"PublishedAt": "*time.Time",
		"CreatedAt":   "*time.Time",
		"UpdatedAt":   "*time.Time",
	} {
		assertField(t, source, field, want)
	}
	assertContains(t, source, "`gorm:\"column:id;primaryKey", "the primary key tag")
	assertContains(t, source, "json:\"published_at\"", "the JSON tag")

	// Rules skeleton: NOT NULL without a default -> Required, varchar
	// length -> Max; defaulted and numeric columns carry TODO notes; the
	// unique slug index carries a uniqueness hint.
	assertContains(t, source, `"title": z.String().Required(z.Message("title cannot be blank")).Max(120, z.Message("title must be at most 120 characters")),`,
		"the title rule")
	assertContains(t, source, `Required(z.Message("slug cannot be blank")).Max(80`, "the slug rule")
	assertContains(t, source, "// TODO \"slug\": carries a unique index", "the unique hint")
	assertContains(t, source, "// TODO \"description\": nullable without a length", "the nullable text note")
	assertContains(t, source, "// TODO \"price\": float column", "the numeric note")
	assertContains(t, source, "// TODO \"stock\": int column", "the defaulted numeric note")
}

func TestGenerateModelRefusesMissingTable(t *testing.T) {
	if _, err := GenerateModel(testConn(t), "missing", Options{Out: t.TempDir()}); err == nil {
		t.Fatal("GenerateModel() of a missing table should error")
	}
}

func TestGenerateModelTemplateOverride(t *testing.T) {
	dir := t.TempDir()
	// A custom model template with a marker line; same file name as the
	// built-in one, so the --templates mechanism picks it up.
	override := filepath.Join(dir, "overrides")
	if err := os.MkdirAll(override, 0o755); err != nil {
		t.Fatal(err)
	}
	custom := "// custom-model-template for {{.Model}}\npackage {{.Package}}\n"
	if err := os.WriteFile(filepath.Join(override, ModelTemplate), []byte(custom), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "models")
	if _, err := GenerateModel(testConn(t), "product", Options{Out: out, Package: "models", TemplateDir: override}); err != nil {
		t.Fatalf("GenerateModel() with template override error = %v", err)
	}
	assertContains(t, readGenerated(t, filepath.Join(out, "product.go")),
		"// custom-model-template for Product", "the overridden template's marker")
}

func TestGenerateControllerSkeletonAndSnippet(t *testing.T) {
	out := filepath.Join(t.TempDir(), "controllers")
	result, err := GenerateController("post", Options{Out: out, Package: "controllers"})
	if err != nil {
		t.Fatalf("GenerateController() error = %v", err)
	}
	path := filepath.Join(out, "post_controller.go")
	if len(result.Files) != 1 || result.Files[0] != path {
		t.Fatalf("GenerateController() files = %v, want [%s]", result.Files, path)
	}
	assertParsable(t, path)
	assertGofmtClean(t, path)
	source := readGenerated(t, path)
	assertContains(t, source, "type PostController struct {", "the controller struct")
	assertContains(t, source, "web.Controller", "the web embed")
	for _, action := range []string{"ActionIndex", "ActionView", "ActionCreate", "ActionUpdate", "ActionDelete"} {
		assertContains(t, source, "func (pc *PostController) "+action+"(", "the "+action+" action")
	}

	if len(result.Snippets) != 1 {
		t.Fatalf("snippets = %d, want 1 (route registration)", len(result.Snippets))
	}
	snippet := result.Snippets[0]
	assertContains(t, snippet, "ctl := &PostController{}", "the controller construction")
	assertContains(t, snippet, `c.Get("/post", ctl.ActionIndex).Name("post/index")`, "the index route")
	assertContains(t, snippet, `c.Delete("/post/{id:[0-9]+}", ctl.ActionDelete).Name("post/delete")`, "the delete route")
}

func TestGenerateControllerRejectsInvalidID(t *testing.T) {
	if err := validateControllerID("post/evil"); err == nil {
		t.Fatal("a controller id with a slash should be rejected")
	}
	if _, err := GenerateController("", Options{Out: t.TempDir()}); err == nil {
		t.Fatal("an empty controller id should be rejected")
	}
}

func TestGenerateCrudProducesModelControllerAndViews(t *testing.T) {
	out := filepath.Join(t.TempDir(), "crud")
	result, err := GenerateCrud(testConn(t), "product", Options{Out: out, Package: "crud"})
	if err != nil {
		t.Fatalf("GenerateCrud() error = %v", err)
	}
	want := []string{"product.go", "product_controller.go", "product_views.templ"}
	if len(result.Files) != len(want) {
		t.Fatalf("GenerateCrud() files = %v, want %v", result.Files, want)
	}
	for i, name := range want {
		if filepath.Base(result.Files[i]) != name {
			t.Fatalf("GenerateCrud() files = %v, want %v", result.Files, want)
		}
	}
	for _, path := range result.Files[:2] {
		assertParsable(t, path)
		assertGofmtClean(t, path)
	}

	controller := readGenerated(t, filepath.Join(out, "product_controller.go"))
	// The five actions of the CRUD set.
	for _, action := range []string{"ActionList", "ActionNew", "ActionEdit", "ActionSave", "ActionDelete"} {
		assertContains(t, controller, "func (pc *ProductController) "+action+"(", "the "+action+" action")
	}
	// The list goes through the dataprovider convention and renders the
	// GridView + LinkPager widgets.
	assertContains(t, controller, "dataprovider.FromRequest(c.Request(), sortableProductFields...)", "the sortable parse")
	assertContains(t, controller, "dataprovider.ActiveDataProvider[Product]{", "the provider")
	assertContains(t, controller, "view.GridView{", "the grid widget")
	assertContains(t, controller, "view.LinkPager{", "the pager widget")
	// The form is an ActiveForm echoed from the record's values.
	assertContains(t, controller, "view.ActiveForm{", "the form widget")
	assertContains(t, controller, "Values: productFormValues(product),", "the form echo")
	// Input types by column kind: decimal -> number, bool -> checkbox,
	// datetime -> datetime-local, text -> textarea.
	assertContains(t, controller, `{Name: "price", Label: "Price", Type: view.FieldNumber, Required: true},`, "the number input")
	assertContains(t, controller, `{Name: "active", Label: "Active", Type: view.FieldCheckbox},`, "the checkbox input")
	assertContains(t, controller, `{Name: "published_at", Label: "Published At", Type: view.FieldDatetimeLocal},`, "the datetime input")
	assertContains(t, controller, `{Name: "description", Label: "Description", Type: view.FieldTextarea},`, "the textarea input")
	// The grid skips the long text column; the form skips the timestamps.
	if strings.Contains(controller, `{Attr: "description"`) {
		t.Error("the grid should skip the text column")
	}
	if strings.Contains(controller, `{Name: "created_at"`) {
		t.Error("the form should skip created_at")
	}
	// Numeric parsing through the invalid-reporting closure.
	assertContains(t, controller, `strconv.ParseFloat(in.price, 64)`, "the decimal parse")
	assertContains(t, controller, `invalid("price", "a number")`, "the parse failure report")
	// Nullable columns left empty stay NULL; the echo map is keyed by the
	// column names.
	assertContains(t, controller, "product.Description = stringPtr(in.description)", "the nullable string apply")
	assertContains(t, controller, `if in.publishedAt != "" {`, "the nullable datetime guard")
	assertContains(t, controller, `values["published_at"] = timeValue(product.PublishedAt)`, "the datetime echo")
	// The helpers the table's columns need — and only those.
	assertContains(t, controller, "func stringPtr(v string) *string {", "the stringPtr helper")
	assertContains(t, controller, "func checkboxValue(on bool) string {", "the checkboxValue helper")
	assertContains(t, controller, "func timeValue(t *time.Time) string {", "the timeValue helper")
	assertContains(t, controller, "const formTimeLayout = \"2006-01-02T15:04\"", "the form time layout")
	if strings.Contains(controller, "func boolPtr") {
		t.Error("boolPtr should not generate without a nullable boolean column")
	}

	views := readGenerated(t, filepath.Join(out, "product_views.templ"))
	assertContains(t, views, "package crud", "the templ package clause")
	assertContains(t, views, "templ productListPage(d productListData) {", "the list page")
	assertContains(t, views, "templ productFormPage(d productFormData) {", "the form page")
	assertContains(t, views, "@d.Grid", "the grid embed")
	assertContains(t, views, "@d.Pager", "the pager embed")
	assertContains(t, views, "@d.Form", "the form embed")
	assertContains(t, views, `name="_csrf"`, "the delete CSRF field")

	// The wiring snippets: routes, view registration, the templ hint.
	joined := strings.Join(result.Snippets, "\n---\n")
	assertContains(t, joined, `c.Get("/product", ctl.ActionList).Name("product/list")`, "the list route snippet")
	assertContains(t, joined, `c.Post("/product/{id:[0-9]+}", ctl.ActionSave).Name("product/update")`, "the update route snippet")
	assertContains(t, joined, `r.Register("product/list", view.Typed(productListPage))`, "the list view registration")
	assertContains(t, joined, `r.Register("product/form", view.Typed(productFormPage))`, "the form view registration")
	assertContains(t, joined, `Run "templ generate"`, "the templ generate hint")
}

func TestGenerateCrudRequiresPrimaryKey(t *testing.T) {
	conn, err := db.Connect(config.DB{Driver: "sqlite", DSN: filepath.Join(t.TempDir(), "noidx.db")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	conn.Exec(`CREATE TABLE nokey (label VARCHAR(10))`)
	if conn.Error != nil {
		t.Fatal(conn.Error)
	}
	if _, err := GenerateCrud(conn.DB, "nokey", Options{Out: t.TempDir()}); err == nil {
		t.Fatal("GenerateCrud() without a primary key should error")
	}
}

// TestGeneratedCrudCompilesAsPackage goes one step past parsing for a
// table without nullable or time columns: the generated model and
// controller share no external scaffolding then (no templ views involved
// beyond the view names), so both files parse against the parser with all
// declarations resolvable — the guarantee the generator can give without
// the surrounding project.
func TestGeneratedCrudParsesAllDeclarations(t *testing.T) {
	conn, err := db.Connect(config.DB{Driver: "sqlite", DSN: filepath.Join(t.TempDir(), "flat.db")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	conn.Exec(`CREATE TABLE tag (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name VARCHAR(40) NOT NULL,
		weight INTEGER NOT NULL DEFAULT 0
	)`)
	if conn.Error != nil {
		t.Fatal(conn.Error)
	}
	out := t.TempDir()
	if _, err := GenerateCrud(conn.DB, "tag", Options{Out: out, Package: "flat"}); err != nil {
		t.Fatalf("GenerateCrud() error = %v", err)
	}
	// Both Go files parse and declare what the pair promises: the model
	// file the struct, the controller file the actions over it.
	model := readGenerated(t, filepath.Join(out, "tag.go"))
	controller := readGenerated(t, filepath.Join(out, "tag_controller.go"))
	assertContains(t, model, "type Tag struct {", "the model")
	assertContains(t, controller, "dataprovider.ActiveDataProvider[Tag]{", "the typed provider")
	// No time columns -> no time import, no time helpers.
	if strings.Contains(controller, "\"time\"") || strings.Contains(controller, "timeValue") {
		t.Error("time helpers should not generate without time columns")
	}
	if strings.Contains(model, "\"time\"") {
		t.Error("the model should not import time without time columns")
	}
}
