package web

import (
	"net/http"

	"github.com/goyii2/framework/base"
)

// Handler is the application's HTTP entry point: it mounts controller
// actions on URL patterns, runs the request pipeline around them and
// implements http.Handler so it plugs directly into net/http servers:
//
//	h := web.NewHandler(app)
//	h.Handle("GET /post/view", postCtrl.ActionView)
//	http.ListenAndServe(app.Config().Web.Listen, h)
//
// Per request the pipeline is: trigger EventBeforeRequest, dispatch to the
// mounted action with a fresh Context carrying the application, render any
// returned error (format negotiated from Accept, panics recovered as 500),
// then trigger EventAfterRequest.
type Handler struct {
	app *base.Application
	mux *http.ServeMux
}

// NewHandler creates the HTTP entry point serving app.
func NewHandler(app *base.Application) *Handler {
	return &Handler{app: app, mux: http.NewServeMux()}
}

// Handle mounts action on the URL pattern (net/http ServeMux syntax,
// including method prefixes such as "GET /post/view"). Full routing
// (parameter syntax, naming, convention fallback, module prefixes) is
// layered on separately.
func (h *Handler) Handle(pattern string, action Action) {
	h.mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
		c := FromContext(r.Context())
		if c == nil {
			// Reached without the pipeline in front (e.g. the mux was
			// used standalone); there is no request Context to run in.
			http.Error(w, "web: action invoked outside the application handler", http.StatusInternalServerError)
			return
		}
		// Keep the dispatched request so later routing layers can attach
		// match information (wildcard values) to it.
		c.r = r
		c.err = action(c)
	})
}

// ServeHTTP runs the request pipeline around the mounted actions: create
// the request Context (carrying the application), trigger
// EventBeforeRequest, dispatch, render any returned error (format
// negotiated from Accept, panics recovered as 500), then trigger
// EventAfterRequest with the final response status. Pattern misses fall
// through to the mux's own 404 until the routing layer takes over
// unknown-path handling.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	r = r.WithContext(h.app.WithContext(r.Context()))
	c := newContext(h.app, w, r)

	// Registered first so it runs last: afterRequest fires after panic
	// recovery has rendered its error response.
	defer h.app.Trigger(c.r.Context(), base.EventAfterRequest, nil)
	defer func() {
		if p := recover(); p != nil {
			h.app.Log().Error("panic recovered", "method", r.Method, "path", r.URL.Path, "panic", p)
			h.renderError(c, ServerError("internal server error"))
		}
	}()

	h.app.Trigger(c.r.Context(), base.EventBeforeRequest, nil)
	h.mux.ServeHTTP(c.w, c.r)
	if c.err != nil {
		h.renderError(c, c.err)
	}
}
