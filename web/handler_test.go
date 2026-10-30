package web_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/goyii2/framework/base"
	"github.com/goyii2/framework/web"
)

// postController is the example controller: it embeds web.Controller and
// exposes actions in the ActionXxx(ctx *web.Context) error shape.
type postController struct {
	web.Controller
}

func (postController) ActionView(c *web.Context) error {
	return c.JSON(map[string]any{"id": 7, "title": "hello goyii"})
}

func (postController) ActionArchive(c *web.Context) error {
	return c.Redirect(http.StatusFound, "/post/archive")
}

func (postController) ActionMissing(c *web.Context) error { return web.NotFound("post not found") }

func (postController) ActionSecret(c *web.Context) error { return web.Forbidden("members only") }

func (postController) ActionCrash(c *web.Context) error { return errors.New("database exploded") }

func (postController) ActionPanic(c *web.Context) error { panic("kaboom") }

func (postController) ActionHalfWritten(c *web.Context) error {
	if err := c.JSON(map[string]string{"partial": "yes"}); err != nil {
		return err
	}
	return web.NotFound("too late")
}

func (postController) ActionProbe(c *web.Context) error {
	seenApp["app"] = base.FromContext(c.Context())
	seenApp["ctx"] = web.FromContext(c.Context())
	return nil
}

// seenApp records what ActionProbe observed; requests run on the test
// goroutine via ServeHTTP, so no locking is needed.
var seenApp = map[string]any{}

// newTestApp assembles the handler with all example actions mounted.
func newTestApp(opts ...base.Option) (*web.Handler, *base.Application) {
	app := base.New(opts...)
	h := web.NewHandler(app)
	ctrl := postController{}
	h.Handle("GET /post/view", ctrl.ActionView)
	h.Handle("GET /post/archive", ctrl.ActionArchive)
	h.Handle("GET /post/missing", ctrl.ActionMissing)
	h.Handle("GET /post/secret", ctrl.ActionSecret)
	h.Handle("GET /post/crash", ctrl.ActionCrash)
	h.Handle("GET /post/panic", ctrl.ActionPanic)
	h.Handle("GET /post/half", ctrl.ActionHalfWritten)
	h.Handle("GET /probe", ctrl.ActionProbe)
	return h, app
}

func do(t *testing.T, h http.Handler, target, accept string) *http.Response {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Result()
}

func TestJSONActionReturnsJSON(t *testing.T) {
	h, _ := newTestApp()

	res := do(t, h, "/post/view", "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q, want application/json; charset=utf-8", ct)
	}
	var body map[string]any
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("decoding body: %v", err)
	}
	if body["title"] != "hello goyii" || body["id"] != float64(7) {
		t.Errorf("body = %v, want post {id:7, title:hello goyii}", body)
	}
}

func TestRedirectAction(t *testing.T) {
	h, _ := newTestApp()

	res := do(t, h, "/post/archive", "")
	if res.StatusCode != http.StatusFound {
		t.Fatalf("status = %d, want 302", res.StatusCode)
	}
	if loc := res.Header.Get("Location"); loc != "/post/archive" {
		t.Errorf("Location = %q, want /post/archive", loc)
	}
}

func TestActionSeesApplicationAndWebContext(t *testing.T) {
	h, app := newTestApp()
	clear(seenApp)

	res := do(t, h, "/probe", "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	if seenApp["app"] != app {
		t.Error("base.FromContext inside the action did not return the owning application")
	}
	if wc, ok := seenApp["ctx"].(*web.Context); !ok || wc == nil {
		t.Error("web.FromContext inside the action did not return the request Context")
	}
}

func TestNotFoundRendersJSONByDefault(t *testing.T) {
	h, _ := newTestApp()

	res := do(t, h, "/post/missing", "")
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q, want JSON for default Accept", ct)
	}
	var body map[string]any
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("decoding body: %v", err)
	}
	if body["name"] != "Not Found" || body["message"] != "post not found" || body["status"] != float64(404) {
		t.Errorf("error body = %v, want {name:Not Found, message:post not found, status:404}", body)
	}
}

func TestForbiddenRendersHTMLWhenPreferred(t *testing.T) {
	h, _ := newTestApp()

	res := do(t, h, "/post/secret", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Errorf("Content-Type = %q, want HTML for browser Accept", ct)
	}
	raw, _ := io.ReadAll(res.Body)
	page := string(raw)
	if !strings.Contains(page, "<h1>403 Forbidden</h1>") || !strings.Contains(page, "members only") {
		t.Errorf("HTML error page = %q, want 403 heading with message", page)
	}
}

func TestExplicitJSONAcceptRendersJSONError(t *testing.T) {
	h, _ := newTestApp()

	res := do(t, h, "/post/missing", "application/json")
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q, want JSON for explicit Accept", ct)
	}
}

func TestPlainErrorIs500WithoutInternals(t *testing.T) {
	h, _ := newTestApp()

	res := do(t, h, "/post/crash", "application/json")
	if res.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", res.StatusCode)
	}
	var body map[string]any
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("decoding body: %v", err)
	}
	if body["message"] != "Internal Server Error" {
		t.Errorf("message = %v, want generic text outside debug mode", body["message"])
	}
}

func TestPlainErrorRevealsTextInDebugMode(t *testing.T) {
	h, _ := newTestApp(base.WithApp("test", "dev", true))

	res := do(t, h, "/post/crash", "application/json")
	if res.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", res.StatusCode)
	}
	var body map[string]any
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("decoding body: %v", err)
	}
	if body["message"] != "database exploded" {
		t.Errorf("message = %v, want the real error text in debug mode", body["message"])
	}
}

func TestPanicRecoveredAs500AndEventsStillFire(t *testing.T) {
	h, app := newTestApp()

	var mu sync.Mutex
	var order []string
	app.On(base.EventBeforeRequest, func(context.Context, base.Event) {})
	app.On(base.EventAfterRequest, func(ctx context.Context, _ base.Event) {
		mu.Lock()
		order = append(order, statusLabel(ctx))
		mu.Unlock()
	})

	res := do(t, h, "/post/panic", "application/json")
	if res.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 after recovered panic", res.StatusCode)
	}
	var body map[string]any
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("decoding body: %v", err)
	}
	if body["status"] != float64(500) {
		t.Errorf("error body = %v, want status 500", body)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(order) != 1 || order[0] != "500" {
		t.Errorf("afterRequest statuses = %v, want [500]", order)
	}
}

func TestErrorAfterPartialWriteKeepsFirstResponse(t *testing.T) {
	h, _ := newTestApp()

	res := do(t, h, "/post/half", "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 from the already-written response", res.StatusCode)
	}
	var body map[string]any
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("decoding body: %v", err)
	}
	if body["partial"] != "yes" {
		t.Errorf("body = %v, want the partial JSON to stand", body)
	}
}

func TestRequestEventsFireInOrder(t *testing.T) {
	h, app := newTestApp()

	var order []string
	app.On(base.EventBeforeRequest, func(ctx context.Context, e base.Event) {
		order = append(order, "before")
		if base.FromContext(ctx) != app {
			t.Error("event context did not carry the application")
		}
		if web.FromContext(ctx) == nil {
			t.Error("event context did not carry the web Context")
		}
	})
	app.On(base.EventAfterRequest, func(ctx context.Context, e base.Event) {
		order = append(order, statusLabel(ctx))
	})

	do(t, h, "/post/view", "")
	do(t, h, "/post/missing", "application/json")

	want := []string{"before", "200", "before", "404"}
	if len(order) != len(want) {
		t.Fatalf("event order = %v, want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("event order = %v, want %v", order, want)
		}
	}
}

func TestRealServerServesJSONOverTCP(t *testing.T) {
	h, app := newTestApp()

	var mu sync.Mutex
	var fired []string
	app.On(base.EventBeforeRequest, func(context.Context, base.Event) {
		mu.Lock()
		fired = append(fired, "before")
		mu.Unlock()
	})
	app.On(base.EventAfterRequest, func(context.Context, base.Event) {
		mu.Lock()
		fired = append(fired, "after")
		mu.Unlock()
	})

	srv := httptest.NewServer(h)
	defer srv.Close()

	res, err := http.Get(srv.URL + "/post/view")
	if err != nil {
		t.Fatalf("requesting server: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	var body map[string]any
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("decoding body: %v", err)
	}
	if body["title"] != "hello goyii" {
		t.Errorf("body = %v, want the action's JSON", body)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(fired) != 2 || fired[0] != "before" || fired[1] != "after" {
		t.Errorf("lifecycle events on a real request = %v, want [before after]", fired)
	}
}

// statusLabel renders the response status carried by the web Context of an
// event context.
func statusLabel(ctx context.Context) string {
	c := web.FromContext(ctx)
	if c == nil {
		return "no-web-context"
	}
	return strconv.Itoa(c.StatusCode())
}
