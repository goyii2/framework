package web_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/goyii2/framework/base"
	"github.com/goyii2/framework/web"
)

// traceMiddleware appends tag to a shared trace whenever it processes a
// request, exposing the exact execution order of the chain.
func traceMiddleware(trace *syncBuffer, tag string) web.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			trace.add(tag)
			next.ServeHTTP(w, r)
		})
	}
}

// syncBuffer is a concurrency-safe string accumulator (requests may run on
// parallel test goroutines through a real server).
type syncBuffer struct {
	mu sync.Mutex
	b  []string
}

func (s *syncBuffer) add(v string) {
	s.mu.Lock()
	s.b = append(s.b, v)
	s.mu.Unlock()
}

func (s *syncBuffer) values() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.b...)
}

// middlewareController declares controller middleware marking responses.
type middlewareController struct {
	web.Controller
	trace *syncBuffer
}

func (c middlewareController) Middleware() []web.Middleware {
	return []web.Middleware{
		func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				c.trace.add("controller")
				w.Header().Set("X-Controller-Middleware", "yes")
				next.ServeHTTP(w, r)
			})
		},
	}
}

func (middlewareController) ActionPing(c *web.Context) error {
	return c.JSON(map[string]string{"m": "ping"})
}

func (middlewareController) ActionEcho(c *web.Context) error {
	return c.JSON(map[string]string{"m": "echo"})
}

// plainController opts out of middleware entirely.
type plainController struct {
	web.Controller
}

func (plainController) ActionPing(c *web.Context) error {
	return c.JSON(map[string]string{"m": "plain"})
}

// newMiddlewareApp wires the global chain, one middleware-declaring
// controller (explicit mount + convention) and one plain controller.
func newMiddlewareApp(trace *syncBuffer) (*web.Handler, *base.Application) {
	app := base.New()
	h := web.NewHandler(app, web.WithoutCsrf())
	h.Use(traceMiddleware(trace, "global-a"), traceMiddleware(trace, "global-b"))

	tagged := middlewareController{trace: trace}
	h.Ctrl(tagged).Get("/tagged/ping", tagged.ActionPing)
	if err := app.RegisterController("tagged", tagged); err != nil {
		panic(err)
	}
	h.Get("/plain/ping", plainController{}.ActionPing)
	return h, app
}

func TestGlobalMiddlewareRunsInRegistrationOrder(t *testing.T) {
	trace := &syncBuffer{}
	h, _ := newMiddlewareApp(trace)

	res := req(t, h, http.MethodGet, "/plain/ping")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	want := []string{"global-a", "global-b"}
	if got := trace.values(); !equalStrings(got, want) {
		t.Errorf("global chain order = %v, want %v", got, want)
	}
}

func TestControllerMiddlewareRunsAfterGlobalChain(t *testing.T) {
	trace := &syncBuffer{}
	h, _ := newMiddlewareApp(trace)

	res := req(t, h, http.MethodGet, "/tagged/ping")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	want := []string{"global-a", "global-b", "controller"}
	if got := trace.values(); !equalStrings(got, want) {
		t.Errorf("middleware order = %v, want %v (global before controller)", got, want)
	}
}

func TestControllerMiddlewareOnlyAffectsItsOwnActions(t *testing.T) {
	trace := &syncBuffer{}
	h, _ := newMiddlewareApp(trace)

	cases := []struct {
		target  string
		marked  string
		traceOK []string
	}{
		{"/tagged/ping", "yes", []string{"global-a", "global-b", "controller"}},
		{"/tagged/echo", "yes", nil}, // convention-resolved action, marks checked below
		{"/plain/ping", "", []string{"global-a", "global-b"}},
	}
	for _, tc := range cases {
		res := req(t, h, http.MethodGet, tc.target)
		if res.StatusCode != http.StatusOK {
			t.Fatalf("%s status = %d, want 200", tc.target, res.StatusCode)
		}
		if got := res.Header.Get("X-Controller-Middleware"); got != tc.marked {
			t.Errorf("%s X-Controller-Middleware = %q, want %q", tc.target, got, tc.marked)
		}
		if tc.traceOK != nil && !equalStrings(trace.values(), tc.traceOK) {
			t.Errorf("%s trace = %v, want %v", tc.target, trace.values(), tc.traceOK)
		}
		trace.b = nil
	}

	// An unroutable path never reaches any controller middleware.
	req(t, h, http.MethodGet, "/nowhere/at/all")
	for _, tag := range trace.values() {
		if tag == "controller" {
			t.Error("controller middleware ran for an unrouted path")
		}
	}
}

// guardedController's middleware rejects every request before the action.
type guardedController struct {
	web.Controller
	ran *bool
}

func (g guardedController) Middleware() []web.Middleware {
	return []web.Middleware{
		func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				c := web.FromContext(r.Context())
				if c != nil {
					c.Response().Header().Set("X-Guard", "on")
					c.SetError(web.Forbidden("blocked by test middleware"))
				}
			})
		},
	}
}

func (g guardedController) ActionIndex(c *web.Context) error {
	*g.ran = true
	return c.JSON(map[string]string{"m": "index"})
}

func TestControllerMiddlewareRejectsWithUnifiedError(t *testing.T) {
	app := base.New()
	h := web.NewHandler(app, web.WithoutCsrf())

	ran := false
	ctrl := guardedController{ran: &ran}
	h.Ctrl(ctrl).Get("/guarded", ctrl.ActionIndex)

	r := httptest.NewRequest(http.MethodGet, "/guarded", nil)
	r.Header.Set("Accept", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	res := rec.Result()

	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", res.StatusCode)
	}
	if res.Header.Get("X-Guard") != "on" {
		t.Error("middleware could not reach the web Context's response writer")
	}
	if ran {
		t.Error("action ran although middleware rejected the request")
	}
	var body map[string]any
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("decoding body: %v", err)
	}
	if body["status"] != float64(http.StatusForbidden) {
		t.Errorf("error body = %v, want the unified error shape", body)
	}
}

func TestMiddlewareSeesRequestWebContext(t *testing.T) {
	app := base.New()
	h := web.NewHandler(app, web.WithoutCsrf())
	h.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c := web.FromContext(r.Context())
			if c == nil {
				t.Error("middleware found no web Context")
			} else if c.App() != app {
				t.Error("middleware Context carries the wrong application")
			}
			next.ServeHTTP(w, r)
		})
	})
	h.Get("/probe", func(c *web.Context) error { return c.JSON(map[string]bool{"ok": true}) })

	res := req(t, h, http.MethodGet, "/probe")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
}

// TestModuleMiddlewareWrapsModuleRoutes covers ModuleRoutes.Use — the
// Yii2 module beforeAction capability: middleware declared on a module
// wraps its explicit mounts and its convention routes, ancestor modules
// run outermost on submodule routes, and nothing leaks outside the prefix.
// It wraps at request time, so declaring middleware after mounting routes
// still applies to them.
func TestModuleMiddlewareWrapsModuleRoutes(t *testing.T) {
	trace := &syncBuffer{}
	app := base.New()
	h := web.NewHandler(app, web.WithoutCsrf())
	h.Use(traceMiddleware(trace, "global"))

	admin, err := base.NewModule(app, "/admin")
	if err != nil {
		t.Fatalf("creating /admin module: %v", err)
	}
	if err := app.RegisterModule(admin); err != nil {
		t.Fatalf("registering /admin module: %v", err)
	}
	report, err := base.NewModule(app, "/admin/report")
	if err != nil {
		t.Fatalf("creating /admin/report module: %v", err)
	}
	if err := admin.RegisterModule(report); err != nil {
		t.Fatalf("nesting /admin/report module: %v", err)
	}

	reportRoutes, err := h.Module("/admin/report")
	if err != nil {
		t.Fatalf("module routes: %v", err)
	}
	// Mounted before any module middleware on purpose: the wrap resolves
	// per request, so later Use calls must still cover this route.
	reportRoutes.Get("/summary", func(c *web.Context) error {
		return c.JSON(map[string]string{"m": "summary"})
	})

	adminRoutes, err := h.Module("/admin")
	if err != nil {
		t.Fatalf("module routes: %v", err)
	}
	adminRoutes.Use(traceMiddleware(trace, "admin-mw"))
	reportRoutes.Use(traceMiddleware(trace, "report-mw"))

	tagged := middlewareController{trace: trace}
	if err := admin.RegisterController("tagged", tagged); err != nil {
		t.Fatalf("registering module controller: %v", err)
	}
	h.Get("/outside", plainController{}.ActionPing)

	cases := []struct {
		target string
		want   []string
	}{
		// Explicit submodule route: the ancestors' middleware wraps
		// outermost, the submodule's innermost.
		{"/admin/report/summary", []string{"global", "admin-mw", "report-mw"}},
		// Convention route of the module: module middleware wraps the
		// controller's, matching the explicit-mount order.
		{"/admin/tagged/ping", []string{"global", "admin-mw", "controller"}},
		// Outside the module prefix none of it runs.
		{"/outside", []string{"global"}},
	}
	for _, tc := range cases {
		res := req(t, h, http.MethodGet, tc.target)
		if res.StatusCode != http.StatusOK {
			t.Fatalf("%s status = %d, want 200", tc.target, res.StatusCode)
		}
		if got := trace.values(); !equalStrings(got, tc.want) {
			t.Errorf("%s trace = %v, want %v", tc.target, got, tc.want)
		}
		trace.b = nil
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
