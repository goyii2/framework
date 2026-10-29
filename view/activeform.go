package view

import (
	"github.com/a-h/templ"

	"github.com/goyii2/framework/validators"
)

// FieldType selects the input rendering of an ActiveField: the value is the
// HTML input type, except that "textarea" and "select" name the element to
// render instead. An empty Type means FieldText. Values beyond the
// constants pass through as the input type (e.g. "email", "url"), so forms
// are not locked to the common set.
type FieldType string

const (
	FieldText          FieldType = "text"
	FieldTextarea      FieldType = "textarea"
	FieldPassword      FieldType = "password"
	FieldHidden        FieldType = "hidden"
	FieldSelect        FieldType = "select"
	FieldCheckbox      FieldType = "checkbox"
	FieldRadio         FieldType = "radio"
	FieldNumber        FieldType = "number"
	FieldDatetimeLocal FieldType = "datetime-local"
	FieldFile          FieldType = "file"
)

// FieldOption is one choice of a FieldSelect or FieldRadio field. Value is
// what the form submits, Label is what the user sees; both render escaped.
type FieldOption struct {
	Value string
	Label string
}

// ActiveField is one field of an ActiveForm: the label, the input control,
// a hint line and any validation messages, wrapped in a group div. Hidden
// fields render as a bare input without the wrapper.
type ActiveField struct {
	Name  string
	Label string
	Type  FieldType
	Hint  string

	// Options are the choices of FieldSelect and FieldRadio fields, in
	// display order.
	Options []FieldOption

	// Required renders the native required attribute. Native validation
	// attributes are declared explicitly here — the form does not derive
	// them from the model's validation rules.
	Required bool

	// MaxLength renders the native maxlength attribute on the types that
	// support it (text, password, textarea); <= 0 omits it.
	MaxLength int

	// Pattern renders the native pattern attribute on the types that support
	// it (text, password); "" omits it.
	Pattern string

	// Placeholder renders the native placeholder attribute on the text-entry
	// types; "" omits it.
	Placeholder string
}

// ActiveForm renders a server-side form bound to validation state: labels,
// input controls, hints and — after a rejected submission — the field
// messages of a validators failure, with the submitted values refilled.
// Client-side rule export is out of scope: browsers only see the native
// attributes each ActiveField declares.
//
// The typical action validates, then either redirects or re-renders:
//
//	errs := validators.Validate(c.Context(), &model, validators.ScenarioInsert)
//	if errs != nil {
//	    c.Render("post/create", PostFormData{
//	        Form: view.ActiveForm{
//	            Fields: fields, Errors: errs,
//	            Values: postFormValues(c.Request()), // map[string]string per field
//	            CSRF:   c.CsrfToken(),
//	        },
//	    })
//	}
type ActiveForm struct {
	// Action is the form target URL; empty submits to the current URL.
	Action string

	// Method is the form's HTTP method; "" means "post".
	Method string

	// ID prefixes the generated element ids ("{ID}-{field name}") and lands
	// on the form element itself; "" means "form". Set it when a page shows
	// more than one form.
	ID string

	// Fields render in order.
	Fields []ActiveField

	// Errors is the validation failure to echo back: pass the
	// *validators.Errors that validators.Validate or the model save chain
	// returned; nil renders no error markup. A field shows the messages
	// keyed by its Name. Record-level issues (zog's "$root" key) have no
	// field to attach to and are not rendered.
	Errors *validators.Errors

	// Values refills each field, keyed by field Name: after a rejected post
	// echo the submitted values (r.PostForm's first value per key), when
	// building an edit form pass the model's current values. Checkbox reads
	// it as "non-empty means checked"; FieldSelect and FieldRadio mark the
	// option whose Value matches. This is the only refill mechanism — a
	// field-level Value override deliberately does not exist.
	Values map[string]string

	// CSRF embeds the double-submit token as a hidden field named CSRFName,
	// matching what the web.Csrf filter verifies; pass the value from
	// web.Context.CsrfToken(). Empty omits the field.
	CSRF     string
	CSRFName string
}

// activeFormView is the precomputed form the template renders.
type activeFormView struct {
	id       string
	action   string
	method   string
	csrf     string
	csrfName string
	fields   []activeFieldView
}

// activeFieldView is the precomputed field the template renders: the echoed
// value resolved against Values, the flags turned into per-type attributes,
// and this field's error messages.
type activeFieldView struct {
	name        string
	id          string
	label       string
	hint        string
	typ         string
	value       string
	placeholder string
	maxLength   int
	pattern     string
	required    bool
	checked     bool
	options     []activeOptionView
	errors      []string
}

// activeOptionView is one select option / radio button with its selection
// state resolved.
type activeOptionView struct {
	value    string
	label    string
	selected bool
}

// resolveOptions marks the option matching the echoed value — the shared
// choice-resolution of ActiveForm fields and GridView filter selects.
func resolveOptions(options []FieldOption, value string) []activeOptionView {
	resolved := make([]activeOptionView, 0, len(options))
	for _, opt := range options {
		resolved = append(resolved, activeOptionView{value: opt.Value, label: opt.Label, selected: value == opt.Value})
	}
	return resolved
}

func (f ActiveForm) prepared() activeFormView {
	id := f.ID
	if id == "" {
		id = "form"
	}
	method := f.Method
	if method == "" {
		method = "post"
	}
	csrfName := f.CSRFName
	if csrfName == "" {
		csrfName = "_csrf"
	}

	var errs map[string][]string
	if f.Errors != nil {
		errs = f.Errors.Fields
	}

	form := activeFormView{id: id, action: f.Action, method: method, csrf: f.CSRF, csrfName: csrfName}
	form.fields = make([]activeFieldView, 0, len(f.Fields))
	for _, field := range f.Fields {
		form.fields = append(form.fields, f.preparedField(field, id, errs))
	}
	return form
}

// preparedField resolves one field's render state. The optional native
// attributes (placeholder, maxlength, pattern) are dropped for the types
// the HTML spec does not apply them to — a browser would ignore them
// silently, so the markup says only what takes effect.
func (f ActiveForm) preparedField(field ActiveField, formID string, errs map[string][]string) activeFieldView {
	typ := string(field.Type)
	if typ == "" {
		typ = string(FieldText)
	}
	value := f.Values[field.Name]

	v := activeFieldView{
		name:   field.Name,
		id:     formID + "-" + field.Name,
		label:  field.Label,
		hint:   field.Hint,
		typ:    typ,
		errors: errs[field.Name],
	}
	switch FieldType(typ) {
	case FieldSelect, FieldRadio:
		v.options = resolveOptions(field.Options, value)
	case FieldCheckbox:
		// A checkbox submits its value when checked and nothing when not,
		// so any echoed value means it was checked.
		v.checked = value != ""
	case FieldFile:
		// A file input cannot carry a value attribute; the browser owns it.
	default:
		v.value = value
	}

	if FieldType(typ) != FieldHidden {
		v.required = field.Required
	}
	switch FieldType(typ) {
	case FieldText, FieldPassword, FieldNumber, FieldDatetimeLocal, FieldTextarea:
		v.placeholder = field.Placeholder
	}
	switch FieldType(typ) {
	case FieldText, FieldPassword, FieldTextarea:
		v.maxLength = field.MaxLength
	}
	switch FieldType(typ) {
	case FieldText, FieldPassword:
		v.pattern = field.Pattern
	}
	return v
}

// fieldClasses joins the wrapper classes: the Yii2-style group, the field
// hook, and the required / has-error states.
func fieldClasses(v activeFieldView) string {
	return classes("form-group", "field-"+v.name,
		requiredClass(v.required), hasErrorClass(len(v.errors) > 0))
}

func requiredClass(required bool) string {
	if required {
		return "required"
	}
	return ""
}

func hasErrorClass(has bool) string {
	if has {
		return "has-error"
	}
	return ""
}

// Render returns the form component.
func (f ActiveForm) Render() templ.Component { return activeForm(f.prepared()) }
