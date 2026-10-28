package web

import (
	"errors"
	"net/http"
)

// Middleware is the standard net/http middleware signature: it receives the
// next handler in the chain and returns a handler wrapping it. Middlewares
// run inside the application pipeline, so they can reach the request's
// *Context through FromContext(r.Context()) to reject with the unified
// error renderer (set the error with writeError, do not call next) or to
// consult negotiated state.
type Middleware func(http.Handler) http.Handler

// Chain composes middlewares into one Middleware: the first argument is
// outermost, so requests flow through the middlewares in the given order.
func Chain(mws ...Middleware) Middleware {
	return func(next http.Handler) http.Handler {
		for i := len(mws) - 1; i >= 0; i-- {
			next = mws[i](next)
		}
		return next
	}
}

// writeError hands err over to the unified error renderer. Inside the
// application pipeline the error is recorded on the request Context and
// rendered once the chain unwinds, in the negotiated format; a middleware
// calling it must return without invoking next. Outside the pipeline (a
// middleware reused standalone) it degrades to a plain status line.
func writeError(w http.ResponseWriter, r *http.Request, err error) {
	c := FromContext(r.Context())
	if c == nil {
		status := http.StatusInternalServerError
		var he HTTPError
		if errors.As(err, &he) {
			status = he.Status()
		}
		http.Error(w, err.Error(), status)
		return
	}
	c.SetError(err)
}

// Use appends middleware to the application-wide chain. Every request
// passes through the chain (registration order: the first Use call is
// outermost) before routing, so global middleware always runs before any
// controller middleware. Call before serving traffic.
func (h *Handler) Use(mws ...Middleware) {
	h.chain = append(h.chain, mws...)
	h.rebuildPipe()
}

// rebuildPipe re-wraps the mux with the current global chain.
func (h *Handler) rebuildPipe() {
	next := http.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.mux.ServeHTTP(w, r)
	}))
	for i := len(h.chain) - 1; i >= 0; i-- {
		next = h.chain[i](next)
	}
	h.pipe = next
}

// controllerMiddlewareSource is the optional hook a controller implements to
// attach middleware to its own actions only:
//
//	func (PostController) Middleware() []web.Middleware { ... }
//
// The middleware wraps each of the controller's mounted actions (explicit
// mounts through Ctrl as well as convention-fallback resolution) and runs
// after the application-wide chain.
type controllerMiddlewareSource interface {
	Middleware() []Middleware
}

// controllerMiddleware collects the middleware a controller declares, nil
// when it opts out (no such method).
func controllerMiddleware(controller any) []Middleware {
	src, ok := controller.(controllerMiddlewareSource)
	if !ok {
		return nil
	}
	return src.Middleware()
}

// withMiddleware adapts middleware to an Action: the action runs as the
// innermost http.Handler, so the middleware wraps it in the usual way and
// may short-circuit (write a response or reject with writeError) before the
// action ever runs. The error the innermost handler (or a rejecting
// middleware) records on the request Context is returned unchanged.
func withMiddleware(action Action, mws []Middleware) Action {
	if len(mws) == 0 {
		return action
	}
	inner := http.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if c := FromContext(r.Context()); c != nil {
			c.r = r
			c.err = action(c)
		}
	}))
	for i := len(mws) - 1; i >= 0; i-- {
		inner = mws[i](inner)
	}
	return func(c *Context) error {
		inner.ServeHTTP(c.w, c.r)
		return c.err
	}
}

// ControllerRoutes mounts the actions of one controller: every action
// mounted through it is wrapped with the controller's declared middleware
// (see controllerMiddlewareSource), while plain Handler mounts stay
// unwrapped.
type ControllerRoutes struct {
	h   *Handler
	mws []Middleware
}

// Ctrl returns a route registrar bound to one controller instance:
//
//	h.Ctrl(postCtrl).Get("/post/view", postCtrl.ActionView)
//
// and the convention fallback resolves the same controller's middleware, so
// both mounting styles run identical controller middleware.
func (h *Handler) Ctrl(controller any) *ControllerRoutes {
	return &ControllerRoutes{h: h, mws: controllerMiddleware(controller)}
}

// wrap binds the controller middleware around action.
func (cr *ControllerRoutes) wrap(action Action) Action {
	return withMiddleware(action, cr.mws)
}

// Get mounts action on GET requests to path, wrapped with the controller's
// middleware.
func (cr *ControllerRoutes) Get(path string, action Action) *Route {
	return cr.h.route(http.MethodGet, path, cr.wrap(action))
}

// Post mounts action on POST requests to path, wrapped with the controller's
// middleware.
func (cr *ControllerRoutes) Post(path string, action Action) *Route {
	return cr.h.route(http.MethodPost, path, cr.wrap(action))
}

// Put mounts action on PUT requests to path, wrapped with the controller's
// middleware.
func (cr *ControllerRoutes) Put(path string, action Action) *Route {
	return cr.h.route(http.MethodPut, path, cr.wrap(action))
}

// Delete mounts action on DELETE requests to path, wrapped with the
// controller's middleware.
func (cr *ControllerRoutes) Delete(path string, action Action) *Route {
	return cr.h.route(http.MethodDelete, path, cr.wrap(action))
}

// Patch mounts action on PATCH requests to path, wrapped with the
// controller's middleware.
func (cr *ControllerRoutes) Patch(path string, action Action) *Route {
	return cr.h.route(http.MethodPatch, path, cr.wrap(action))
}

// Head mounts action on HEAD requests to path, wrapped with the controller's
// middleware.
func (cr *ControllerRoutes) Head(path string, action Action) *Route {
	return cr.h.route(http.MethodHead, path, cr.wrap(action))
}

// Options mounts action on OPTIONS requests to path, wrapped with the
// controller's middleware.
func (cr *ControllerRoutes) Options(path string, action Action) *Route {
	return cr.h.route(http.MethodOptions, path, cr.wrap(action))
}

// Handle mounts action on an arbitrary pattern (method prefix allowed),
// wrapped with the controller's middleware.
func (cr *ControllerRoutes) Handle(pattern string, action Action) {
	cr.h.Handle(pattern, cr.wrap(action))
}
