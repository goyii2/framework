package web_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/goyii2/framework/base"
	"github.com/goyii2/framework/session"
	"github.com/goyii2/framework/web"
)

// fileSessionManager returns a file-backed session manager rooted in a
// per-test temporary directory.
func fileSessionManager(t *testing.T) *session.Manager {
	t.Helper()
	m, err := session.New(session.Config{Name: "goyii-session", Backend: "file", Path: t.TempDir(), Timeout: 1440})
	if err != nil {
		t.Fatalf("session.New(file): %v", err)
	}
	return m
}

// redisSessionManager returns a redis-backed session manager pointing at an
// in-process miniredis server (pure Go, no real redis needed).
func redisSessionManager(t *testing.T) *session.Manager {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	m, err := session.New(session.Config{Name: "goyii-session", Backend: "redis", Timeout: 1440}, session.WithRedis(client))
	if err != nil {
		t.Fatalf("session.New(redis): %v", err)
	}
	return m
}

// sessionApp assembles an application whose actions exercise the session
// lifecycle: writing values (two keys to observe merge-on-load), reading them
// back, removing one, destroying the session and rotating the session id.
// CSRF is off: these tests exercise session mechanics, not form tokens.
func sessionApp(t *testing.T, m *session.Manager) *web.Handler {
	t.Helper()
	app := base.New()
	app.SetSession(m)
	h := web.NewHandler(app, web.WithoutCsrf())
	h.Get("/write", func(c *web.Context) error {
		c.Session().Set("k", "v")
		return c.JSON(map[string]string{"ok": "yes"})
	})
	h.Get("/write2", func(c *web.Context) error {
		c.Session().Set("k2", "v2")
		return c.JSON(map[string]string{"ok": "yes"})
	})
	h.Get("/read", func(c *web.Context) error {
		return c.JSON(map[string]string{"k": c.Session().GetString("k"), "k2": c.Session().GetString("k2")})
	})
	h.Get("/remove", func(c *web.Context) error {
		c.Session().Remove("k")
		return c.JSON(map[string]string{"ok": "yes"})
	})
	h.Get("/destroy", func(c *web.Context) error {
		c.Session().Destroy()
		return c.JSON(map[string]string{"ok": "yes"})
	})
	h.Get("/regen", func(c *web.Context) error {
		c.Session().Set("k", "kept")
		c.Session().RegenerateID()
		return c.JSON(map[string]string{"ok": "yes"})
	})
	return h
}

// sessionReq performs a request carrying the given cookies and returns the
// raw response.
func sessionReq(t *testing.T, h http.Handler, method, target string, cookies ...*http.Cookie) *http.Response {
	t.Helper()
	r := httptest.NewRequest(method, target, nil)
	for _, ck := range cookies {
		r.AddCookie(ck)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec.Result()
}

// sessionCookieOf extracts the named cookie from a response, nil when absent.
func sessionCookieOf(t *testing.T, res *http.Response, name string) *http.Cookie {
	t.Helper()
	for _, ck := range res.Cookies() {
		if ck.Name == name {
			return ck
		}
	}
	return nil
}

// readSession performs GET /read and returns the decoded body.
func readSession(t *testing.T, h http.Handler, cookies ...*http.Cookie) map[string]string {
	t.Helper()
	res := sessionReq(t, h, http.MethodGet, "/read", cookies...)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d reading session", res.StatusCode)
	}
	var body map[string]string
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("decoding body: %v", err)
	}
	return body
}

// fileIDs lists the session ids persisted under the file backend directory.
func fileIDs(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("listing session dir: %v", err)
	}
	var ids []string
	for _, e := range entries {
		ids = append(ids, e.Name())
	}
	return ids
}

// loadSessionRecord reads a session record straight from the manager's
// backend, bypassing the request machinery — tests assert on persistence
// itself, not just on observed behavior.
func loadSessionRecord(t *testing.T, m *session.Manager, id string) (map[string]any, bool) {
	t.Helper()
	data, ok, err := m.Store.Load(context.Background(), id, time.Minute)
	if err != nil {
		t.Fatalf("loading session record: %v", err)
	}
	return data, ok
}

func TestSessionUnusedIssuesNoCookieAndNoBackendRecord(t *testing.T) {
	dir := t.TempDir()
	m, err := session.New(session.Config{Name: "goyii-session", Backend: "file", Path: dir, Timeout: 1440})
	if err != nil {
		t.Fatalf("session.New: %v", err)
	}
	h := sessionApp(t, m)

	// An action that never touches the session must not force one open: no
	// cookie is issued and no record lands in the backend.
	h.Get("/plain", func(c *web.Context) error {
		return c.JSON(map[string]string{"ok": "yes"})
	})
	res := sessionReq(t, h, http.MethodGet, "/plain")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", res.StatusCode)
	}
	if ck := sessionCookieOf(t, res, "goyii-session"); ck != nil {
		t.Fatalf("unused session issued cookie %q", ck.Value)
	}
	if ids := fileIDs(t, dir); len(ids) != 0 {
		t.Fatalf("unused session wrote backend records %v", ids)
	}
}

func TestSessionWritePersistsAcrossRequests(t *testing.T) {
	h := sessionApp(t, fileSessionManager(t))

	res := sessionReq(t, h, http.MethodGet, "/write")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d writing session", res.StatusCode)
	}
	ck := sessionCookieOf(t, res, "goyii-session")
	if ck == nil || ck.Value == "" {
		t.Fatal("writing the session issued no id cookie")
	}
	if !ck.HttpOnly {
		t.Error("session cookie must be HttpOnly")
	}
	if ck.SameSite != http.SameSiteLaxMode {
		t.Errorf("session cookie SameSite = %v, want Lax", ck.SameSite)
	}
	if ck.Path != "/" {
		t.Errorf("session cookie Path = %q, want /", ck.Path)
	}

	// A second request carrying the cookie sees the value; writing another
	// key preserves the first (load-then-modify, not replace).
	if body := readSession(t, h, ck); body["k"] != "v" {
		t.Fatalf("k = %q, want v", body["k"])
	}
	res = sessionReq(t, h, http.MethodGet, "/write2", ck)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d writing second key", res.StatusCode)
	}
	if body := readSession(t, h, ck); body["k"] != "v" || body["k2"] != "v2" {
		t.Fatalf("after second write k=%q k2=%q, want v and v2", body["k"], body["k2"])
	}
}

// TestSessionRedisBackendRoundTrips runs the write/read cycle over the redis
// backend: the same behavior the file backend shows, records on the redis
// server.
func TestSessionRedisBackendRoundTrips(t *testing.T) {
	h := sessionApp(t, redisSessionManager(t))

	ck := sessionCookieOf(t, sessionReq(t, h, http.MethodGet, "/write"), "goyii-session")
	if ck == nil || ck.Value == "" {
		t.Fatal("writing the session issued no id cookie")
	}
	if body := readSession(t, h, ck); body["k"] != "v" {
		t.Fatalf("k = %q through the redis backend, want v", body["k"])
	}
}

func TestSessionRemoveDropsKey(t *testing.T) {
	h := sessionApp(t, fileSessionManager(t))

	ck := sessionCookieOf(t, sessionReq(t, h, http.MethodGet, "/write"), "goyii-session")
	res := sessionReq(t, h, http.MethodGet, "/remove", ck)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d removing key", res.StatusCode)
	}
	if body := readSession(t, h, ck); body["k"] != "" {
		t.Fatalf("k = %q after removal, want empty", body["k"])
	}
}

func TestSessionDestroyClearsCookieAndBackend(t *testing.T) {
	dir := t.TempDir()
	m, err := session.New(session.Config{Name: "goyii-session", Backend: "file", Path: dir, Timeout: 1440})
	if err != nil {
		t.Fatalf("session.New: %v", err)
	}
	h := sessionApp(t, m)

	ck := sessionCookieOf(t, sessionReq(t, h, http.MethodGet, "/write"), "goyii-session")
	if n := len(fileIDs(t, dir)); n != 1 {
		t.Fatalf("session dir holds %d records, want 1", n)
	}

	res := sessionReq(t, h, http.MethodGet, "/destroy", ck)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d destroying session", res.StatusCode)
	}
	if ck2 := sessionCookieOf(t, res, "goyii-session"); ck2 == nil || ck2.MaxAge >= 0 {
		t.Fatalf("destroy must clear the cookie, got %+v", ck2)
	}
	if ids := fileIDs(t, dir); len(ids) != 0 {
		t.Fatalf("session dir still holds %v after destroy", ids)
	}

	// The cleared id no longer resolves: a client retrying with it starts
	// from an empty session instead of resurrecting the data.
	if body := readSession(t, h, ck); body["k"] != "" {
		t.Fatalf("k = %q after destroy, want empty", body["k"])
	}
}

func TestSessionWriteAfterDestroyStaysDead(t *testing.T) {
	dir := t.TempDir()
	m, err := session.New(session.Config{Name: "goyii-session", Backend: "file", Path: dir, Timeout: 1440})
	if err != nil {
		t.Fatalf("session.New: %v", err)
	}
	h := sessionApp(t, m)

	ck := sessionCookieOf(t, sessionReq(t, h, http.MethodGet, "/write"), "goyii-session")
	res := sessionReq(t, h, http.MethodGet, "/destroy", ck)
	clearing := sessionCookieOf(t, res, "goyii-session")
	if clearing == nil || clearing.MaxAge >= 0 {
		t.Fatalf("destroy must clear the cookie, got %+v", clearing)
	}

	// A write issued after the destroy in the same request (an action
	// logging out and then touching the session) must not resurrect the
	// session or replace the clearing cookie.
	h.Get("/destroy-then-write", func(c *web.Context) error {
		c.Session().Destroy()
		c.Session().Set("k", "zombie")
		return c.JSON(map[string]string{"ok": "yes"})
	})
	res = sessionReq(t, h, http.MethodGet, "/destroy-then-write", ck)
	ck2 := sessionCookieOf(t, res, "goyii-session")
	if ck2 == nil || ck2.MaxAge >= 0 {
		t.Fatalf("write after destroy replaced the clearing cookie: %+v", ck2)
	}
	if ids := fileIDs(t, dir); len(ids) != 0 {
		t.Fatalf("session dir holds %v after destroy-then-write, want empty", ids)
	}
}

func TestSessionRegenerateIDRotatesCookieAndInvalidatesOldID(t *testing.T) {
	dir := t.TempDir()
	m, err := session.New(session.Config{Name: "goyii-session", Backend: "file", Path: dir, Timeout: 1440})
	if err != nil {
		t.Fatalf("session.New: %v", err)
	}
	h := sessionApp(t, m)

	ck := sessionCookieOf(t, sessionReq(t, h, http.MethodGet, "/write"), "goyii-session")
	res := sessionReq(t, h, http.MethodGet, "/regen", ck)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d regenerating id", res.StatusCode)
	}
	ck2 := sessionCookieOf(t, res, "goyii-session")
	if ck2 == nil || ck2.Value == "" || ck2.Value == ck.Value {
		t.Fatalf("regenerated cookie = %+v, want a fresh value distinct from %q", ck2, ck.Value)
	}

	// Data written before the rotation survives under the new id...
	if body := readSession(t, h, ck2); body["k"] != "kept" {
		t.Fatalf("k = %q under new id, want kept", body["k"])
	}
	// ...while the old id is gone from the backend.
	if _, err := os.Stat(filepath.Join(dir, ck.Value)); !os.IsNotExist(err) {
		t.Fatalf("old session record still present after regeneration (stat err = %v)", err)
	}
}

func TestSessionUnknownCookieIDStartsFresh(t *testing.T) {
	dir := t.TempDir()
	m, err := session.New(session.Config{Name: "goyii-session", Backend: "file", Path: dir, Timeout: 1440})
	if err != nil {
		t.Fatalf("session.New: %v", err)
	}
	h := sessionApp(t, m)

	ck := &http.Cookie{Name: "goyii-session", Value: "forged-or-expired-id"}
	res := sessionReq(t, h, http.MethodGet, "/write", ck)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d with unknown id", res.StatusCode)
	}
	ck2 := sessionCookieOf(t, res, "goyii-session")
	if ck2 == nil || ck2.Value == "" || ck2.Value == ck.Value {
		t.Fatalf("unknown id must yield a fresh id cookie, got %+v", ck2)
	}
	if _, err := os.Stat(filepath.Join(dir, ck.Value)); !os.IsNotExist(err) {
		t.Fatalf("unknown id must not be persisted (stat err = %v)", err)
	}
	if body := readSession(t, h, ck2); body["k"] != "v" {
		t.Fatalf("k = %q under fresh id, want v", body["k"])
	}
}

func TestSessionFileBackendExpiresAfterTimeout(t *testing.T) {
	dir := t.TempDir()
	m, err := session.New(session.Config{Name: "goyii-session", Backend: "file", Path: dir, Timeout: 2})
	if err != nil {
		t.Fatalf("session.New: %v", err)
	}
	h := sessionApp(t, m)

	ck := sessionCookieOf(t, sessionReq(t, h, http.MethodGet, "/write"), "goyii-session")
	// Simulate the record sitting idle past the timeout by backdating its
	// modification time (the file backend's expiry base).
	past := time.Now().Add(-3 * time.Second)
	if err := os.Chtimes(filepath.Join(dir, ck.Value), past, past); err != nil {
		t.Fatalf("backdating session file: %v", err)
	}
	if body := readSession(t, h, ck); body["k"] != "" {
		t.Fatalf("k = %q after expiry, want empty", body["k"])
	}
}

// TestSessionConcurrentRequests drives parallel requests through the full
// middleware pipeline, each with its own session cookie; run under -race it
// guards the request-scoped session wiring.
func TestSessionConcurrentRequests(t *testing.T) {
	h := sessionApp(t, fileSessionManager(t))

	const workers = 16
	var wg sync.WaitGroup
	for w := range workers {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			res := sessionReq(t, h, http.MethodGet, "/write")
			ck := sessionCookieOf(t, res, "goyii-session")
			if ck == nil {
				t.Errorf("worker %d: no session cookie", w)
				return
			}
			for i := range 5 {
				res := sessionReq(t, h, http.MethodGet, "/write2", ck)
				if res.StatusCode != http.StatusOK {
					t.Errorf("worker %d pass %d: status %d", w, i, res.StatusCode)
					return
				}
				body := readSession(t, h, ck)
				if body["k"] != "v" {
					t.Errorf("worker %d pass %d: k = %q, want v", w, i, body["k"])
					return
				}
			}
		}(w)
	}
	wg.Wait()
}
