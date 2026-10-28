package web_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/goyii2/framework/base"
	"github.com/goyii2/framework/config"
	"github.com/goyii2/framework/web"
	"github.com/redis/go-redis/v9"
)

// fileSessionConfig returns a file-backed session configuration rooted in a
// per-test temporary directory.
func fileSessionConfig(t *testing.T) config.Session {
	t.Helper()
	return config.Session{Name: "goyii-session", Backend: "file", Path: t.TempDir(), Timeout: 1440}
}

// redisSessionConfig returns a redis-backed session configuration pointing at
// an in-process miniredis server (pure Go, no real redis needed).
func redisSessionConfig(t *testing.T) config.Session {
	t.Helper()
	mr := miniredis.RunT(t)
	return config.Session{Name: "goyii-session", Backend: "redis", Timeout: 1440,
		Redis: config.Redis{Addr: mr.Addr()}}
}

// sessionApp assembles an application whose actions exercise the session
// lifecycle: writing values (two keys to observe merge-on-load), reading them
// back, removing one, destroying the session and rotating the session id.
// CSRF is off: these tests exercise session mechanics, not form tokens.
func sessionApp(t *testing.T, cfg config.Session) *web.Handler {
	t.Helper()
	app := base.New(config.Config{Web: config.Web{DisableCsrf: true}, Session: cfg})
	h := web.NewHandler(app)
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

func TestSessionUnusedIssuesNoCookieAndNoBackendRecord(t *testing.T) {
	dir := t.TempDir()
	h := sessionApp(t, config.Session{Name: "goyii-session", Backend: "file", Path: dir, Timeout: 1440})

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
	h := sessionApp(t, fileSessionConfig(t))

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

func TestSessionRemoveDropsKey(t *testing.T) {
	h := sessionApp(t, fileSessionConfig(t))

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
	cfg := fileSessionConfig(t)
	h := sessionApp(t, cfg)

	ck := sessionCookieOf(t, sessionReq(t, h, http.MethodGet, "/write"), "goyii-session")
	if n := len(fileIDs(t, cfg.Path)); n != 1 {
		t.Fatalf("session dir holds %d records, want 1", n)
	}

	res := sessionReq(t, h, http.MethodGet, "/destroy", ck)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d destroying session", res.StatusCode)
	}
	if ck2 := sessionCookieOf(t, res, "goyii-session"); ck2 == nil || ck2.MaxAge >= 0 {
		t.Fatalf("destroy must clear the cookie, got %+v", ck2)
	}
	if ids := fileIDs(t, cfg.Path); len(ids) != 0 {
		t.Fatalf("session dir still holds %v after destroy", ids)
	}

	// The cleared id no longer resolves: a client retrying with it starts
	// from an empty session instead of resurrecting the data.
	if body := readSession(t, h, ck); body["k"] != "" {
		t.Fatalf("k = %q after destroy, want empty", body["k"])
	}
}

func TestSessionWriteAfterDestroyStaysDead(t *testing.T) {
	cfg := fileSessionConfig(t)
	h := sessionApp(t, cfg)

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
	if ids := fileIDs(t, cfg.Path); len(ids) != 0 {
		t.Fatalf("session dir holds %v after destroy-then-write, want empty", ids)
	}
}

func TestSessionRegenerateIDRotatesCookieAndInvalidatesOldID(t *testing.T) {
	cfg := fileSessionConfig(t)
	h := sessionApp(t, cfg)

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
	if _, err := os.Stat(filepath.Join(cfg.Path, ck.Value)); !os.IsNotExist(err) {
		t.Fatalf("old session record still present after regeneration (stat err = %v)", err)
	}
}

func TestSessionUnknownCookieIDStartsFresh(t *testing.T) {
	cfg := fileSessionConfig(t)
	h := sessionApp(t, cfg)

	ck := &http.Cookie{Name: "goyii-session", Value: "forged-or-expired-id"}
	res := sessionReq(t, h, http.MethodGet, "/write", ck)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d with unknown id", res.StatusCode)
	}
	ck2 := sessionCookieOf(t, res, "goyii-session")
	if ck2 == nil || ck2.Value == "" || ck2.Value == ck.Value {
		t.Fatalf("unknown id must yield a fresh id cookie, got %+v", ck2)
	}
	if _, err := os.Stat(filepath.Join(cfg.Path, ck.Value)); !os.IsNotExist(err) {
		t.Fatalf("unknown id must not be persisted (stat err = %v)", err)
	}
	if body := readSession(t, h, ck2); body["k"] != "v" {
		t.Fatalf("k = %q under fresh id, want v", body["k"])
	}
}

func TestSessionFileBackendExpiresAfterTimeout(t *testing.T) {
	cfg := fileSessionConfig(t)
	cfg.Timeout = 2 // seconds
	h := sessionApp(t, cfg)

	ck := sessionCookieOf(t, sessionReq(t, h, http.MethodGet, "/write"), "goyii-session")
	// Simulate the record sitting idle past the timeout by backdating its
	// modification time (the file backend's expiry base).
	past := time.Now().Add(-3 * time.Second)
	if err := os.Chtimes(filepath.Join(cfg.Path, ck.Value), past, past); err != nil {
		t.Fatalf("backdating session file: %v", err)
	}
	if body := readSession(t, h, ck); body["k"] != "" {
		t.Fatalf("k = %q after expiry, want empty", body["k"])
	}
}

// loadSessionRecord reads a session record straight from the backend the
// configuration selects, bypassing the store interfaces — tests assert on
// persistence itself, not just on observed behavior.
func loadSessionRecord(t *testing.T, cfg config.Session, id string) (map[string]any, bool) {
	t.Helper()
	if cfg.Backend == "redis" {
		client := redis.NewClient(&redis.Options{Addr: cfg.Redis.Addr})
		defer client.Close()
		b, err := client.Get(t.Context(), "goyii:session:"+id).Bytes()
		if err == redis.Nil {
			return nil, false
		}
		if err != nil {
			t.Fatalf("reading redis session record: %v", err)
		}
		var data map[string]any
		if err := json.Unmarshal(b, &data); err != nil {
			t.Fatalf("decoding redis session record: %v", err)
		}
		return data, true
	}
	b, err := os.ReadFile(filepath.Join(cfg.Path, id))
	if os.IsNotExist(err) {
		return nil, false
	}
	if err != nil {
		t.Fatalf("reading file session record: %v", err)
	}
	var data map[string]any
	if err := json.Unmarshal(b, &data); err != nil {
		t.Fatalf("decoding file session record: %v", err)
	}
	return data, true
}

// sid pads a label into the shape of a minted session id (at least 22
// URL-safe base64 characters): store-level ids must look like the ones the
// manager mints, since they double as file names.
func sid(label string) string {
	for len(label) < 24 {
		label += "-0"
	}
	return label
}

// sessionStoreCase is the behavioral matrix both backends must satisfy
// identically. expire simulates the record outliving its timeout without
// waiting on wall-clock time.
type sessionStoreCase struct {
	name   string
	store  web.SessionStore
	expire func(id string)
}

func sessionStoreCases(t *testing.T, ttl time.Duration) []sessionStoreCase {
	t.Helper()

	dir := t.TempDir()
	file := web.NewFileSessionStore(dir)

	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	redisStore := web.NewRedisSessionStore(client, "goyii:session:")

	return []sessionStoreCase{
		{
			name:  "file",
			store: file,
			expire: func(id string) {
				past := time.Now().Add(-(ttl + time.Second))
				if err := os.Chtimes(filepath.Join(dir, id), past, past); err != nil {
					t.Fatalf("backdating session file: %v", err)
				}
			},
		},
		{
			name:  "redis",
			store: redisStore,
			expire: func(id string) {
				mr.FastForward(ttl + time.Second)
			},
		},
	}
}

// TestSessionStoreBehaviorConsistency runs one scenario table over the file
// and redis backends: both must load, round-trip, overwrite, destroy and
// expire identically.
func TestSessionStoreBehaviorConsistency(t *testing.T) {
	ttl := 30 * time.Second
	ctx := t.Context()

	for _, tc := range sessionStoreCases(t, ttl) {
		t.Run(tc.name, func(t *testing.T) {
			// Round trip: values survive a save/load cycle. JSON encoding
			// turns numbers into float64 on the way back, so the expectation
			// matches that shape.
			data := map[string]any{"user": "alice", "count": 7, "nested": map[string]any{"a": "b"}}
			if err := tc.store.Save(ctx, sid("s1"), data, ttl); err != nil {
				t.Fatalf("save: %v", err)
			}
			loaded, ok, err := tc.store.Load(ctx, sid("s1"), ttl)
			if err != nil || !ok {
				t.Fatalf("load after save: ok=%v err=%v", ok, err)
			}
			if loaded["user"] != "alice" || loaded["nested"].(map[string]any)["a"] != "b" {
				t.Fatalf("round trip mismatch: %#v", loaded)
			}
			if n := loaded["count"].(float64); n != 7 {
				t.Fatalf("count = %v, want 7", n)
			}

			// Unknown ids report missing, never an error nor a session.
			if _, ok, err := tc.store.Load(ctx, sid("none"), ttl); ok || err != nil {
				t.Fatalf("load unknown id: ok=%v err=%v, want ok=false err=nil", ok, err)
			}

			// A save replaces the record wholesale: keys dropped from the
			// map disappear instead of merging into the old record.
			if err := tc.store.Save(ctx, sid("s1"), map[string]any{"only": "one"}, ttl); err != nil {
				t.Fatalf("overwrite save: %v", err)
			}
			loaded, ok, err = tc.store.Load(ctx, sid("s1"), ttl)
			if err != nil || !ok {
				t.Fatalf("load after overwrite: ok=%v err=%v", ok, err)
			}
			if len(loaded) != 1 || loaded["only"] != "one" {
				t.Fatalf("overwrite left residue: %#v", loaded)
			}

			// Destroy removes the record; destroying again stays clean.
			if err := tc.store.Destroy(ctx, sid("s1")); err != nil {
				t.Fatalf("destroy: %v", err)
			}
			if err := tc.store.Destroy(ctx, sid("s1")); err != nil {
				t.Fatalf("destroy twice: %v", err)
			}
			if _, ok, _ := tc.store.Load(ctx, sid("s1"), ttl); ok {
				t.Fatal("destroyed id still loads")
			}

			// Records idle past the timeout expire: a load reports missing
			// instead of resurrecting stale data.
			if err := tc.store.Save(ctx, sid("s2"), map[string]any{"k": "v"}, ttl); err != nil {
				t.Fatalf("save s2: %v", err)
			}
			tc.expire(sid("s2"))
			if _, ok, _ := tc.store.Load(ctx, sid("s2"), ttl); ok {
				t.Fatal("expired id still loads")
			}
		})
	}
}

// TestSessionStoreConcurrentAccess hammers both backends from parallel
// goroutines; beyond correctness it guards the -race build (store internals
// must be safe for concurrent use).
func TestSessionStoreConcurrentAccess(t *testing.T) {
	ttl := 30 * time.Second
	ctx := t.Context()

	for _, tc := range sessionStoreCases(t, ttl) {
		t.Run(tc.name, func(t *testing.T) {
			const workers, ops = 16, 25
			var wg sync.WaitGroup
			for w := range workers {
				wg.Add(1)
				go func(w int) {
					defer wg.Done()
					id := sid(fmt.Sprintf("cw-%d", w))
					for i := range ops {
						if err := tc.store.Save(ctx, id, map[string]any{"w": w, "i": i}, ttl); err != nil {
							t.Errorf("worker %d save: %v", w, err)
							return
						}
						if _, ok, err := tc.store.Load(ctx, id, ttl); !ok || err != nil {
							t.Errorf("worker %d load: ok=%v err=%v", w, ok, err)
							return
						}
						// A shared id exercises cross-goroutine contention.
						if _, _, err := tc.store.Load(ctx, sid("shared"), ttl); err != nil {
							t.Errorf("worker %d shared load: %v", w, err)
							return
						}
					}
					if err := tc.store.Destroy(ctx, id); err != nil {
						t.Errorf("worker %d destroy: %v", w, err)
					}
				}(w)
			}
			wg.Wait()
		})
	}
}

// TestSessionConcurrentRequests drives parallel requests through the full
// middleware pipeline, each with its own session cookie; run under -race it
// guards the request-scoped session wiring.
func TestSessionConcurrentRequests(t *testing.T) {
	h := sessionApp(t, fileSessionConfig(t))

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
