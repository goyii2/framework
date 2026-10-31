// Package form binds HTTP request values into form models: plain structs
// whose fields opt in with a `form` tag. It replaces Yii2's massive
// assignment (Model.load) with an explicit, tag-driven allow list, and feeds
// the same per-field error shape the validators package produces, so a bound
// model fails and re-renders exactly like a validated one.
//
// # Sources
//
// Bind reads an x-www-form-urlencoded request body; BindQuery reads the URL
// query string. Both run the same assignment core, and each reads its own
// source only: a form body never picks up query parameters, a query binding
// never sees body values. A request body of any other content type binds
// nothing and reports no failure — dispatching other body formats is a
// caller concern. An empty form or an empty query is a valid binding: the
// model keeps its current field values and no error is reported.
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
// # Errors
//
// A value that fails to parse is a field error, not a request failure: each
// failing key collects its message, every other field still assigns, and the
// result is a *validators.Errors under the "bind" scenario — nil when
// everything parsed. Messages are stable ids ("not a valid integer",
// "not a valid date-time", ...) translated through the request context's i18n
// source under the validators' message category, exactly like rule messages;
// without a source they render verbatim. A body that cannot be decoded as
// form pairs at all is reported under the record-level "$root" key instead of
// a field.
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
