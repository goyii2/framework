package web_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/goyii2/framework/base"
	"github.com/goyii2/framework/url"
	"github.com/goyii2/framework/web"
)

// siteController is registered in the application root scope and reached
// through the /{controller}/{action} convention fallback.
type siteController struct {
	web.Controller
}

func (siteController) ActionHello(c *web.Context) error {
	return c.JSON(map[string]string{"from": "convention"})
}

func (siteController) ActionBye(c *web.Context) error {
	return c.JSON(map[string]string{"from": "bye-convention"})
}

// userController is registered only inside a module and must not resolve
// outside its prefix.
type userController struct {
	web.Controller
}

func (userController) ActionList(c *web.Context) error {
	return c.JSON(map[string]string{"from": "module-convention"})
}

// reportController has an action method with the right name but the wrong
// signature; hitting it must surface a server error, not a silent miss.
type reportController struct {
	web.Controller
}

func (reportController) ActionBroken(c *web.Context, extra string) error { return nil }

// dashController lives in the /admin module scope of the nesting tests.
type dashController struct {
	web.Controller
}

func (dashController) ActionIndex(c *web.Context) error {
	return c.JSON(map[string]string{"from": "admin-convention"})
}

// newConventionApp assembles an application with one root controller and
// one /admin module controller for convention-fallback tests. CSRF is off:
// these tests exercise routing, not form-token mechanics (covered by the
// filter tests).
func newConventionApp(t *testing.T) (*web.Handler, *base.Application) {
	t.Helper()
	app := base.New()
	if err := app.RegisterController("site", siteController{}); err != nil {
		t.Fatalf("registering root controller: %v", err)
	}
	admin, err := base.NewModule(app, "/admin")
	if err != nil {
		t.Fatalf("creating module: %v", err)
	}
	if err := app.RegisterModule(admin); err != nil {
		t.Fatalf("registering module: %v", err)
	}
	if err := admin.RegisterController("user", userController{}); err != nil {
		t.Fatalf("registering module controller: %v", err)
	}
	return web.NewHandler(app, web.WithoutCsrf()), app
}

// req performs a single request against h and returns the raw response.
func req(t *testing.T, h http.Handler, method, target string) *http.Response {
	t.Helper()
	r := httptest.NewRequest(method, target, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec.Result()
}

// method asserts the request succeeds and returns the body's "m" value.
func method(t *testing.T, h http.Handler, m, target string) string {
	t.Helper()
	res := req(t, h, m, target)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d requesting %s %s", res.StatusCode, m, target)
	}
	var body map[string]string
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("decoding body of %s %s: %v", m, target, err)
	}
	return body["m"]
}

// source asserts the request succeeds and returns the body's "from" value,
// identifying which handler answered.
func source(t *testing.T, h http.Handler, target string) string {
	t.Helper()
	res := do(t, h, target, "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d requesting %s", res.StatusCode, target)
	}
	var body map[string]string
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("decoding body of %s: %v", target, err)
	}
	return body["from"]
}

func TestExplicitRouteParamsTypedValues(t *testing.T) {
	h := web.NewHandler(base.New())
	h.Get("/post/{id:[0-9]+}/{slug}/{flag}", func(c *web.Context) error {
		id, err := c.ParamInt("id")
		if err != nil {
			return web.BadRequest(err.Error())
		}
		flag, err := c.ParamBool("flag")
		if err != nil {
			return web.BadRequest(err.Error())
		}
		return c.JSON(map[string]any{"id": id, "slug": c.Param("slug"), "flag": flag})
	})
	h.Get("/n/{id:[0-9]+}", func(c *web.Context) error {
		id, err := c.ParamInt64("id")
		if err != nil {
			return web.BadRequest(err.Error())
		}
		return c.JSON(map[string]any{"id64": id})
	})

	res := do(t, h, "/post/42/hello-world/1", "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	var body map[string]any
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("decoding body: %v", err)
	}
	if body["id"] != float64(42) || body["slug"] != "hello-world" || body["flag"] != true {
		t.Errorf("body = %v, want typed path parameters echoed", body)
	}

	res = do(t, h, "/n/5000000000", "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	body = nil
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("decoding body: %v", err)
	}
	if body["id64"] != float64(5000000000) {
		t.Errorf("body = %v, want id64 5000000000", body)
	}
}

func TestTypedParamParseFailureBecomesBadRequest(t *testing.T) {
	h := web.NewHandler(base.New())
	h.Get("/big/{id:[0-9]+}", func(c *web.Context) error {
		if _, err := c.ParamInt("id"); err != nil {
			return web.BadRequest(err.Error())
		}
		return c.JSON(map[string]string{"ok": "yes"})
	})

	// The wildcard regex accepts the digits but the int does not fit:
	// the typed getter must report the parse failure to the action.
	res := do(t, h, "/big/99999999999999999999", "")
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for a non-int parameter", res.StatusCode)
	}
}

func TestMethodHelpersAndMethodMismatch(t *testing.T) {
	h := web.NewHandler(base.New(), web.WithoutCsrf())
	h.Get("/thing", func(c *web.Context) error { return c.JSON(map[string]string{"m": "get"}) })
	h.Post("/thing", func(c *web.Context) error { return c.JSON(map[string]string{"m": "post"}) })

	if got := method(t, h, http.MethodGet, "/thing"); got != "get" {
		t.Errorf("GET /thing = %q, want get", got)
	}
	if got := method(t, h, http.MethodPost, "/thing"); got != "post" {
		t.Errorf("POST /thing = %q, want post", got)
	}

	// A verb no explicit binding accepts is a verb mismatch, not a routing
	// miss: the handler answers 405 with the bound verbs in Allow instead
	// of silently falling through to the convention fallback.
	res := req(t, h, http.MethodPut, "/thing")
	if res.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", res.StatusCode)
	}
	if allow := res.Header.Get("Allow"); allow != "GET, POST" {
		t.Errorf("Allow = %q, want \"GET, POST\"", allow)
	}
}

func TestExplicitRouteVerbMismatch405(t *testing.T) {
	h := web.NewHandler(base.New(), web.WithoutCsrf())
	h.Get("/post/{id:[0-9]+}", func(c *web.Context) error {
		return c.JSON(map[string]string{"m": "get"})
	})
	h.Handle("/any", func(c *web.Context) error { // any-method binding
		return c.JSON(map[string]string{"m": "any"})
	})

	// Wrong verb on a bound wildcard path: 405 naming the bound verb.
	res := req(t, h, http.MethodPut, "/post/42")
	if res.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("PUT /post/42 status = %d, want 405", res.StatusCode)
	}
	if allow := res.Header.Get("Allow"); allow != "GET" {
		t.Errorf("Allow = %q, want GET", allow)
	}

	// A value the wildcard constraint rejects keeps the convention-fallback
	// semantics: the explicit route never applied, so no 405.
	if res := req(t, h, http.MethodPut, "/post/abc"); res.StatusCode != http.StatusNotFound {
		t.Fatalf("PUT /post/abc status = %d, want 404 (constraint miss is not a verb mismatch)", res.StatusCode)
	}

	// Method-less bindings accept every verb.
	if res := req(t, h, http.MethodPut, "/any"); res.StatusCode != http.StatusOK {
		t.Fatalf("PUT /any status = %d, want 200 (method-less binding)", res.StatusCode)
	}
}

func TestNamedRoutesPowerReverseGeneration(t *testing.T) {
	url.SetDefault(nil)
	defer url.SetDefault(nil)

	h := web.NewHandler(base.New())
	h.Get("/post/{id:[0-9]+}", func(c *web.Context) error { return nil }).Name("post/view")

	got, err := h.URLs().URL("post/view", url.P{"id": "9"})
	if err != nil {
		t.Fatalf("URL(post/view) error: %v", err)
	}
	if got != "/post/9" {
		t.Errorf("URL = %q, want /post/9", got)
	}

	// Names absent from the table assemble by convention, like Yii2 Url::to.
	got, err = h.URLs().URL("site/hello", url.P{"id": "1"})
	if err != nil {
		t.Fatalf("URL(site/hello) error: %v", err)
	}
	if got != "/site/hello?id=1" {
		t.Errorf("URL = %q, want /site/hello?id=1 by convention", got)
	}

	// Wiring the table into the package default makes url.To resolve the
	// same routes without a handle.
	url.SetDefault(h.URLs())
	if got := url.To("post/view", url.P{"id": "5"}); got != "/post/5" {
		t.Errorf("url.To = %q, want /post/5", got)
	}
}

func TestExplicitRouteBeatsConventionFallback(t *testing.T) {
	app := base.New()
	if err := app.RegisterController("site", siteController{}); err != nil {
		t.Fatalf("registering controller: %v", err)
	}
	h := web.NewHandler(app)
	h.Get("/site/hello", func(c *web.Context) error {
		return c.JSON(map[string]string{"from": "explicit"})
	})

	// The explicit binding wins over the identical convention route.
	if got := source(t, h, "/site/hello"); got != "explicit" {
		t.Errorf("explicit route lost to convention: from = %q", got)
	}
	// Actions without an explicit binding still resolve by convention.
	if got := source(t, h, "/site/bye"); got != "bye-convention" {
		t.Errorf("convention route missing: from = %q, want bye-convention", got)
	}
}

// TestRootRouteBindsOnlyTheRoot pins the root-binding semantics: a path of
// exactly "/" answers the root request alone. A plain "/" pattern would be a
// subtree catch-all in ServeMux and swallow every unmatched GET — including
// paths the convention fallback (and the module middleware it applies)
// should own.
func TestRootRouteBindsOnlyTheRoot(t *testing.T) {
	h, _ := newConventionApp(t)
	h.Get("/", func(c *web.Context) error {
		return c.JSON(map[string]string{"from": "root"})
	})

	// The root itself serves the binding.
	if got := source(t, h, "/"); got != "root" {
		t.Errorf("root route missing: from = %q, want root", got)
	}
	// Other paths still resolve by convention instead of landing on the
	// root action.
	if got := source(t, h, "/site/hello"); got != "convention" {
		t.Errorf("convention route starved by root binding: from = %q", got)
	}
	if got := source(t, h, "/admin/user/list"); got != "module-convention" {
		t.Errorf("module convention route starved by root binding: from = %q", got)
	}
	// A path nothing resolves is a 404, not the root page.
	res := do(t, h, "/no/such", "")
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d requesting /no/such, want 404", res.StatusCode)
	}
	// The root keeps its verb binding: POST / answers 405 with Allow.
	res = req(t, h, http.MethodPost, "/")
	if res.StatusCode != http.StatusMethodNotAllowed || res.Header.Get("Allow") != http.MethodGet {
		t.Fatalf("POST / = %d Allow %q, want 405 with Allow: GET", res.StatusCode, res.Header.Get("Allow"))
	}
}

func TestConventionFallbackDisabled(t *testing.T) {
	app := base.New()
	if err := app.RegisterController("site", siteController{}); err != nil {
		t.Fatalf("registering controller: %v", err)
	}
	h := web.NewHandler(app, web.WithoutConventionFallback())

	res := do(t, h, "/site/hello", "")
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 with the fallback disabled", res.StatusCode)
	}

	// Toggling at runtime brings the convention route back.
	h.SetConventionFallback(true)
	res = do(t, h, "/site/hello", "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d after re-enabling, want 200", res.StatusCode)
	}
}

func TestConventionFallbackModuleScopeAndIsolation(t *testing.T) {
	h, _ := newConventionApp(t)

	// Module controllers resolve under their prefix...
	if got := source(t, h, "/admin/user/list"); got != "module-convention" {
		t.Errorf("/admin/user/list from = %q, want module-convention", got)
	}
	// ...and root controllers at the root.
	if got := source(t, h, "/site/hello"); got != "convention" {
		t.Errorf("/site/hello from = %q, want convention", got)
	}

	// Isolation: neither registry leaks into the other scope.
	for _, target := range []string{"/user/list", "/admin/site/hello"} {
		if res := do(t, h, target, ""); res.StatusCode != http.StatusNotFound {
			t.Errorf("%s status = %d, want 404 (scope isolation)", target, res.StatusCode)
		}
	}

	// Unknown controller, unknown action and wrong segment counts all 404.
	for _, target := range []string{"/admin/nobody/list", "/admin/user/missing", "/admin", "/admin/user", "/site/hello/extra"} {
		if res := do(t, h, target, ""); res.StatusCode != http.StatusNotFound {
			t.Errorf("%s status = %d, want 404", target, res.StatusCode)
		}
	}
}

func TestConventionRoutesServeAnyMethod(t *testing.T) {
	h, _ := newConventionApp(t)

	res := req(t, h, http.MethodPost, "/site/hello")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 (convention routes are not method-bound)", res.StatusCode)
	}
}

func TestConventionActionSignatureMismatchIsServerError(t *testing.T) {
	app := base.New()
	if err := app.RegisterController("report", reportController{}); err != nil {
		t.Fatalf("registering controller: %v", err)
	}
	h := web.NewHandler(app)

	res := do(t, h, "/report/broken", "")
	if res.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 for a wrong-signature action", res.StatusCode)
	}
}

func TestMultiSegmentModulePrefixResolves(t *testing.T) {
	app := base.New()
	api, err := base.NewModule(app, "/api")
	if err != nil {
		t.Fatalf("creating /api module: %v", err)
	}
	v1, err := base.NewModule(app, "/api/v1")
	if err != nil {
		t.Fatalf("creating /api/v1 module: %v", err)
	}
	if err := app.RegisterModule(api); err != nil {
		t.Fatalf("registering /api module: %v", err)
	}
	if err := app.RegisterModule(v1); err != nil {
		t.Fatalf("registering /api/v1 module: %v", err)
	}
	if err := api.RegisterController("user", userController{}); err != nil {
		t.Fatalf("registering /api controller: %v", err)
	}
	if err := v1.RegisterController("site", siteController{}); err != nil {
		t.Fatalf("registering /api/v1 controller: %v", err)
	}
	h := web.NewHandler(app)

	// The longest matching prefix owns the path.
	if got := source(t, h, "/api/v1/site/hello"); got != "convention" {
		t.Errorf("/api/v1/site/hello from = %q, want the /api/v1 module to answer", got)
	}
	if got := source(t, h, "/api/user/list"); got != "module-convention" {
		t.Errorf("/api/user/list from = %q, want the /api module to answer", got)
	}
	// The v1 registry is invisible under the shorter prefix.
	if res := do(t, h, "/api/site/hello", ""); res.StatusCode != http.StatusNotFound {
		t.Errorf("/api/site/hello status = %d, want 404", res.StatusCode)
	}
}

func TestModuleScopedExplicitRoutes(t *testing.T) {
	h, _ := newConventionApp(t)

	admin, err := h.Module("/admin")
	if err != nil {
		t.Fatalf("Module(/admin) error: %v", err)
	}
	if _, err := h.Module("/ghost"); err == nil {
		t.Error("Module under an unregistered prefix must error")
	}

	// Names are module-relative: the module path is prepended
	// automatically, so "report/run" registers "admin/report/run".
	admin.Get("report/run", func(c *web.Context) error {
		return c.JSON(map[string]string{"from": "module-explicit"})
	}).Name("report/run")
	admin.Get("user/{id:[0-9]+}", func(c *web.Context) error {
		id, err := c.ParamInt("id")
		if err != nil {
			return web.BadRequest(err.Error())
		}
		return c.JSON(map[string]any{"id": id})
	}).Name("user/view")

	// Explicit mounts below the prefix answer and reverse by name.
	if got := source(t, h, "/admin/report/run"); got != "module-explicit" {
		t.Errorf("/admin/report/run from = %q, want module-explicit", got)
	}
	got, err := h.URLs().URL("admin/report/run", nil)
	if err != nil || got != "/admin/report/run" {
		t.Errorf("URL(admin/report/run) = %q, %v; want /admin/report/run", got, err)
	}
	got, err = h.URLs().URL("admin/user/view", url.P{"id": "9"})
	if err != nil || got != "/admin/user/9" {
		t.Errorf("URL(admin/user/view) = %q, %v; want /admin/user/9", got, err)
	}

	// Explicit patterns also win inside the module scope (without one,
	// /admin/user/7 would resolve by convention into a missing action).
	res := do(t, h, "/admin/user/7", "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	var body map[string]any
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("decoding body: %v", err)
	}
	if body["id"] != float64(7) {
		t.Errorf("body = %v, want id 7", body)
	}

	// The same controller's convention route stays reachable.
	if got := source(t, h, "/admin/user/list"); got != "module-convention" {
		t.Errorf("/admin/user/list from = %q, want module-convention", got)
	}
}

// newNestedApp assembles the module tree of the nesting tests: /admin with
// a dash controller and a nested /admin/report submodule with a user
// controller, both reachable through the convention fallback.
func newNestedApp(t *testing.T) (*web.Handler, *base.Application) {
	t.Helper()
	app := base.New()
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
	if err := admin.RegisterController("dash", dashController{}); err != nil {
		t.Fatalf("registering /admin controller: %v", err)
	}
	if err := report.RegisterController("user", userController{}); err != nil {
		t.Fatalf("registering /admin/report controller: %v", err)
	}
	return web.NewHandler(app, web.WithoutCsrf()), app
}

// TestNestedModuleConventionResolution pins layered controller lookup along
// the module tree: each module scope resolves its own controllers under its
// full prefix and no registry leaks into another level.
func TestNestedModuleConventionResolution(t *testing.T) {
	h, _ := newNestedApp(t)

	// The submodule answers under its full prefix...
	if got := source(t, h, "/admin/report/user/list"); got != "module-convention" {
		t.Errorf("/admin/report/user/list from = %q, want module-convention", got)
	}
	// ...the parent module under its own, undisturbed by the child.
	if got := source(t, h, "/admin/dash/index"); got != "admin-convention" {
		t.Errorf("/admin/dash/index from = %q, want admin-convention", got)
	}

	// Scope isolation across the tree levels.
	for _, target := range []string{
		"/admin/user/list",         // submodule controller is invisible to the parent scope
		"/admin/report/dash/index", // parent controller is invisible to the submodule scope
		"/user/list",               // submodule prefix does not exist at the root
		"/report/user/list",        // ditto without the /admin stem
		"/admin/report",            // prefix alone is no convention route
	} {
		if res := do(t, h, target, ""); res.StatusCode != http.StatusNotFound {
			t.Errorf("%s status = %d, want 404 (scope isolation)", target, res.StatusCode)
		}
	}
}

// TestNestedModuleExplicitRoutesAndNames mounts explicit routes through the
// nested registrar: names stay module-relative and automatically carry the
// module path, so reverse generation hits via the manager and url.To.
func TestNestedModuleExplicitRoutesAndNames(t *testing.T) {
	url.SetDefault(nil)
	defer url.SetDefault(nil)

	h, _ := newNestedApp(t)

	adminRoutes, err := h.Module("/admin")
	if err != nil {
		t.Fatalf("Module(/admin) error: %v", err)
	}
	adminRoutes.Get("panel", func(c *web.Context) error {
		return c.JSON(map[string]string{"from": "admin-explicit"})
	}).Name("panel")

	reportRoutes, err := h.Module("/admin/report")
	if err != nil {
		t.Fatalf("Module(/admin/report) error: %v", err)
	}
	reportRoutes.Get("dashboard", func(c *web.Context) error {
		return c.JSON(map[string]string{"from": "nested-explicit"})
	}).Name("dashboard")
	reportRoutes.Get("user/{id:[0-9]+}", func(c *web.Context) error {
		id, err := c.ParamInt("id")
		if err != nil {
			return web.BadRequest(err.Error())
		}
		return c.JSON(map[string]any{"id": id})
	}).Name("user/view")

	// The mounts answer under the concatenated prefixes.
	if got := source(t, h, "/admin/report/dashboard"); got != "nested-explicit" {
		t.Errorf("/admin/report/dashboard from = %q, want nested-explicit", got)
	}
	if got := source(t, h, "/admin/panel"); got != "admin-explicit" {
		t.Errorf("/admin/panel from = %q, want admin-explicit", got)
	}

	// Names carry the module path automatically: "dashboard" below
	// /admin/report registered as "admin/report/dashboard".
	cases := []struct {
		name   string
		params url.P
		want   string
	}{
		{"admin/panel", nil, "/admin/panel"},
		{"admin/report/dashboard", nil, "/admin/report/dashboard"},
		{"admin/report/user/view", url.P{"id": "9"}, "/admin/report/user/9"},
	}
	for _, tc := range cases {
		got, err := h.URLs().URL(tc.name, tc.params)
		if err != nil || got != tc.want {
			t.Errorf("URL(%s) = %q, %v; want %q", tc.name, got, err, tc.want)
		}
	}

	// Through the package default, url.To resolves the nested names too.
	url.SetDefault(h.URLs())
	if got := url.To("admin/report/dashboard", nil); got != "/admin/report/dashboard" {
		t.Errorf("url.To(admin/report/dashboard) = %q, want /admin/report/dashboard", got)
	}
	if got := url.To("admin/report/user/view", url.P{"id": "4"}); got != "/admin/report/user/4" {
		t.Errorf("url.To(admin/report/user/view) = %q, want /admin/report/user/4", got)
	}

	// The module's convention route keeps working beside the explicit one.
	if got := source(t, h, "/admin/report/user/list"); got != "module-convention" {
		t.Errorf("/admin/report/user/list from = %q, want module-convention", got)
	}
}

func TestUnmatchedPathsRenderNegotiatedNotFound(t *testing.T) {
	h := web.NewHandler(base.New())

	res := do(t, h, "/nope", "")
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q, want the negotiated JSON 404", ct)
	}

	res = do(t, h, "/a/b/c", "text/html")
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Errorf("Content-Type = %q, want the negotiated HTML 404", ct)
	}
}

func TestRoutingOverRealServer(t *testing.T) {
	h, _ := newConventionApp(t)
	h.Get("/post/{id:[0-9]+}", func(c *web.Context) error {
		id, err := c.ParamInt("id")
		if err != nil {
			return web.BadRequest(err.Error())
		}
		return c.JSON(map[string]any{"id": id})
	}).Name("post/view")

	srv := httptest.NewServer(h)
	defer srv.Close()

	// Explicit route with a typed parameter over real TCP.
	res, err := http.Get(srv.URL + "/post/42")
	if err != nil {
		t.Fatalf("requesting /post/42: %v", err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d on /post/42, want 200", res.StatusCode)
	}

	// Convention fallback on the same server.
	res, err = http.Get(srv.URL + "/site/hello")
	if err != nil {
		t.Fatalf("requesting /site/hello: %v", err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d on /site/hello, want 200", res.StatusCode)
	}

	// A regex miss falls through to the convention handler and 404s.
	res, err = http.Get(srv.URL + "/post/abc")
	if err != nil {
		t.Fatalf("requesting /post/abc: %v", err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d on /post/abc, want 404", res.StatusCode)
	}
}
