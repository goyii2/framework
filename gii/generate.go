package gii

import (
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"

	"gorm.io/gorm"
)

// Options control where and under which package name generation writes.
type Options struct {
	// Out is the output directory; files land directly inside it. It is
	// created when missing. Empty means the working directory.
	Out string
	// Package is the package clause of the generated files; empty derives
	// it from the base name of Out (pass it explicitly when that guess is
	// wrong, e.g. --package main).
	Package string
	// TemplateDir is a directory whose same-named template files override
	// the embedded set; empty uses the embedded set only.
	TemplateDir string
}

// resolve normalizes the options: Out defaults to the working directory and
// is created, Package defaults to the Out base name and must be a usable Go
// identifier.
func (o Options) resolve() (Options, error) {
	if strings.TrimSpace(o.Out) == "" {
		o.Out = "."
	}
	if err := os.MkdirAll(o.Out, 0o755); err != nil {
		return o, fmt.Errorf("gii: create output directory %s: %w", o.Out, err)
	}
	if o.Package == "" {
		abs, err := filepath.Abs(o.Out)
		if err != nil {
			return o, err
		}
		o.Package = filepath.Base(abs)
	}
	if !isValidIdentifier(o.Package) {
		return o, fmt.Errorf("gii: package name %q is not a valid Go identifier; pass --package explicitly", o.Package)
	}
	return o, nil
}

// Result reports one generation run: the files written and the console
// snippets to print afterwards (route registration, view registration,
// follow-up hints).
type Result struct {
	// Files lists the written paths, in write order.
	Files []string
	// Snippets carries the code blocks the caller should print — wiring
	// the generated code into the application is left to the user.
	Snippets []string
}

// GenerateModel introspects table and writes its Active Record model file,
// <out>/<table>.go: a struct embedding ar.Model with one field per column
// plus the Rules() skeleton.
func GenerateModel(conn *gorm.DB, table string, opts Options) (*Result, error) {
	opts, err := opts.resolve()
	if err != nil {
		return nil, err
	}
	t, err := Introspect(conn, table)
	if err != nil {
		return nil, err
	}
	tmpl, err := loadTemplate(ModelTemplate, opts.TemplateDir)
	if err != nil {
		return nil, err
	}
	result := &Result{}
	if err := writeGoFile(filepath.Join(opts.Out, table+".go"), tmpl, buildModelData(t, opts.Package), result); err != nil {
		return nil, err
	}
	return result, nil
}

// GenerateController writes the controller skeleton of the given controller
// id, <out>/<id>_controller.go: a web.Controller embed with empty actions,
// meant to be filled in by hand. The returned snippet shows the route
// registration.
func GenerateController(id string, opts Options) (*Result, error) {
	if err := validateControllerID(id); err != nil {
		return nil, err
	}
	opts, err := opts.resolve()
	if err != nil {
		return nil, err
	}
	tmpl, err := loadTemplate(ControllerTemplate, opts.TemplateDir)
	if err != nil {
		return nil, err
	}
	structName, receiver := controllerName(id)
	data := controllerData{
		Package:    opts.Package,
		ID:         id,
		StructName: structName,
		Receiver:   receiver,
		Label:      label(strings.NewReplacer("-", "_").Replace(id)),
	}

	result := &Result{}
	name := strings.NewReplacer("-", "_").Replace(id) + "_controller.go"
	if err := writeGoFile(filepath.Join(opts.Out, name), tmpl, data, result); err != nil {
		return nil, err
	}
	result.Snippets = append(result.Snippets, routeSnippetController(id))
	return result, nil
}

// GenerateCrud writes the full CRUD set of a table: the model file of
// GenerateModel, the <out>/<table>_controller.go with the list/new/edit/
// save/delete actions, and the <out>/<table>_views.templ view file. The
// returned snippets carry the route and view registration plus the templ
// generate hint; compiling the views is left to the user's project.
func GenerateCrud(conn *gorm.DB, table string, opts Options) (*Result, error) {
	opts, err := opts.resolve()
	if err != nil {
		return nil, err
	}
	t, err := Introspect(conn, table)
	if err != nil {
		return nil, err
	}
	data, err := buildCrudData(t, table, opts.Package)
	if err != nil {
		return nil, err
	}

	result := &Result{}
	modelTmpl, err := loadTemplate(ModelTemplate, opts.TemplateDir)
	if err != nil {
		return nil, err
	}
	controllerTmpl, err := loadTemplate(CrudControllerTemplate, opts.TemplateDir)
	if err != nil {
		return nil, err
	}
	viewsTmpl, err := loadTemplate(CrudViewsTemplate, opts.TemplateDir)
	if err != nil {
		return nil, err
	}

	if err := writeGoFile(filepath.Join(opts.Out, table+".go"), modelTmpl, buildModelData(t, opts.Package), result); err != nil {
		return nil, err
	}
	if err := writeGoFile(filepath.Join(opts.Out, data.FileBase+"_controller.go"), controllerTmpl, data, result); err != nil {
		return nil, err
	}
	if err := writePlainFile(filepath.Join(opts.Out, data.FileBase+"_views.templ"), viewsTmpl, data, result); err != nil {
		return nil, err
	}

	result.Snippets = append(result.Snippets,
		routeSnippetCrud(data),
		viewSnippetCrud(data),
		"Run \"templ generate\" (or \"go run github.com/a-h/templ/cmd/templ generate\") to compile the generated views.")
	return result, nil
}

// writeGoFile renders tmpl over data, gofmt's the output and writes it to
// path, recording it on result. The format step doubles as a syntax check:
// a template that renders unparsable code fails here instead of writing a
// broken file.
func writeGoFile(path string, tmpl *template.Template, data any, result *Result) error {
	source, err := render(tmpl, data)
	if err != nil {
		return err
	}
	formatted, err := format.Source([]byte(source))
	if err != nil {
		return fmt.Errorf("gii: format %s: %w", path, err)
	}
	return writeRecorded(path, formatted, result)
}

// writePlainFile renders and writes without gofmt — the .templ views are
// not Go source until templ generate compiles them.
func writePlainFile(path string, tmpl *template.Template, data any, result *Result) error {
	source, err := render(tmpl, data)
	if err != nil {
		return err
	}
	return writeRecorded(path, []byte(source), result)
}

// writeRecorded writes content to path and records the path on result.
func writeRecorded(path string, content []byte, result *Result) error {
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return fmt.Errorf("gii: write %s: %w", path, err)
	}
	result.Files = append(result.Files, path)
	return nil
}

// reservedFieldNames are ar.Model's exported methods: a column mapping to
// one of them would shadow the Active Record behavior with a field, so the
// generated code carries a rename hint.
var reservedFieldNames = map[string]bool{
	"Context": true, "Validate": true, "Save": true, "Delete": true, "Reload": true,
}

// textualDBTypes are the unbounded string types: too wide for a table cell
// or a length rule, so the CRUD grid leaves them out (they stay in the form
// as textareas).
var textualDBTypes = map[string]bool{
	"text": true, "tinytext": true, "mediumtext": true, "longtext": true, "clob": true,
}

// timestampColumns are the conventionally server-stamped columns: the CRUD
// form skips them and the Rules skeleton leaves them alone.
var timestampColumns = map[string]bool{"created_at": true, "updated_at": true}

// modelField is one struct field of a generated model plus its Rules
// skeleton pieces.
type modelField struct {
	// Name is the exported Go field name.
	Name string
	// Column is the database column name (also the JSON tag).
	Column string
	// Type is the Go type, pointer included when the column is nullable.
	Type string
	// GormTag is the gorm struct tag's value.
	GormTag string
	// Comment is a trailing field comment; "" renders none.
	Comment string
	// Rule is the z.Shape entry of the Rules skeleton, "" when none was
	// guessed.
	Rule string
	// Notes are the comment lines inside the shape marking columns the
	// generator did not dare to classify.
	Notes []string
}

// modelData drives the model template.
type modelData struct {
	Package  string
	Table    string
	Model    string
	Receiver string
	// UsesTime adds the time import.
	UsesTime bool
	Fields   []modelField
}

// buildModelData assembles the model template's view of a table.
func buildModelData(t *Table, pkg string) modelData {
	data := modelData{
		Package:  pkg,
		Table:    t.Name,
		Model:    exportedName(t.Name),
		Receiver: lowerName(t.Name),
	}
	for _, c := range t.Columns {
		field := modelField{
			Name:   exportedName(c.Name),
			Column: c.Name,
			Type:   c.goType(),
		}
		tag := "column:" + c.Name
		if c.PrimaryKey {
			tag += ";primaryKey"
			if c.AutoIncrement {
				tag += ";autoIncrement"
			}
		}
		field.GormTag = tag
		if c.Unmapped {
			field.Comment = "TODO: unmapped type " + c.DBType + " — adjust the Go type"
		}
		if reservedFieldNames[field.Name] {
			field.Comment = "TODO: shadows ar.Model's " + field.Name + " method — rename the field"
		}
		if c.GoType == "time.Time" {
			data.UsesTime = true
		}
		field.Rule, field.Notes = ruleSkeleton(c)
		data.Fields = append(data.Fields, field)
	}
	return data
}

// ruleSkeleton guesses one column's validation rule: a NOT NULL column
// without a database default becomes Required, a varchar length becomes
// Max. Anything the generator cannot classify comes back as notes instead —
// an honest TODO beats a wrong rule.
func ruleSkeleton(c Column) (rule string, notes []string) {
	if c.PrimaryKey || timestampColumns[c.Name] || c.Kind() == KindBytes {
		return "", nil
	}
	if c.Kind() == KindString {
		var chain []string
		if !c.Nullable && !c.HasDefault {
			chain = append(chain, fmt.Sprintf("Required(z.Message(%q))", c.Name+" cannot be blank"))
		}
		if c.HasLength && c.Length > 0 {
			chain = append(chain, fmt.Sprintf("Max(%d, z.Message(%q))", c.Length,
				fmt.Sprintf("%s must be at most %d characters", c.Name, c.Length)))
		}
		if len(chain) > 0 {
			rule = fmt.Sprintf("%q: z.String().%s,", c.Name, strings.Join(chain, "."))
		} else {
			notes = append(notes, fmt.Sprintf("// TODO %q: nullable without a length — no rule guessed.", c.Name))
		}
	} else {
		notes = append(notes, fmt.Sprintf("// TODO %q: %s column — no rule guessed.", c.Name, c.Kind()))
	}
	if c.Unique {
		notes = append(notes, fmt.Sprintf("// TODO %q: carries a unique index — consider a uniqueness check.", c.Name))
	}
	return rule, notes
}

// controllerData drives the plain controller template.
type controllerData struct {
	Package    string
	ID         string
	StructName string
	Receiver   string
	Label      string
}

// crudPK is the primary key's view in the CRUD templates.
type crudPK struct {
	// Column is the key's column and path parameter name.
	Column string
	// GoName is the model field name.
	GoName string
	// Numeric reports an integer key: the route pattern then restricts to
	// digits and lookups parse through ParamInt64.
	Numeric bool
	// Pattern is the path parameter pattern of one row.
	Pattern string
	// ZeroCheck is the create-or-edit discriminator over the model
	// variable ("post.ID != 0").
	ZeroCheck string
	// ValueExpr formats the key for URL parameters.
	ValueExpr string
}

// crudColumn is one GridView column of the CRUD list.
type crudColumn struct {
	Attr      string
	Label     string
	ValueCode string
}

// crudFormField is one ActiveForm field of the CRUD form.
type crudFormField struct {
	// Name is the form field name — the column name.
	Name string
	// Label heads the field.
	Label string
	// InputType is the view.ActiveField input constant ("view.FieldText").
	InputType string
	// Required and MaxLength carry the native attributes the column
	// suggests.
	Required  bool
	MaxLength int64
	// GoName is the form input struct's field name.
	GoName string
	// ApplyCode writes the submitted value into the record; parse failures
	// report through the apply function's invalid closure.
	ApplyCode string
	// ValueStmt fills the form's echo map from the record's value.
	ValueStmt string
}

// crudData drives the CRUD controller and view templates.
type crudData struct {
	Package    string
	ID         string
	Model      string
	ModelVar   string
	Controller string
	Receiver   string
	Label      string
	// FileBase is the file name stem of the controller and view files.
	FileBase    string
	PK          crudPK
	Sortable    []string
	GridColumns []crudColumn
	FormFields  []crudFormField

	// The Uses* flags keep the generated imports and helpers down to what
	// the table actually needs: the time import, the formTimeLayout const
	// and the timeValue helper, the nullable/checkbox helpers, and the
	// parse-failure closure of the apply function.
	UsesTime        bool
	UsesTimeHelpers bool
	UsesStringPtr   bool
	UsesBoolPtr     bool
	UsesCheckbox    bool
	UsesInvalid     bool
}

// buildCrudData assembles the CRUD templates' view of a table: the grid
// skips textual and binary columns, the form skips the key and the
// timestamp columns, and everything else renders with the input type its
// database type suggests.
func buildCrudData(t *Table, table, pkg string) (*crudData, error) {
	pk := t.PrimaryKey()
	if pk == nil {
		return nil, fmt.Errorf("gii: table %q has no primary key; CRUD generation requires exactly one", table)
	}

	data := &crudData{
		Package: pkg,
		ID:      table,
		Model:   exportedName(table),
		Label:   label(table),
		PK:      crudPK{Column: pk.Name, GoName: exportedName(pk.Name)},
	}
	data.ModelVar = lowerName(table)
	data.Controller = data.Model + "Controller"
	data.Receiver = strings.ToLower(data.Model[:1]) + "c"
	data.FileBase = strings.NewReplacer("-", "_").Replace(table)

	data.PK.Numeric = pk.Kind() == KindInt
	switch {
	case data.PK.Numeric:
		data.PK.Pattern = "{" + pk.Name + ":[0-9]+}"
		data.PK.ZeroCheck = fmt.Sprintf("%s.%s != 0", data.ModelVar, data.PK.GoName)
		key := fmt.Sprintf("%s.%s", data.ModelVar, data.PK.GoName)
		if pk.GoType != "int64" {
			// Narrower integer keys cast up for FormatInt.
			key = fmt.Sprintf("int64(%s)", key)
		}
		data.PK.ValueExpr = fmt.Sprintf("strconv.FormatInt(%s, 10)", key)
	case pk.Kind() == KindTime:
		data.PK.Pattern = "{" + pk.Name + "}"
		data.PK.ZeroCheck = fmt.Sprintf("!%s.%s.IsZero()", data.ModelVar, data.PK.GoName)
		data.PK.ValueExpr = fmt.Sprintf("%s.%s.Format(time.RFC3339)", data.ModelVar, data.PK.GoName)
	default:
		data.PK.Pattern = "{" + pk.Name + "}"
		data.PK.ZeroCheck = fmt.Sprintf("%s.%s != \"\"", data.ModelVar, data.PK.GoName)
		data.PK.ValueExpr = fmt.Sprintf("%s.%s", data.ModelVar, data.PK.GoName)
	}

	for _, c := range t.Columns {
		if c.Kind() == KindTime {
			data.UsesTime = true
		}
		if c.PrimaryKey || c.Kind() == KindBytes {
			continue
		}
		if !textualDBTypes[c.DBType] {
			data.GridColumns = append(data.GridColumns, crudColumn{
				Attr:      c.Name,
				Label:     label(c.Name),
				ValueCode: gridValueCode(data.Model, c),
			})
			data.Sortable = append(data.Sortable, c.Name)
		}
		// The flags track what the form fields — not the grid — need:
		// helpers and imports that no apply statement uses would not
		// compile.
		if !timestampColumns[c.Name] {
			switch {
			case c.Kind() == KindTime:
				data.UsesTimeHelpers = true
				data.UsesInvalid = true
			case c.Kind() == KindInt || c.Kind() == KindFloat:
				data.UsesInvalid = true
			case c.Kind() == KindBool:
				data.UsesCheckbox = true
				if c.Pointer() {
					data.UsesBoolPtr = true
				}
			case c.Kind() == KindString && c.Pointer():
				data.UsesStringPtr = true
			}
			data.FormFields = append(data.FormFields, buildFormField(data.ModelVar, c))
		}
	}
	return data, nil
}

// gridValueCode builds the Value closure of one grid column: how a row's
// cell resolves from the generic item the GridView hands in. Nullable
// fields render empty when NULL, times render in a readable layout.
func gridValueCode(model string, c Column) string {
	field := exportedName(c.Name)
	row := fmt.Sprintf("item.(*%s)", model)
	switch {
	case c.Kind() == KindTime && c.Pointer():
		return fmt.Sprintf("func(item any) any { if row := %s; row.%s != nil { return row.%s.Format(%q) }; return \"\" }",
			row, field, field, "2006-01-02 15:04")
	case c.Kind() == KindTime:
		return fmt.Sprintf("func(item any) any { return %s.%s.Format(%q) }", row, field, "2006-01-02 15:04")
	case c.Pointer():
		return fmt.Sprintf("func(item any) any { if row := %s; row.%s != nil { return *row.%s }; return \"\" }",
			row, field, field)
	default:
		return fmt.Sprintf("func(item any) any { return %s.%s }", row, field)
	}
}

// inputType maps a column to its ActiveForm input constant: TEXT columns
// become textareas, numeric columns number inputs, booleans checkboxes,
// date-ish columns datetime-local pickers and everything else text inputs.
func inputType(c Column) string {
	switch {
	case textualDBTypes[c.DBType]:
		return "view.FieldTextarea"
	case c.Kind() == KindInt || c.Kind() == KindFloat:
		return "view.FieldNumber"
	case c.Kind() == KindBool:
		return "view.FieldCheckbox"
	case c.Kind() == KindTime:
		return "view.FieldDatetimeLocal"
	default:
		return "view.FieldText"
	}
}

// buildFormField assembles one form field with its apply and echo code.
func buildFormField(modelVar string, c Column) crudFormField {
	field := crudFormField{
		Name:      c.Name,
		Label:     label(c.Name),
		InputType: inputType(c),
		Required:  !c.Nullable && !c.HasDefault,
		GoName:    lowerName(c.Name),
	}
	if c.HasLength && c.Length > 0 && c.Kind() == KindString && !textualDBTypes[c.DBType] {
		field.MaxLength = c.Length
	}
	field.ApplyCode = applyCode(modelVar, c)
	field.ValueStmt = valueStmt(modelVar, c)
	return field
}

// applyCode builds the statement writing one submitted value into the
// record. Values that cannot parse report through the invalid closure of
// the generated apply function; nullable columns left empty stay NULL.
func applyCode(modelVar string, c Column) string {
	name := lowerName(c.Name)
	target := fmt.Sprintf("%s.%s", modelVar, exportedName(c.Name))
	input := "in." + name

	parse, assign, want := "", "", ""
	switch c.Kind() {
	case KindInt:
		parse = fmt.Sprintf("strconv.ParseInt(%s, 10, %d)", input, intBits(c.GoType))
		assign = typedAssign(target, "v", c.GoType, c.Pointer())
		want = "an integer"
	case KindFloat:
		parse = fmt.Sprintf("strconv.ParseFloat(%s, 64)", input)
		assign = typedAssign(target, "v", c.GoType, c.Pointer())
		want = "a number"
	case KindTime:
		parse = fmt.Sprintf("time.Parse(formTimeLayout, %s)", input)
		assign = typedAssign(target, "v", c.GoType, c.Pointer())
		want = "a date and time"
	case KindBool:
		if c.Pointer() {
			return fmt.Sprintf("%s = boolPtr(%s != \"\")", target, input)
		}
		return fmt.Sprintf("%s = %s != \"\"", target, input)
	default:
		if c.Pointer() {
			return fmt.Sprintf("%s = stringPtr(%s)", target, input)
		}
		return fmt.Sprintf("%s = %s", target, input)
	}

	call := fmt.Sprintf("if v, err := %s; err == nil { %s } else { invalid(%q, %q) }", parse, assign, c.Name, want)
	if c.Pointer() {
		// An empty input on a nullable column means NULL — nothing to
		// parse, the field keeps its nil.
		call = fmt.Sprintf("if %s != \"\" { %s }", input, call)
	}
	return call
}

// typedAssign builds the assignment of parsed value (int64 from ParseInt,
// float64 from ParseFloat, time.Time from time.Parse) to target: narrower
// integer types cast, pointer fields take the address of a local so NULL
// stays expressible.
func typedAssign(target, value, goType string, pointer bool) string {
	if goType == "int" || goType == "int8" || goType == "int16" || goType == "int32" {
		value = goType + "(" + value + ")"
	}
	if !pointer {
		return fmt.Sprintf("%s = %s", target, value)
	}
	return fmt.Sprintf("parsed := %s; %s = &parsed", value, target)
}

// intBits returns the ParseInt bit width of a Go integer type; "int" uses
// the platform's int size so overflow is rejected by the parser itself.
func intBits(goType string) int {
	switch goType {
	case "int8":
		return 8
	case "int16":
		return 16
	case "int32":
		return 32
	case "int":
		return strconv.IntSize
	default:
		return 64
	}
}

// valueStmt builds the statement filling the form's echo map from the
// record's current value; nullable values render empty when NULL. The map
// is keyed by the column name — the ActiveForm field name — while the
// in-struct field names stay lowerCamel.
func valueStmt(modelVar string, c Column) string {
	target := fmt.Sprintf("%s.%s", modelVar, exportedName(c.Name))
	set := func(expr string) string {
		return fmt.Sprintf("values[%q] = %s", c.Name, expr)
	}

	switch c.Kind() {
	case KindInt:
		if c.Pointer() {
			return fmt.Sprintf("if %s != nil { values[%q] = strconv.FormatInt(int64(*%s), 10) }", target, c.Name, target)
		}
		return set(fmt.Sprintf("strconv.FormatInt(int64(%s), 10)", target))
	case KindFloat:
		if c.Pointer() {
			return fmt.Sprintf("if %s != nil { values[%q] = strconv.FormatFloat(float64(*%s), 'f', -1, 64) }", target, c.Name, target)
		}
		return set(fmt.Sprintf("strconv.FormatFloat(float64(%s), 'f', -1, 64)", target))
	case KindBool:
		if c.Pointer() {
			return fmt.Sprintf("if %s != nil { values[%q] = checkboxValue(*%s) }", target, c.Name, target)
		}
		return set(fmt.Sprintf("checkboxValue(%s)", target))
	case KindTime:
		if c.Pointer() {
			return set(fmt.Sprintf("timeValue(%s)", target))
		}
		return set(fmt.Sprintf("timeValue(&%s)", target))
	default:
		if c.Pointer() {
			return fmt.Sprintf("if %s != nil { values[%q] = *%s }", target, c.Name, target)
		}
		return set(target)
	}
}

// routeSnippetController builds the route registration snippet of a plain
// controller: the JSON API shape its skeleton actions suggest.
func routeSnippetController(id string) string {
	var sb strings.Builder
	fmt.Fprintln(&sb, "Route registration — add to your route setup (h is the *web.Handler):")
	fmt.Fprintln(&sb)
	fmt.Fprintf(&sb, "\tctl := &%s{}\n", mustControllerName(id))
	fmt.Fprintf(&sb, "\tc := h.Ctrl(ctl)\n")
	fmt.Fprintf(&sb, "\tc.Get(\"/%s\", ctl.ActionIndex).Name(\"%s/index\")\n", id, id)
	fmt.Fprintf(&sb, "\tc.Get(\"/%s/{id:[0-9]+}\", ctl.ActionView).Name(\"%s/view\")\n", id, id)
	fmt.Fprintf(&sb, "\tc.Post(\"/%s\", ctl.ActionCreate).Name(\"%s/create\")\n", id, id)
	fmt.Fprintf(&sb, "\tc.Put(\"/%s/{id:[0-9]+}\", ctl.ActionUpdate).Name(\"%s/update\")\n", id, id)
	fmt.Fprintf(&sb, "\tc.Delete(\"/%s/{id:[0-9]+}\", ctl.ActionDelete).Name(\"%s/delete\")", id, id)
	return sb.String()
}

// routeSnippetCrud builds the CRUD route registration snippet: the five
// generated actions behind six routes (save serves create and update).
func routeSnippetCrud(data *crudData) string {
	var sb strings.Builder
	fmt.Fprintln(&sb, "Route registration — add to your route setup (h is the *web.Handler):")
	fmt.Fprintln(&sb)
	fmt.Fprintf(&sb, "\tctl := &%s{}\n", data.Controller)
	fmt.Fprintf(&sb, "\tc := h.Ctrl(ctl)\n")
	fmt.Fprintf(&sb, "\tc.Get(\"/%s\", ctl.ActionList).Name(\"%s/list\")\n", data.ID, data.ID)
	fmt.Fprintf(&sb, "\tc.Get(\"/%s/new\", ctl.ActionNew).Name(\"%s/new\")\n", data.ID, data.ID)
	fmt.Fprintf(&sb, "\tc.Post(\"/%s\", ctl.ActionSave).Name(\"%s/save\")\n", data.ID, data.ID)
	fmt.Fprintf(&sb, "\tc.Get(\"/%s/%s/edit\", ctl.ActionEdit).Name(\"%s/edit\")\n", data.ID, data.PK.Pattern, data.ID)
	fmt.Fprintf(&sb, "\tc.Post(\"/%s/%s\", ctl.ActionSave).Name(\"%s/update\")\n", data.ID, data.PK.Pattern, data.ID)
	fmt.Fprintf(&sb, "\tc.Post(\"/%s/%s/delete\", ctl.ActionDelete).Name(\"%s/delete\")", data.ID, data.PK.Pattern, data.ID)
	return sb.String()
}

// viewSnippetCrud builds the view registration snippet of the CRUD pages.
func viewSnippetCrud(data *crudData) string {
	var sb strings.Builder
	fmt.Fprintln(&sb, "View registration — add to your view setup (r is the *view.Registry):")
	fmt.Fprintln(&sb)
	fmt.Fprintf(&sb, "\tr.Register(\"%s/list\", view.Typed(%sListPage))\n", data.ID, data.ModelVar)
	fmt.Fprintf(&sb, "\tr.Register(\"%s/form\", view.Typed(%sFormPage))", data.ID, data.ModelVar)
	return sb.String()
}

// mustControllerName resolves a controller id to its struct name; the id was
// validated before the snippet is built.
func mustControllerName(id string) string {
	name, _ := controllerName(id)
	return name
}
