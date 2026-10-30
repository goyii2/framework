package debug

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/goyii2/framework/base"
	"github.com/goyii2/framework/session"
	"github.com/goyii2/framework/web"
)

const toolbarMark = `id="goyii-debug-toolbar"`

// newPanelServer assembles a debug application: handler, mounted panel
// storing into a temp directory, isolated session path.
func newPanelServer(t *testing.T, hopts ...web.HandlerOption) (*web.Handler, *Panel, *base.Application) {
	t.Helper()
	app := base.New(base.WithApp("panel-test", "dev", true))
	sess, err := session.New(session.Config{Path: t.TempDir()})
	if err != nil {
		t.Fatalf("session.New: %v", err)
	}
	app.SetSession(sess)
	h := web.NewHandler(app, hopts...)
	p := New(app)
	p.Dir = t.TempDir()
	p.Mount(h)
	return h, p, app
}

// serve runs one request against h from a chosen client address.
func serve(h http.Handler, method, target, remote string) *httptest.ResponseRecorder {
	t := httptest.NewRecorder()
	req := httptest.NewRequest(method, target, nil)
	req.RemoteAddr = remote
	h.ServeHTTP(t, req)
	return t
}

// writeHTML writes a minimal HTML page from an action.
func writeHTML(c *web.Context, body string) error {
	c.Response().Header().Set("Content-Type", "text/html; charset=utf-8")
	_, err := c.Response().Write([]byte(body))
	return err
}

func singleEntry(t *testing.T, p *Panel) *Entry {
	t.Helper()
	entries, err := p.history(10)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("stored %d entries, want 1", len(entries))
	}
	return entries[0]
}

func TestToolbarInjectedIntoHtmlResponses(t *testing.T) {
	h, _, _ := newPanelServer(t, nil)
	h.Get("/page", func(c *web.Context) error {
		return writeHTML(c, "<html><body>hello</body></html>")
	})
	rec := serve(h, http.MethodGet, "/page", "127.0.0.1:1000")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, toolbarMark) {
		t.Errorf("toolbar missing from HTML response:\n%s", body)
	}
	if toolbarAt, bodyAt := strings.Index(body, toolbarMark), strings.LastIndex(body, "</body>"); toolbarAt < 0 || bodyAt < toolbarAt {
		t.Errorf("toolbar not injected before </body>:\n%s", body)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type lost: %q", ct)
	}
}

func TestToolbarNotInjectedIntoNonHtmlResponses(t *testing.T) {
	h, _, _ := newPanelServer(t, nil)
	h.Get("/api", func(c *web.Context) error {
		c.Response().Header().Set("Content-Type", "application/json")
		_, err := c.Response().Write([]byte(`{"ok":true}`))
		return err
	})
	rec := serve(h, http.MethodGet, "/api", "127.0.0.1:1000")
	if rec.Body.String() != `{"ok":true}` {
		t.Errorf("JSON response modified:\n%s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), toolbarMark) {
		t.Error("toolbar injected into a JSON response")
	}
}

func TestToolbarNotInjectedIntoErrorResponses(t *testing.T) {
	h, p, _ := newPanelServer(t, nil)
	h.Get("/boom", func(c *web.Context) error {
		return web.NotFound("no such thing")
	})
	rec := serve(h, http.MethodGet, "/boom", "127.0.0.1:1000")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	if strings.Contains(rec.Body.String(), toolbarMark) {
		t.Error("toolbar injected into an error response")
	}
	// The request is still recorded, with the error it ended in.
	e := singleEntry(t, p)
	if e.Request.Status != http.StatusNotFound {
		t.Errorf("recorded status = %d, want 404", e.Request.Status)
	}
	if e.Request.Error == "" {
		t.Error("recorded error empty")
	}
}

// TestErrorResponsesKeepHeadersSetBeforeTheFailure pins the error-path
// contract of the hold: an action (or a filter inside the capture) that set
// response headers before failing — a VerbFilter's Allow, a Location set
// ahead of an abort — must not lose them, because the unified renderer
// answers through the restored writer while the headers live on the real
// response's map.
func TestErrorResponsesKeepHeadersSetBeforeTheFailure(t *testing.T) {
	h, _, _ := newPanelServer(t, nil)
	h.Get("/denied", func(c *web.Context) error {
		c.Response().Header().Set("Allow", "GET, HEAD")
		return web.MethodNotAllowed("method not allowed")
	})
	rec := serve(h, http.MethodGet, "/denied", "127.0.0.1:1000")
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
	if allow := rec.Header().Get("Allow"); allow != "GET, HEAD" {
		t.Errorf("Allow header lost on the error path: %q", allow)
	}
	if strings.Contains(rec.Body.String(), toolbarMark) {
		t.Error("toolbar injected into an error response")
	}
}

func TestToolbarSkipsHeadRequests(t *testing.T) {
	h, _, _ := newPanelServer(t, nil)
	h.Get("/page", func(c *web.Context) error {
		return writeHTML(c, "<html><body>hello</body></html>")
	})
	rec := serve(h, http.MethodHead, "/page", "127.0.0.1:1000")
	if strings.Contains(rec.Body.String(), toolbarMark) {
		t.Errorf("toolbar injected into a HEAD response:\n%s", rec.Body.String())
	}
}

func TestStreamingResponsePassesThroughWithoutToolbar(t *testing.T) {
	h, _, _ := newPanelServer(t, nil)
	h.Get("/stream", func(c *web.Context) error {
		c.Response().Header().Set("Content-Type", "text/html; charset=utf-8")
		if _, err := c.Response().Write([]byte("<html><body>first ")); err != nil {
			return err
		}
		if err := http.NewResponseController(c.Response()).Flush(); err != nil {
			return err
		}
		_, err := c.Response().Write([]byte("second</body></html>"))
		return err
	})
	rec := serve(h, http.MethodGet, "/stream", "127.0.0.1:1000")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if body != "<html><body>first second</body></html>" {
		t.Errorf("streamed body = %q", body)
	}
	if strings.Contains(body, toolbarMark) {
		t.Error("toolbar injected into a streamed response")
	}
}

func TestEntryCapturesRequestAndAllPanels(t *testing.T) {
	h, p, _ := newPanelServer(t)
	h.Get("/post/{id:[0-9]+}", func(c *web.Context) error {
		c.Session().Set("cart", "books")
		c.App().Log().Info("handling it", "step", 2)
		dblog := c.App().Logger().Category("db")
		dblog.Log(slog.LevelDebug, "db", "query", "sql", "SELECT 1", "elapsed_ms", 0.4, "rows", int64(1))
		c.App().Logger().BeginProfile("segment").End()
		c.App().Logger().Category("mail").Info("From: noreply@example.com\r\nSubject: hi\r\n\r\nbody")
		return writeHTML(c, "<html><body>ok</body></html>")
	})
	rec := serve(h, http.MethodGet, "/post/7", "127.0.0.1:1000")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}

	e := singleEntry(t, p)
	// Request/routing panel.
	if e.Request.Method != http.MethodGet || e.Request.Path != "/post/7" {
		t.Errorf("request panel = %+v", e.Request)
	}
	if e.Request.Route != "GET /post/{id}" {
		t.Errorf("route = %q, want GET /post/{id}", e.Request.Route)
	}
	if e.Request.Params["id"] != "7" {
		t.Errorf("params = %v, want id=7", e.Request.Params)
	}
	if e.Request.Status != http.StatusOK || e.Request.DurationMs < 0 {
		t.Errorf("status/duration = %d/%v", e.Request.Status, e.Request.DurationMs)
	}
	// Log panel.
	found := false
	for _, l := range e.Logs {
		if l.Message == "handling it" {
			found = true
			// Values round-trip through the stored JSON, so numbers come
			// back as float64.
			if l.Attrs["step"] != float64(2) {
				t.Errorf("log attrs = %v", l.Attrs)
			}
		}
	}
	if !found {
		t.Errorf("action log record missing: %+v", e.Logs)
	}
	// SQL panel.
	if len(e.SQL) != 1 || e.SQL[0].SQL != "SELECT 1" || e.SQL[0].Rows != 1 {
		t.Errorf("sql panel = %+v", e.SQL)
	}
	// Timeline panel.
	if len(e.Timeline) != 1 || e.Timeline[0].Name != "segment" {
		t.Errorf("timeline panel = %+v", e.Timeline)
	}
	// Session/User panels.
	if e.Session == nil || e.Session.Values["cart"] != "books" || e.Session.ID == "" {
		t.Errorf("session panel = %+v", e.Session)
	}
	if e.User == nil || !e.User.Guest {
		t.Errorf("user panel = %+v, want guest", e.User)
	}
	// Mail panel.
	if len(e.Mail) != 1 || !strings.Contains(e.Mail[0].Message, "Subject: hi") {
		t.Errorf("mail panel = %+v", e.Mail)
	}
}

func TestLogCaptureAttributesRecordsToTheirRequest(t *testing.T) {
	h, p, _ := newPanelServer(t, nil)
	h.Get("/page", func(c *web.Context) error {
		c.App().Log().Info(c.Request().URL.Query().Get("msg"))
		return writeHTML(c, "<html><body>ok</body></html>")
	})
	serve(h, http.MethodGet, "/page?msg=first-only", "127.0.0.1:1000")
	serve(h, http.MethodGet, "/page?msg=second-only", "127.0.0.1:1000")

	entries, err := p.history(10)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("stored %d entries, want 2", len(entries))
	}
	// Newest first.
	second, first := entries[0], entries[1]
	for _, e := range []struct {
		name  string
		entry *Entry
	}{{"first", first}, {"second", second}} {
		msgs := make([]string, 0, len(e.entry.Logs))
		for _, l := range e.entry.Logs {
			msgs = append(msgs, l.Message)
		}
		joined := strings.Join(msgs, ",")
		if !strings.Contains(joined, e.name+"-only") {
			t.Errorf("%s entry misses its own record: %v", e.name, msgs)
		}
		other := "first-only"
		if e.name == "first" {
			other = "second-only"
		}
		if strings.Contains(joined, other) {
			t.Errorf("%s entry captured the other request's record: %v", e.name, msgs)
		}
	}
}

func TestPanelPagesServeWithoutCapture(t *testing.T) {
	h, p, _ := newPanelServer(t, nil)
	h.Get("/page", func(c *web.Context) error {
		return writeHTML(c, "<html><body>hello</body></html>")
	})
	serve(h, http.MethodGet, "/page", "127.0.0.1:1000")

	rec := serve(h, http.MethodGet, "/debug", "127.0.0.1:1000")
	if rec.Code != http.StatusOK {
		t.Fatalf("history status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Request history") {
		t.Errorf("history page body unexpected:\n%s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), toolbarMark) {
		t.Error("toolbar injected into the panel's own page")
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("history Content-Type = %q", ct)
	}

	entry := singleEntry(t, p)
	rec = serve(h, http.MethodGet, "/debug/view?id="+entry.ID, "127.0.0.1:1000")
	if rec.Code != http.StatusOK {
		t.Fatalf("detail status = %d", rec.Code)
	}
	detail := rec.Body.String()
	if !strings.Contains(detail, "Session / User") || !strings.Contains(detail, "Mail") {
		t.Errorf("detail page missing panels:\n%s", detail)
	}
	if strings.Contains(detail, toolbarMark) {
		t.Error("toolbar injected into the panel's detail page")
	}

	// Panel browsing wrote no additional records.
	if entries, _ := p.history(10); len(entries) != 1 {
		t.Errorf("panel browsing wrote records: %d entries", len(entries))
	}
	// Unknown ids answer 404.
	if rec := serve(h, http.MethodGet, "/debug/view?id=nope", "127.0.0.1:1000"); rec.Code != http.StatusNotFound {
		t.Errorf("unknown id status = %d, want 404", rec.Code)
	}
}

func TestPanelAccessControl(t *testing.T) {
	h, p, _ := newPanelServer(t)
	h.Get("/page", func(c *web.Context) error {
		return writeHTML(c, "<html><body>x</body></html>")
	})
	p.AllowCIDRs = []string{"203.0.113.0/24"}

	// Configured range opens the panel.
	if rec := serve(h, http.MethodGet, "/debug", "203.0.113.7:55"); rec.Code != http.StatusOK {
		t.Errorf("allowed client status = %d, want 200", rec.Code)
	}
	// Loopback is not included once a list is configured.
	if rec := serve(h, http.MethodGet, "/debug", "127.0.0.1:55"); rec.Code != http.StatusForbidden {
		t.Errorf("loopback status = %d, want 403", rec.Code)
	}
	// Everyone else is denied.
	if rec := serve(h, http.MethodGet, "/debug", "192.0.2.1:55"); rec.Code != http.StatusForbidden {
		t.Errorf("foreign client status = %d, want 403", rec.Code)
	}
	// The restriction covers panel pages only; application pages answer
	// regardless of the client address.
	if rec := serve(h, http.MethodGet, "/page", "192.0.2.1:55"); rec.Code != http.StatusOK {
		t.Errorf("application page status = %d, want 200", rec.Code)
	}
}

func TestPanelAccessControlLoopbackByDefault(t *testing.T) {
	h, _, _ := newPanelServer(t, nil)
	for _, addr := range []string{"127.0.0.1:55", "[::1]:55"} {
		if rec := serve(h, http.MethodGet, "/debug", addr); rec.Code != http.StatusOK {
			t.Errorf("loopback %s status = %d, want 200", addr, rec.Code)
		}
	}
	if rec := serve(h, http.MethodGet, "/debug", "10.0.0.1:55"); rec.Code != http.StatusForbidden {
		t.Errorf("non-loopback status = %d, want 403", rec.Code)
	}
}

func TestMountWithoutDebugDoesNothing(t *testing.T) {
	app := base.New()
	h := web.NewHandler(app)
	p := New(app)
	p.Dir = t.TempDir()
	p.Mount(h)
	h.Get("/page", func(c *web.Context) error {
		return writeHTML(c, "<html><body>plain</body></html>")
	})
	rec := serve(h, http.MethodGet, "/debug", "127.0.0.1:1000")
	if rec.Code != http.StatusNotFound {
		t.Errorf("/debug status = %d, want 404 (panel must not mount without debug)", rec.Code)
	}
	rec = serve(h, http.MethodGet, "/page", "127.0.0.1:1000")
	if strings.Contains(rec.Body.String(), toolbarMark) {
		t.Error("toolbar injected although the app is not in debug mode")
	}
	if _, err := os.Stat(filepath.Join(p.Dir, ".")); err == nil || !os.IsNotExist(err) {
		// Nothing should have written records; the directory itself may not
		// even exist.
		t.Logf("record dir state: %v", err)
	}
}

func TestPageCacheBelowCaptureStaysCleanAndReplayGetsToolbar(t *testing.T) {
	h, p, app := newPanelServer(t, nil)

	cached := &cachedPageController{}
	h.Ctrl(cached).Get("/cached", cached.ActionPage).Name("cached")

	first := serve(h, http.MethodGet, "/cached", "127.0.0.1:1000")
	if !strings.Contains(first.Body.String(), toolbarMark) {
		t.Fatalf("first render misses the toolbar:\n%s", first.Body.String())
	}
	second := serve(h, http.MethodGet, "/cached", "127.0.0.1:1000")
	if !strings.Contains(second.Body.String(), toolbarMark) {
		t.Fatalf("replayed page misses the toolbar:\n%s", second.Body.String())
	}
	// The replay carries its own fresh toolbar: the injected record ids
	// differ because they belong to different captured requests.
	firstID, secondID := entryIDIn(first.Body.String()), entryIDIn(second.Body.String())
	if firstID == "" || secondID == "" || firstID == secondID {
		t.Errorf("toolbar ids not per-request: %q vs %q", firstID, secondID)
	}
	// The page cache entry stays clean: no toolbar baked into the stored
	// body, or every later visitor would replay a stale toolbar.
	var stored struct {
		Status      int    `json:"status"`
		ContentType string `json:"contentType"`
		Body        []byte `json:"body"`
	}
	found, err := app.Cache().Get(context.Background(), "/cached", &stored)
	if err != nil || !found {
		t.Fatalf("cache entry: found=%v err=%v", found, err)
	}
	if strings.Contains(string(stored.Body), toolbarMark) {
		t.Errorf("cached page carries the toolbar:\n%s", stored.Body)
	}
	// Both requests were recorded.
	if entries, _ := p.history(10); len(entries) != 2 {
		t.Errorf("recorded %d entries, want 2", len(entries))
	}
}

// entryIDIn extracts the record id out of a toolbar's detail link.
func entryIDIn(body string) string {
	marker := `/debug/view?id=`
	i := strings.Index(body, marker)
	if i < 0 {
		return ""
	}
	rest := body[i+len(marker):]
	if end := strings.IndexAny(rest, `"'`); end >= 0 {
		return rest[:end]
	}
	return rest
}

// cachedPageController mounts a PageCache filter around its action, the
// way applications compose filters per controller.
type cachedPageController struct{}

func (cachedPageController) Middleware() []web.Middleware {
	return []web.Middleware{web.PageCache{Duration: time.Minute}.Middleware()}
}

func (cachedPageController) ActionPage(c *web.Context) error {
	return writeHTML(c, "<html><body>cached content</body></html>")
}

func TestToolbarEligibility(t *testing.T) {
	newCapture := func(status int, contentType string) *web.CaptureWriter {
		rec := httptest.NewRecorder()
		cw := web.NewCaptureWriter(rec)
		if contentType != "" {
			cw.Header().Set("Content-Type", contentType)
		}
		cw.WriteHeader(status)
		return cw
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	head := httptest.NewRequest(http.MethodHead, "/", nil)

	cases := []struct {
		name    string
		cw      *web.CaptureWriter
		request *http.Request
		want    bool
	}{
		{"html 200", newCapture(200, "text/html; charset=utf-8"), req, true},
		{"json 200", newCapture(200, "application/json"), req, false},
		{"html 404", newCapture(404, "text/html"), req, false},
		{"html 500", newCapture(500, "text/html"), req, false},
		{"no content type", newCapture(200, ""), req, false},
		{"head", newCapture(200, "text/html"), head, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := toolbarEligible(tc.cw, tc.request); got != tc.want {
				t.Errorf("toolbarEligible = %v, want %v", got, tc.want)
			}
		})
	}

	// A flushed response has left the holding regime.
	rec := httptest.NewRecorder()
	cw := web.NewCaptureWriter(rec)
	cw.Header().Set("Content-Type", "text/html")
	cw.WriteHeader(200)
	cw.Flush()
	if toolbarEligible(cw, req) {
		t.Error("toolbarEligible true for a flushed response")
	}
}

func TestInjectBeforeBodyEnd(t *testing.T) {
	toolbar := []byte("<div id='tb'></div>")
	t.Run("splices before last body close", func(t *testing.T) {
		got := string(injectBeforeBodyEnd([]byte("<html><body>a</body></html>"), toolbar))
		want := "<html><body>a" + string(toolbar) + "</body></html>"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})
	t.Run("case-insensitive close", func(t *testing.T) {
		got := string(injectBeforeBodyEnd([]byte("<html><BODY>a</BODY></html>"), toolbar))
		if !strings.HasPrefix(got, "<html><BODY>a<div") {
			t.Errorf("got %q", got)
		}
	})
	t.Run("appends without close tag", func(t *testing.T) {
		got := string(injectBeforeBodyEnd([]byte("<p>fragment"), toolbar))
		if got != "<p>fragment"+string(toolbar) {
			t.Errorf("got %q", got)
		}
	})
	t.Run("keeps trailing script after body close", func(t *testing.T) {
		got := string(injectBeforeBodyEnd([]byte("<body>x</body><!-- c -->"), toolbar))
		if !strings.HasSuffix(got, "</body><!-- c -->") || strings.Index(got, "<div") > strings.Index(got, "</body>") {
			t.Errorf("got %q", got)
		}
	})
}

func TestConcurrentRequestsKeepTheirOwnLogs(t *testing.T) {
	// Real server, real goroutines: overlapping requests must not bleed
	// records into each other — the routing target attributes by the
	// goroutine that emits, and each request is served on its own.
	h, p, _ := newPanelServer(t, nil)
	h.Get("/work", func(c *web.Context) error {
		msg := c.Request().URL.Query().Get("msg")
		c.App().Log().Info(msg + "-begin")
		time.Sleep(10 * time.Millisecond) // keep the requests overlapping
		c.App().Log().Info(msg + "-end")
		return writeHTML(c, "<html><body>x</body></html>")
	})
	srv := httptest.NewServer(h)
	defer srv.Close()

	const workers = 8
	var wg sync.WaitGroup
	for i := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := http.Get(srv.URL + "/work?msg=worker" + strconv.Itoa(i))
			if err != nil {
				t.Errorf("request %d: %v", i, err)
				return
			}
			resp.Body.Close()
		}()
	}
	wg.Wait()

	entries, err := p.history(workers)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(entries) != workers {
		t.Fatalf("recorded %d entries, want %d", len(entries), workers)
	}
	seen := map[string]bool{}
	for _, e := range entries {
		if len(e.Logs) != 2 {
			t.Errorf("entry %s carries %d log records (%v), want its own 2", e.ID, len(e.Logs), e.Logs)
			continue
		}
		begin, end := e.Logs[0].Message, e.Logs[1].Message
		if !strings.HasSuffix(begin, "-begin") || !strings.HasSuffix(end, "-end") || begin[:len(begin)-6] != end[:len(end)-4] {
			t.Errorf("entry %s mixed records: %q / %q", e.ID, begin, end)
		}
		seen[begin] = true
	}
	if len(seen) != workers {
		t.Errorf("%d distinct workers recorded, want %d", len(seen), workers)
	}
}

func TestPanickingActionGetsErrorPageAndRecord(t *testing.T) {
	// A panic unwinds through the capture: the pipeline's recovery must
	// still reach the client (the Context writer restored), and the
	// panicked request must be recorded.
	h, p, _ := newPanelServer(t, nil)
	h.Get("/panic", func(c *web.Context) error {
		panic("kaboom")
	})
	rec := serve(h, http.MethodGet, "/panic", "127.0.0.1:1000")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if rec.Body.Len() == 0 {
		t.Fatal("error page never reached the client")
	}
	if strings.Contains(rec.Body.String(), toolbarMark) {
		t.Error("toolbar injected into a panic error page")
	}
	e := singleEntry(t, p)
	if e.Request.Status != http.StatusInternalServerError || !strings.Contains(e.Request.Error, "kaboom") {
		t.Errorf("panic record = %+v", e.Request)
	}
}
func TestPanelPagesGetOnly(t *testing.T) {
	// CSRF sits outside the capture and answers tokenless POSTs first, so
	// the method guard is exercised with it disabled.
	h, _, _ := newPanelServer(t, web.WithoutCsrf())
	rec := serve(h, http.MethodPost, "/debug", "127.0.0.1:1")
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /debug status = %d, want 405", rec.Code)
	}
	if allow := rec.Header().Get("Allow"); allow != http.MethodGet {
		t.Errorf("Allow = %q, want GET", allow)
	}
}
