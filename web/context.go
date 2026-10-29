package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/goyii2/framework/base"
	"github.com/goyii2/framework/view"
)

// Context carries one HTTP request through the action pipeline: it exposes
// the request, the response writer (with the status written so far), the
// owning application and response helpers. The Handler creates a fresh
// Context for every request.
type Context struct {
	app *base.Application
	w   *responseWriter
	r   *http.Request

	// err holds the error returned by the action so the dispatch wrapper
	// can bubble it up to the pipeline without changing http.Handler
	// signatures.
	err error

	// format is the response format the ContentNegotiator middleware
	// negotiated for this request ("json", "html", ...); empty means no
	// negotiation ran and error rendering falls back to Accept sniffing.
	format string

	// csrf is the double-submit token the Csrf middleware established for
	// this request (read from the cookie or freshly generated); empty when
	// the filter did not run or is disabled.
	csrf string

	// session is the request-scoped session the session middleware opened;
	// user is the authentication state bound to it. Nil when the middleware
	// did not run.
	session *Session
	user    *User

	// auth is the authorization backend (AccessChecker) the handler was
	// configured with; nil leaves access control on its identity-role
	// defaults. The User and AccessControl read it.
	auth AccessChecker

	// views is the named-view table Context.Render resolves through; the
	// owning Handler installs its registry, nil outside the pipeline.
	views *view.Registry
	// layout is the page frame override for the next render (nil means the
	// built-in view.Shell); bare suppresses framing entirely (partial
	// content); title is the page title layouts read.
	layout view.Layout
	bare   bool
	title  string
}

// ResponseFormat returns the response format negotiated for this request by
// the ContentNegotiator middleware ("" when none ran). Unified error
// rendering honors it: "html" renders the HTML error page, any other
// negotiated value renders JSON.
func (c *Context) ResponseFormat() string { return c.format }

// CsrfToken returns the CSRF token active for this request: the value the
// Csrf middleware read from (or issued into) the double-submit cookie.
// Form rendering embeds it in the hidden field the filter verifies;
// without the filter it returns "".
func (c *Context) CsrfToken() string { return c.csrf }

// Session returns the request-scoped session; nil when the session
// middleware is not in the chain. The HTTP entry point installs it by
// default, so applications assembling a custom chain are the nil case.
func (c *Context) Session() *Session { return c.session }

// User returns the request-scoped authentication state (the current
// identity, Login/Logout); nil when the session middleware is not in the
// chain. The stateless authentication middlewares install a session-less
// User on demand, so identity-bearing code works either way.
func (c *Context) User() *User { return c.user }

// SetError records err as the request's outcome without writing a response
// yet: the unified error renderer produces the final status, body and
// format once the middleware chain unwinds. It is the rejection path for
// middleware — record the error and return without calling next.
func (c *Context) SetError(err error) { c.err = err }

// Err returns the error recorded for this request — set by an action's
// return or a middleware's rejection — or nil when the request succeeded.
// The unified error renderer answers it after the chain unwinds; response
// captures inside the chain (the debug toolbar) consult this to leave the
// response to the renderer instead of committing their own.
func (c *Context) Err() error { return c.err }

// App returns the application serving this request.
func (c *Context) App() *base.Application { return c.app }

// Context returns the underlying context.Context of the request; it carries
// the application, so base.FromContext(c.Context()) resolves the same
// instance as c.App().
func (c *Context) Context() context.Context { return c.r.Context() }

// Request returns the HTTP request of the current request cycle.
func (c *Context) Request() *http.Request { return c.r }

// Response returns the response writer. The writer records the status code,
// readable with StatusCode.
func (c *Context) Response() http.ResponseWriter { return c.w }

// SwapWriter replaces the writer actions and later middleware write through
// — the Context writer — with one wrapping w, and returns the function
// restoring the previous writer. It is the seam response-capturing code
// outside the web package (the debug toolbar) uses: actions write through
// the Context writer, so wrapping only the writer handed down the chain
// captures nothing (PageCache performs the same swap in-package). Swap
// before the response starts and restore it before the unified error
// renderer runs, exactly like PageCache.render does.
func (c *Context) SwapWriter(w http.ResponseWriter) func() {
	prev := c.w
	c.w = &responseWriter{ResponseWriter: w, status: http.StatusOK}
	return func() { c.w = prev }
}

// StatusCode returns the status code written so far; before anything is
// written it is 200 OK.
func (c *Context) StatusCode() int { return c.w.status }

// Param returns the value of the path parameter matched by the mounted
// pattern's wildcard (e.g. "id" for "/post/{id:[0-9]+}"); parameters the
// pattern does not declare return "".
func (c *Context) Param(name string) string { return c.r.PathValue(name) }

// ParamInt parses the path parameter as an int in base 10. Patterns with
// digit-only wildcards still need this: values longer than the machine int
// parse-fail and must reach the action as an error.
func (c *Context) ParamInt(name string) (int, error) {
	v, err := strconv.Atoi(c.Param(name))
	if err != nil {
		return 0, fmt.Errorf("web: path parameter %q: %w", name, err)
	}
	return v, nil
}

// ParamInt64 parses the path parameter as a 64-bit int in base 10.
func (c *Context) ParamInt64(name string) (int64, error) {
	v, err := strconv.ParseInt(c.Param(name), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("web: path parameter %q: %w", name, err)
	}
	return v, nil
}

// ParamBool parses the path parameter as a boolean (strconv.ParseBool
// syntax: 1, t, T, TRUE, true, True, 0, f, F, FALSE, false, False).
func (c *Context) ParamBool(name string) (bool, error) {
	v, err := strconv.ParseBool(c.Param(name))
	if err != nil {
		return false, fmt.Errorf("web: path parameter %q: %w", name, err)
	}
	return v, nil
}

// JSON writes v as the response body with an application/json content type.
func (c *Context) JSON(v any) error {
	c.w.Header().Set("Content-Type", "application/json; charset=utf-8")
	return json.NewEncoder(c.w).Encode(v)
}

// JSONStatus writes v as the response body with an explicit status code and
// an application/json content type — the reply shape for payloads rejected
// before normal processing, such as model validation failures returned with
// 422 and their field -> messages map.
func (c *Context) JSONStatus(status int, v any) error {
	c.w.Header().Set("Content-Type", "application/json; charset=utf-8")
	c.w.WriteHeader(status)
	return json.NewEncoder(c.w).Encode(v)
}

// Redirect replies with the redirect status and a Location header pointing
// at url.
func (c *Context) Redirect(status int, url string) error {
	http.Redirect(c.w, c.r, url, status)
	return nil
}

// ctxKey is the unexported context key under which the *Context travels
// through the request context, so dispatch can hand it back to actions and
// event handlers can recover it with web.FromContext.
type ctxKey struct{}

// FromContext returns the request-scoped web.Context carried by ctx, or nil
// when ctx does not belong to a web request (e.g. a CLI context).
func FromContext(ctx context.Context) *Context {
	c, _ := ctx.Value(ctxKey{}).(*Context)
	return c
}

// newContext builds the per-request Context: w is wrapped with a status
// recorder and the result is stashed into the request context. r must
// already carry the application (base.WithContext).
func newContext(app *base.Application, w http.ResponseWriter, r *http.Request) *Context {
	c := &Context{
		app: app,
		w:   &responseWriter{ResponseWriter: w, status: http.StatusOK},
		r:   r,
	}
	c.r = r.WithContext(context.WithValue(r.Context(), ctxKey{}, c))
	return c
}

// responseWriter records the status code and whether the response has
// started. The error handler uses this to skip re-rendering after a partial
// write, and event handlers can read the final status.
type responseWriter struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (w *responseWriter) WriteHeader(code int) {
	if w.wrote {
		// Suppress duplicate headers instead of forwarding them; the
		// first status line is the one the client sees.
		return
	}
	w.status, w.wrote = code, true
	w.ResponseWriter.WriteHeader(code)
}

func (w *responseWriter) Write(b []byte) (int, error) {
	w.wrote = true
	return w.ResponseWriter.Write(b)
}

// Unwrap lets http.ResponseController reach the underlying writer's
// optional interfaces (Flusher, Hijacker, ...).
func (w *responseWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
