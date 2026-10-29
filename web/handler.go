package web

import (
	"net/http"

	"github.com/goyii2/framework/base"
	"github.com/goyii2/framework/url"
	"github.com/goyii2/framework/view"
)

// Handler is the application's HTTP entry point: it mounts controller
// actions on URL patterns, runs the request pipeline around them and
// implements http.Handler so it plugs directly into net/http servers:
//
//	h := web.NewHandler(app)
//	h.Get("/post/{id:[0-9]+}", postCtrl.ActionView).Name("post/view")
//	http.ListenAndServe(app.Config().Web.Listen, h)
//
// Per request the pipeline is: trigger EventBeforeRequest, run the global
// middleware chain around routing, dispatch to the mounted action with a
// fresh Context carrying the application (wrapped with the middleware of
// the controller — when mounted through Ctrl or resolved by convention —
// and of the module owning the route's prefix, see ModuleRoutes.Use),
// render any returned error (format negotiated from Accept, panics
// recovered as 500), then trigger EventAfterRequest. Requests no explicit
// pattern matches land on the convention fallback (see router.go); a path
// bound explicitly under other verbs answers 405 with Allow first.
type Handler struct {
	app *base.Application
	mux *http.ServeMux

	// urls is the named-route table powering reverse URL generation.
	urls *url.Manager
	// views is the named-view table powering Context.Render and the error
	// page overrides; exposed for registration while assembling the
	// application.
	views *view.Registry
	// conventionFallback enables /{controller}/{action} resolution for
	// paths no explicit route matched; on by default, configurable off.
	conventionFallback bool

	// session is the session manager installed in the global chain; exposed
	// for configuration (most notably wiring IdentityFinder) before serving
	// traffic.
	session *SessionManager

	// auth is the authorization backend (AccessChecker) the request pipeline
	// consults; installed through SetAuthManager before serving traffic.
	auth AccessChecker

	// chain holds the application-wide middleware, first registered
	// outermost (first to see each request); pipe is the mux wrapped by it.
	// The chain starts with the CSRF middleware unless configuration
	// disables it.
	chain []Middleware
	pipe  http.Handler

	// verbBindings remembers the path patterns mounted explicitly with the
	// verbs bound to each, so a request hitting a bound path with an
	// unbound verb can be answered 405 + Allow instead of silently falling
	// through to the convention routes.
	verbBindings map[string]*verbBinding

	// moduleMW holds the middleware declared per module through
	// ModuleRoutes.Use; it wraps every route below the module prefix —
	// explicit mounts and convention routes alike, submodules included.
	// Written while assembling, read per request.
	moduleMW map[*base.Module][]Middleware
}

// NewHandler creates the HTTP entry point serving app. The convention
// fallback follows the application configuration (on unless explicitly
// disabled) and can be flipped later with SetConventionFallback. CSRF
// validation and the session middleware are installed in the global chain
// unless the configuration disables them (see Csrf and SessionManager).
func NewHandler(app *base.Application) *Handler {
	h := &Handler{
		app:                app,
		mux:                http.NewServeMux(),
		urls:               url.NewManager(),
		views:              view.NewRegistry(),
		conventionFallback: !app.Config().Web.DisableConventionFallback,
		verbBindings:       map[string]*verbBinding{},
		moduleMW:           map[*base.Module][]Middleware{},
	}
	h.rebuildPipe()
	if sc := app.Config().Session; sc.Backend == "redis" {
		// A session section without redis values inherits the top-level
		// section and shares the application's connection; a non-empty one
		// resolves to a dedicated connection (see Application.RedisFor).
		h.session = NewSessionManagerWithRedis(sc, app.RedisFor(sc.Redis))
	} else {
		h.session = NewSessionManager(sc)
	}
	h.Use(h.session.Middleware())
	h.Use(Language())
	if !app.Config().Web.DisableCsrf {
		h.Use(Csrf{}.Middleware())
	}
	h.mountFallback()
	return h
}

// SessionManager returns the session manager installed in the global chain.
// Configure it (backend, cookie attributes, IdentityFinder) before serving
// traffic; mutations during serving are not synchronized.
func (h *Handler) SessionManager() *SessionManager { return h.session }

// SetAuthManager installs the authorization backend the request pipeline
// consults: with it, User.Can and AccessControl rules check roles and
// permissions against the backend (the framework's rbac.DbManager plugs in
// directly) instead of the identity's own role list. Without it the
// role-callback behavior stays in place. Configure before serving traffic;
// mutations during serving are not synchronized.
func (h *Handler) SetAuthManager(a AccessChecker) { h.auth = a }

// AuthManager returns the installed authorization backend, nil when none
// was set.
func (h *Handler) AuthManager() AccessChecker { return h.auth }

// URLs returns this handler's named-route table. Install it as the
// package-level default (url.SetDefault) to let url.To resolve this
// application's routes.
func (h *Handler) URLs() *url.Manager { return h.urls }

// Views returns this handler's view registry, the named-view table behind
// Context.Render. Register the application's views while assembling, before
// serving traffic; the names "error/<status>" and "error" override the
// unified error handler's HTML pages.
func (h *Handler) Views() *view.Registry { return h.views }

// SetConventionFallback toggles /{controller}/{action} resolution for
// paths no explicit route matched. Explicit routes are unaffected either
// way; call before serving in production code.
func (h *Handler) SetConventionFallback(enabled bool) { h.conventionFallback = enabled }

// Handle mounts action on the URL pattern (net/http ServeMux syntax,
// including method prefixes such as "GET /post/view"); the Get/Post/...
// helpers wrap it for method-specific mounts and return a Route handle
// that can carry a name for reverse URL generation. Wildcards may carry
// constraints, "{id:[0-9]+}", which the mux itself does not accept — they
// are rewritten to plain wildcards and enforced per request (see
// expandWildcards).
func (h *Handler) Handle(pattern string, action Action) {
	clean, constraints := expandWildcards(pattern)
	h.recordVerbs(pattern, constraints)
	h.mux.HandleFunc(clean, func(w http.ResponseWriter, r *http.Request) {
		c := FromContext(r.Context())
		if c == nil {
			// Reached without the pipeline in front (e.g. the mux was
			// used standalone); there is no request Context to run in.
			http.Error(w, "web: action invoked outside the application handler", http.StatusInternalServerError)
			return
		}
		for _, wc := range constraints {
			if !wc.re.MatchString(r.PathValue(wc.name)) {
				// The value misses this binding's constraints, so the
				// explicit route does not apply after all: finish the
				// request through the convention fallback (a 404 when it
				// cannot resolve the path either).
				h.serveConvention(w, r)
				return
			}
		}
		// Keep the dispatched request so later routing layers can attach
		// match information (wildcard values) to it.
		c.r = r
		c.err = action(c)
	})
}

// ServeHTTP runs the request pipeline around the mounted actions: create
// the request Context (carrying the application), trigger
// EventBeforeRequest, run the global middleware chain around routing,
// render any returned error (format negotiated from Accept, panics
// recovered as 500), then trigger EventAfterRequest with the final response
// status. Requests no pattern matches are finished by the
// convention-fallback handler, so every miss renders a negotiated 404 —
// except paths explicitly bound under other verbs, which answer 405 with an
// Allow header.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	r = r.WithContext(h.app.WithContext(r.Context()))
	c := newContext(h.app, w, r)
	c.views = h.views
	c.auth = h.auth

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
	h.pipe.ServeHTTP(c.w, c.r)
	if c.err != nil {
		h.renderError(c, c.err)
	}
}
