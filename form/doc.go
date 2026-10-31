// Package form binds HTTP request values into form models: plain structs
// whose fields opt in with a `form` tag. It replaces Yii2's massive
// assignment (Model.load) with an explicit, tag-driven allow list, and feeds
// the same per-field error shape the validators package produces, so a bound
// model fails and re-renders exactly like a validated one.
//
// # Sources
//
// Bind dispatches on the body's declared content type: an application/json
// body goes through the JSON source, an x-www-form-urlencoded body through
// the form source. BindQuery reads the URL query string. All three run the
// same assignment core, and each reads its own source only: a form body
// never picks up query parameters, a query binding never sees body values, a
// JSON body never sees either. A body declaring any other content type — or
// declaring none at all — binds nothing and reports no failure: only bodies
// that explicitly declare a supported type are routed to a source. An empty
// body or query, like an empty JSON object, is a valid binding that submits
// no values: the model keeps its current field values and no error is
// reported.
//
// # Security model: the tag is the whitelist
//
// Only exported fields carrying a `form:"name"` tag are ever assigned. An
// untagged exported field stays untouched however the request is shaped, a
// `form:"-"` tag opts a field out, and request keys with no tagged field —
// hidden inputs such as _csrf among them — are silently ignored. There is no
// path by which a request sets a field the model did not explicitly declare,
// so over-posting attacks (submitting fields the action never meant to edit,
// like an isAdmin flag or a price column) have no entry point.
//
// The JSON source keeps that promise by never decoding the body straight
// into the model: the body is decoded into a generic map and assigned from
// that map through the tag walk, so a JSON key assigns a field only when it
// matches the field's tag exactly. encoding/json's case-insensitive struct
// field matching — where a key "Untagged" or "untagged" reaches an exported
// field with that name — is precisely the hole this detour closes; key case
// variations never bind anything the tag did not declare.
//
// # Supported field types
//
//	string                     every fixed-size int and uint
//	float32, float64           bool
//	[]string                   time.Time (RFC 3339)
//	encoding.TextUnmarshaler   any field whose type decodes itself
//
// time.Time parses RFC 3339 text ("2026-10-31T12:30:00+08:00", fractional
// seconds included). A TextUnmarshaler field receives its value through
// UnmarshalText, letting models parse custom wire formats without new
// framework hooks. A tagged field of any other type is a programming error
// and panics on the first value submitted for it.
//
// A scalar field consumes the first submitted value; a []string field
// consumes all of them, in order. A lone blank value ("age=") means the
// input was left unfilled: text fields take the empty text, every other
// field is treated as not provided and keeps its current value. Judging
// whether a field may stay blank is the validators' job, not the binder's.
//
// # JSON values
//
// JSON carries typed values, and the JSON source takes them at face value:
// numbers feed numeric fields, booleans feed bool fields, and text feeds
// text, time and self-decoding fields. A value whose kind does not fit the
// field — a string sent to an int field, a number to a text field — is a
// per-field type error carrying the message of what the field needed, just
// like an unparseable text in the form source. Numbers keep their literal
// wire form, so an integer field never silently rounds 19.5 or a huge
// literal.
//
// The empty-input verdict carries over: a JSON empty string sent to a
// non-text field, like a lone blank form value, means the input was left
// unfilled and binds nothing; null means the same. Arrays bind []string
// fields — all elements must be text — and an explicit empty array assigns
// an empty list; a lone string mirrors a form carrying one submitted value
// and binds a one-element list. Nested objects bind no scalar field: they
// are a type error under the field's own message, and a TextUnmarshaler
// field decodes itself from the string form of its value.
//
// # Errors
//
// A value that fails to parse is a field error, not a request failure: each
// failing key collects its message, every other field still assigns, and the
// result is a *validators.Errors under the "bind" scenario — nil when
// everything parsed. Messages are stable ids ("not a valid integer",
// "not a valid date-time", ...) translated through the request context's i18n
// source under the validators' message category, exactly like rule messages;
// without a source they render verbatim. A body that cannot be decoded at
// all — form pairs that do not parse, JSON that is not one object (truncated
// text, a non-object value, trailing garbage) — is reported under the
// record-level "$root" key instead of a field.
//
// Wiring a form into an action:
//
//	type LoginForm struct {
//	    Login    string `form:"login"`
//	    Password string `form:"password"`
//	}
//
//	func (c *SiteController) ActionLogin(ctx *web.Context) error {
//	    var f LoginForm
//	    if errs := form.Bind(ctx.Request, &f); errs != nil {
//	        return ctx.HTMLStatus(http.StatusUnprocessableEntity,
//	            loginFormView(f, errs.Fields))
//	    }
//	    // ... authenticate ...
//	}
package form
