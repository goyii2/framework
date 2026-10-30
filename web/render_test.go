package web_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/a-h/templ"
	"github.com/goyii2/framework/base"
	"github.com/goyii2/framework/view"
	"github.com/goyii2/framework/web"
)

// renderViewData is the typed payload the test views render.
type renderViewData struct {
	Title string
}

// markup builds a component writing fixed markup.
func markup(s string) templ.Component {
	return templ.ComponentFunc(func(_ context.Context, w io.Writer) error {
		_, err := io.WriteString(w, s)
		return err
	})
}

func body(t *testing.T, res *http.Response) string {
	t.Helper()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("reading body: %v", err)
	}
	return string(b)
}

// newRenderApp assembles a handler with the view layer wired: a named
// "page/hello" view, a direct-component action, a module under /admin with
// its own layout, and error-producing actions.
func newRenderApp(t *testing.T) *web.Handler {
	t.Helper()
	app := base.New()
	h := web.NewHandler(app)

	h.Views().Register("page/hello", view.Typed(func(data renderViewData) templ.Component {
		return markup("<h1>" + data.Title + "</h1>")
	}))
	// A view routing model text through a template, for escaping coverage
	// on the HTTP path.
	h.Views().Register("page/raw", view.Typed(func(data renderViewData) templ.Component {
		return view.DetailView{Rows: []view.DetailRow{{Label: "Title", Value: data.Title}}}.Render()
	}))

	h.Get("/hello", func(c *web.Context) error {
		c.SetTitle("Hello")
		return c.Render("page/hello", renderViewData{Title: "world"})
	})
	h.Get("/raw", func(c *web.Context) error {
		return c.Render("page/raw", renderViewData{Title: "<script>alert(1)</script>"})
	})
	h.Get("/direct", func(c *web.Context) error {
		return c.RenderComponent(markup("<p>direct</p>"))
	})
	h.Get("/rejected", func(c *web.Context) error {
		return c.RenderStatus(http.StatusUnprocessableEntity, "page/hello", renderViewData{Title: "try again"})
	})
	h.Get("/partial", func(c *web.Context) error {
		c.ClearLayout()
		return c.RenderComponent(markup("<p>fragment</p>"))
	})
	h.Get("/custom-frame", func(c *web.Context) error {
		c.SetLayout(func(p view.Page) templ.Component {
			return markup(`<div id="page-frame">` + p.Title + `</div>`)
		})
		return c.Render("page/hello", renderViewData{Title: "framed"})
	})
	h.Get("/missing-view", func(c *web.Context) error {
		return c.Render("nope/missing", nil)
	})
	h.Get("/boom", func(c *web.Context) error {
		return web.ServerError("kaboom")
	})
	h.Get("/gone", func(c *web.Context) error {
		return web.NotFound("nothing here")
	})

	mod, err := base.NewModule(app, "/admin")
	if err != nil {
		t.Fatalf("creating module: %v", err)
	}
	if err := app.RegisterModule(mod); err != nil {
		t.Fatalf("registering module: %v", err)
	}
	admin, err := h.Module("/admin")
	if err != nil {
		t.Fatalf("module routes: %v", err)
	}
	admin.Layout(func(p view.Page) templ.Component {
		return markup(`<div id="admin-frame"></div>`)
	})
	admin.Get("/dash", func(c *web.Context) error {
		return c.Render("page/hello", renderViewData{Title: "dash"})
	})
	admin.Get("/own-frame", func(c *web.Context) error {
		c.SetLayout(func(p view.Page) templ.Component {
			return markup(`<div id="own-frame"></div>`)
		})
		return c.Render("page/hello", renderViewData{Title: "own"})
	})

	// A nested submodule with its own frame: its routes must carry the
	// child frame, not the parent module's.
	report, err := base.NewModule(app, "/admin/report")
	if err != nil {
		t.Fatalf("creating /admin/report module: %v", err)
	}
	if err := mod.RegisterModule(report); err != nil {
		t.Fatalf("nesting /admin/report module: %v", err)
	}
	reportRoutes, err := h.Module("/admin/report")
	if err != nil {
		t.Fatalf("module routes: %v", err)
	}
	reportRoutes.Layout(func(p view.Page) templ.Component {
		return markup(`<div id="report-frame">` + p.Title + `</div>`)
	})
	reportRoutes.Get("/panel", func(c *web.Context) error {
		c.SetTitle("Panel")
		return c.Render("page/hello", renderViewData{Title: "panel"})
	})
	reportRoutes.Get("/own-frame", func(c *web.Context) error {
		c.SetLayout(func(p view.Page) templ.Component {
			return markup(`<div id="own-frame"></div>`)
		})
		return c.Render("page/hello", renderViewData{Title: "own"})
	})
	return h
}

func get(t *testing.T, h http.Handler, target string) (*http.Response, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	req.Header.Set("Accept", "text/html")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	res := rec.Result()
	return res, body(t, res)
}

func TestRenderByNameReturnsFullHTML(t *testing.T) {
	h := newRenderApp(t)
	res, b := get(t, h, "/hello")

	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Errorf("Content-Type = %q, want text/html; charset=utf-8", ct)
	}
	for _, want := range []string{"<!doctype html>", "<title>Hello</title>", "<h1>world</h1>"} {
		if !strings.Contains(b, want) {
			t.Errorf("body missing %q:\n%s", want, b)
		}
	}
}

func TestRenderComponentReturnsFullHTML(t *testing.T) {
	h := newRenderApp(t)
	res, b := get(t, h, "/direct")

	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	if !strings.Contains(b, "<!doctype html>") || !strings.Contains(b, "<p>direct</p>") {
		t.Errorf("direct render is not a full document:\n%s", b)
	}
}

// TestRenderStatusCarriesStatusCode pins the form-re-render path: the reply
// keeps the explicit status (422 for validation failures), the HTML content
// type and the layout frame around the re-rendered content.
func TestRenderStatusCarriesStatusCode(t *testing.T) {
	h := newRenderApp(t)
	res, b := get(t, h, "/rejected")

	if res.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d", res.StatusCode, http.StatusUnprocessableEntity)
	}
	if ct := res.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("content type = %q, want text/html", ct)
	}
	if !strings.Contains(b, "<!doctype html>") || !strings.Contains(b, "<h1>try again</h1>") {
		t.Errorf("framed content missing:\n%s", b)
	}
}

func TestRenderEscapesModelText(t *testing.T) {
	h := newRenderApp(t)
	_, b := get(t, h, "/raw")

	if strings.Contains(b, "<script>alert(1)</script>") {
		t.Errorf("script payload passed through unescaped:\n%s", b)
	}
	if !strings.Contains(b, "&lt;script&gt;alert(1)&lt;/script&gt;") {
		t.Errorf("escaped payload missing:\n%s", b)
	}
}

func TestClearLayoutRendersBareFragment(t *testing.T) {
	h := newRenderApp(t)
	res, b := get(t, h, "/partial")

	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", res.StatusCode)
	}
	if b != "<p>fragment</p>" {
		t.Errorf("bare render = %q, want the unframed fragment", b)
	}
}

func TestPageLayoutOverride(t *testing.T) {
	h := newRenderApp(t)
	_, b := get(t, h, "/custom-frame")

	if !strings.Contains(b, `<div id="page-frame">`) {
		t.Errorf("page layout not applied:\n%s", b)
	}
	if strings.Contains(b, "<main>") {
		t.Errorf("default shell still rendered under page override:\n%s", b)
	}
}

func TestModuleLayoutOverride(t *testing.T) {
	h := newRenderApp(t)

	_, admin := get(t, h, "/admin/dash")
	if !strings.Contains(admin, `<div id="admin-frame">`) {
		t.Errorf("module layout not applied:\n%s", admin)
	}

	// Outside the module the default frame stays.
	_, outside := get(t, h, "/direct")
	if strings.Contains(outside, "admin-frame") {
		t.Errorf("module layout leaked outside its prefix:\n%s", outside)
	}

	// A page-level override wins over the module frame.
	_, own := get(t, h, "/admin/own-frame")
	if !strings.Contains(own, `<div id="own-frame">`) {
		t.Errorf("page override did not win over module layout:\n%s", own)
	}
	if strings.Contains(own, "admin-frame") {
		t.Errorf("module frame still rendered under page override:\n%s", own)
	}
}

// TestNestedModuleLayoutOverride pins layout isolation under nesting: the
// submodule's frame wins on its routes, the parent module's frame stays on
// its own, and a page-level override still wins over both.
func TestNestedModuleLayoutOverride(t *testing.T) {
	h := newRenderApp(t)

	// The submodule route carries the submodule's frame, not the parent's
	// and not the built-in shell.
	_, panel := get(t, h, "/admin/report/panel")
	if !strings.Contains(panel, `<div id="report-frame">Panel</div>`) {
		t.Errorf("submodule layout not applied:\n%s", panel)
	}
	if strings.Contains(panel, "admin-frame") || strings.Contains(panel, "<main>") {
		t.Errorf("parent or default frame leaked into the submodule route:\n%s", panel)
	}

	// The parent module's routes keep their frame.
	_, admin := get(t, h, "/admin/dash")
	if !strings.Contains(admin, `<div id="admin-frame">`) || strings.Contains(admin, "report-frame") {
		t.Errorf("parent module layout disturbed by the submodule:\n%s", admin)
	}

	// A page-level override wins over the submodule frame.
	_, own := get(t, h, "/admin/report/own-frame")
	if !strings.Contains(own, `<div id="own-frame">`) {
		t.Errorf("page override did not win over the submodule layout:\n%s", own)
	}
	if strings.Contains(own, "report-frame") {
		t.Errorf("submodule frame still rendered under page override:\n%s", own)
	}
}

func TestUnknownViewNameIsServerError(t *testing.T) {
	h := newRenderApp(t)
	res, b := get(t, h, "/missing-view")

	if res.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", res.StatusCode)
	}
	if strings.Contains(b, "nope/missing") {
		t.Errorf("internal view name leaked into the response:\n%s", b)
	}
}

func TestErrorViewOverrideByStatus(t *testing.T) {
	h := newRenderApp(t)
	h.Views().Register("error/404", view.Typed(func(data view.ErrorData) templ.Component {
		return markup("<h1 id=\"custom-404\">" + data.Name + "</h1><p>" + data.Message + "</p>")
	}))
	h.Views().Register("error", view.Typed(func(data view.ErrorData) templ.Component {
		return markup("<h1 id=\"generic-error\">" + data.Name + "</h1>")
	}))

	res, notFound := get(t, h, "/gone")
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", res.StatusCode)
	}
	for _, want := range []string{`<h1 id="custom-404">Not Found</h1>`, "<p>nothing here</p>"} {
		if !strings.Contains(notFound, want) {
			t.Errorf("404 override missing %q:\n%s", want, notFound)
		}
	}

	// 500 falls through to the generic "error" view.
	res, serverErr := get(t, h, "/boom")
	if res.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", res.StatusCode)
	}
	if !strings.Contains(serverErr, `<h1 id="generic-error">Internal Server Error</h1>`) {
		t.Errorf("generic error view not used for 500:\n%s", serverErr)
	}
	if strings.Contains(serverErr, "custom-404") {
		t.Errorf("404 view used for a 500:\n%s", serverErr)
	}
}

func TestErrorViewOverrideGoesThroughLayout(t *testing.T) {
	h := newRenderApp(t)
	h.Views().Register("error/404", view.Typed(func(data view.ErrorData) templ.Component {
		return markup("<p>gone fishing</p>")
	}))
	_, b := get(t, h, "/gone")
	if !strings.Contains(b, "<!doctype html>") || !strings.Contains(b, "<p>gone fishing</p>") {
		t.Errorf("error override not framed as a page:\n%s", b)
	}
}

func TestErrorPageFallsBackToBuiltinWithoutRegistration(t *testing.T) {
	h := newRenderApp(t) // no error views registered
	res, b := get(t, h, "/gone")

	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", res.StatusCode)
	}
	if !strings.Contains(b, "<!DOCTYPE html>") || !strings.Contains(b, "<h1>404 Not Found</h1>") {
		t.Errorf("builtin error page not rendered:\n%s", b)
	}
}

func TestJSONErrorsStayJSONWithViewOverride(t *testing.T) {
	h := newRenderApp(t)
	h.Views().Register("error/404", view.Typed(func(data view.ErrorData) templ.Component {
		return markup("<p>should not appear</p>")
	}))

	req := httptest.NewRequest(http.MethodGet, "/gone", nil)
	req.Header.Set("Accept", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	res := rec.Result()

	if ct := res.Header.Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q, want JSON", ct)
	}
	if b := body(t, res); !strings.Contains(b, `"message":"nothing here"`) {
		t.Errorf("JSON error body = %s", b)
	}
}
