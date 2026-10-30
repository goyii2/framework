package web_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/goyii2/framework/base"
	"github.com/goyii2/framework/cache"
	"github.com/goyii2/framework/web"
)

// pageCacheApp builds a handler whose /page route renders a text/html page
// counting how often the action ran; the body carries the current version
// so replays and rebuilds are distinguishable.
func pageCacheApp(t *testing.T, pc web.PageCache, app *base.Application, version *string, runs *int) *web.Handler {
	t.Helper()
	if app == nil {
		app = base.New()
	}
	h := web.NewHandler(app, web.WithoutCsrf())
	h.Use(pc.Middleware())
	h.Get("/page", func(c *web.Context) error {
		*runs++
		c.Response().Header().Set("Content-Type", "text/html; charset=utf-8")
		_, err := c.Response().Write([]byte("<html><body>page " + *version + "</body></html>"))
		return err
	})
	return h
}

// pageReq performs one request against h carrying header and returns the
// raw response.
func pageReq(t *testing.T, h http.Handler, method, target, header, headerValue string) *http.Response {
	t.Helper()
	r := httptest.NewRequest(method, target, nil)
	if header != "" {
		r.Header.Set(header, headerValue)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec.Result()
}

func TestPageCacheHitReplaysWithoutAction(t *testing.T) {
	version := "v1"
	runs := 0
	pc := web.PageCache{Duration: time.Minute}
	h := pageCacheApp(t, pc, nil, &version, &runs)

	first := req(t, h, http.MethodGet, "/page")
	body, _ := io.ReadAll(first.Body)
	if first.StatusCode != http.StatusOK || runs != 1 || string(body) != "<html><body>page v1</body></html>" {
		t.Fatalf("first fetch: status = %d, runs = %d, body = %q", first.StatusCode, runs, body)
	}

	second := req(t, h, http.MethodGet, "/page")
	body, _ = io.ReadAll(second.Body)
	if second.StatusCode != http.StatusOK {
		t.Fatalf("replay status = %d, want 200", second.StatusCode)
	}
	if runs != 1 {
		t.Errorf("action ran %d times on a cache hit, want 1", runs)
	}
	if string(body) != "<html><body>page v1</body></html>" {
		t.Errorf("replay body = %q, want the cached page", body)
	}
	if got := second.Header.Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Errorf("replay Content-Type = %q, want the cached content type", got)
	}
}

func TestPageCacheStoresEntryInAppComponentWithTTLAndDependency(t *testing.T) {
	version := "v1"
	runs := 0
	dep := cache.NewTimeDependency(time.Now().Add(time.Hour))
	recording := &recordingPageCache{Cache: cache.NewMemory(time.Hour)}
	app := base.New()
	app.SetCache(recording)
	h := pageCacheApp(t, web.PageCache{Duration: 2 * time.Minute, Dependency: dep}, app, &version, &runs)

	req(t, h, http.MethodGet, "/page")

	found, err := app.Cache().Exists(context.Background(), "/page")
	if err != nil || !found {
		t.Fatalf("entry under key %q in the application cache: found = %v, err = %v", "/page", found, err)
	}
	if recording.ttl != 2*time.Minute {
		t.Errorf("stored ttl = %v, want the filter's Duration", recording.ttl)
	}
	if recording.dep != dep {
		t.Error("dependency passed to the cache component must be the filter's Dependency")
	}
}

func TestPageCacheKeyNormalizesQueryOrder(t *testing.T) {
	version := "v1"
	runs := 0
	h := pageCacheApp(t, web.PageCache{Duration: time.Minute}, nil, &version, &runs)

	req(t, h, http.MethodGet, "/page?sort=id&filter=new")
	req(t, h, http.MethodGet, "/page?filter=new&sort=id")

	if runs != 1 {
		t.Errorf("action ran %d times for reordered query parameters, want 1 (one cache entry)", runs)
	}
	// A different parameter value is a different page.
	req(t, h, http.MethodGet, "/page?sort=id&filter=old")
	if runs != 2 {
		t.Errorf("action ran %d times after a parameter value changed, want 2 (separate entry)", runs)
	}
}

func TestPageCacheDependencyInvalidationRebuilds(t *testing.T) {
	dep := &versionDependency{version: "v1"}
	version := "v1"
	runs := 0
	h := pageCacheApp(t, web.PageCache{Duration: time.Hour, Dependency: dep}, nil, &version, &runs)

	assertBody := func(want string, wantRuns int) {
		t.Helper()
		res := req(t, h, http.MethodGet, "/page")
		body, _ := io.ReadAll(res.Body)
		if string(body) != "<html><body>page "+want+"</body></html>" || runs != wantRuns {
			t.Fatalf("body = %q, runs = %d, want page %q rendered %d times", body, runs, want, wantRuns)
		}
	}

	assertBody("v1", 1)
	assertBody("v1", 1) // unchanged dependency: replay

	dep.version = "v2"
	version = "v2"
	assertBody("v2", 2) // invalidated: rebuild
	assertBody("v2", 2) // new fingerprint: replay again
}

func TestPageCacheVaryBySplitsVariants(t *testing.T) {
	version := "v1"
	runs := 0
	pc := web.PageCache{
		Duration: time.Minute,
		VaryBy:   func(r *http.Request) string { return r.Header.Get("Accept-Language") },
	}
	h := pageCacheApp(t, pc, nil, &version, &runs)

	en1 := pageReq(t, h, http.MethodGet, "/page", "Accept-Language", "en")
	de1 := pageReq(t, h, http.MethodGet, "/page", "Accept-Language", "de")
	if runs != 2 {
		t.Fatalf("action ran %d times for two VaryBy variants, want 2", runs)
	}

	en2 := pageReq(t, h, http.MethodGet, "/page", "Accept-Language", "en")
	de2 := pageReq(t, h, http.MethodGet, "/page", "Accept-Language", "de")
	if runs != 2 {
		t.Errorf("action ran %d times when both variants replay, want 2", runs)
	}
	if en1.Body == nil || de1.Body == nil {
		t.Fatal("missing bodies")
	}
	en1Body, _ := io.ReadAll(en1.Body)
	en2Body, _ := io.ReadAll(en2.Body)
	de1Body, _ := io.ReadAll(de1.Body)
	de2Body, _ := io.ReadAll(de2.Body)
	if string(en1Body) != string(en2Body) || string(de1Body) != string(de2Body) {
		t.Error("each VaryBy variant must replay its own cached page")
	}
}

func TestPageCacheSkipsWriteVerbs(t *testing.T) {
	version := "v1"
	runs := 0
	app := base.New()
	h := pageCacheApp(t, web.PageCache{Duration: time.Minute}, app, &version, &runs)
	h.Post("/page", func(c *web.Context) error {
		runs++
		c.Response().Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, err := c.Response().Write([]byte("posted"))
		return err
	})

	res := req(t, h, http.MethodPost, "/page")
	if res.StatusCode != http.StatusOK || runs != 1 {
		t.Fatalf("POST status = %d, runs = %d, want the action served", res.StatusCode, runs)
	}
	// The POST warmed no entry: the first GET still renders.
	req(t, h, http.MethodGet, "/page")
	if runs != 2 {
		t.Errorf("runs = %d after POST + first GET, want 2 (POST must not read or seed the cache)", runs)
	}
	found, err := app.Cache().Exists(context.Background(), "/page")
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}
	if !found {
		t.Error("the GET should have seeded the page entry")
	}
}

func TestPageCacheSkipsHeadRequests(t *testing.T) {
	version := "v1"
	runs := 0
	h := pageCacheApp(t, web.PageCache{Duration: time.Minute}, nil, &version, &runs)

	// ServeMux routes HEAD onto the GET handler; the filter passes HEAD
	// through so the shared key is never seeded with a HEAD's empty body.
	req(t, h, http.MethodHead, "/page")
	req(t, h, http.MethodHead, "/page")
	if runs != 2 {
		t.Errorf("action ran %d times for two HEAD requests, want 2 (HEAD never replays)", runs)
	}
}

func TestPageCacheSkipsNonTextResponses(t *testing.T) {
	runs := 0
	app := base.New()
	h := web.NewHandler(app, web.WithoutCsrf())
	h.Use(web.PageCache{Duration: time.Minute}.Middleware())
	h.Get("/data", func(c *web.Context) error {
		runs++
		return c.JSON(map[string]int{"n": runs})
	})

	for i := 0; i < 2; i++ {
		res := req(t, h, http.MethodGet, "/data")
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want the action served", res.StatusCode)
		}
	}
	if runs != 2 {
		t.Errorf("action ran %d times, want 2 (JSON responses stay uncached)", runs)
	}
}

func TestPageCacheSkipsNonSuccessStatuses(t *testing.T) {
	runs := 0
	app := base.New()
	h := web.NewHandler(app, web.WithoutCsrf())
	h.Use(web.PageCache{Duration: time.Minute}.Middleware())
	h.Get("/gone", func(c *web.Context) error {
		runs++
		c.Response().Header().Set("Content-Type", "text/plain; charset=utf-8")
		c.Response().WriteHeader(http.StatusNotFound)
		_, err := c.Response().Write([]byte("nothing here"))
		return err
	})
	h.Get("/broken", func(c *web.Context) error {
		runs++
		return web.NotFound("broken page")
	})

	for _, target := range []string{"/gone", "/broken", "/gone", "/broken"} {
		res := req(t, h, http.MethodGet, target)
		if res.StatusCode != http.StatusNotFound {
			t.Fatalf("GET %s: status = %d, want 404", target, res.StatusCode)
		}
	}
	if runs != 4 {
		t.Errorf("actions ran %d times, want 4 (404 pages and failed actions stay uncached)", runs)
	}
	for _, key := range []string{"/gone", "/broken"} {
		if found, _ := app.Cache().Exists(context.Background(), key); found {
			t.Errorf("key %q must not land in the cache", key)
		}
	}
}

func TestPageCacheSkipsFlushedResponses(t *testing.T) {
	runs := 0
	app := base.New()
	h := web.NewHandler(app, web.WithoutCsrf())
	h.Use(web.PageCache{Duration: time.Minute}.Middleware())
	h.Get("/page", func(c *web.Context) error {
		runs++
		c.Response().Header().Set("Content-Type", "text/html; charset=utf-8")
		if _, err := c.Response().Write([]byte("<html>")); err != nil {
			return err
		}
		// Streaming past the buffering regime disqualifies the page.
		if err := http.NewResponseController(c.Response()).Flush(); err != nil {
			return err
		}
		_, err := c.Response().Write([]byte("<body>streamed</body></html>"))
		return err
	})

	for i := 0; i < 2; i++ {
		res := req(t, h, http.MethodGet, "/page")
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want the action served", res.StatusCode)
		}
	}
	if runs != 2 {
		t.Errorf("action ran %d times, want 2 (flushed responses stay uncached)", runs)
	}
}

func TestPageCacheBackendFailureServesUncached(t *testing.T) {
	version := "v1"
	runs := 0
	app := base.New()
	app.SetCache(&failingPageCache{runBuild: false})
	h := pageCacheApp(t, web.PageCache{Duration: time.Minute}, app, &version, &runs)

	res := req(t, h, http.MethodGet, "/page")
	if res.StatusCode != http.StatusOK || runs != 1 {
		t.Fatalf("status = %d, runs = %d, want the page served despite the cache backend failing", res.StatusCode, runs)
	}

	// A store failure after the render must not break the served page
	// either.
	app.SetCache(&failingPageCache{runBuild: true})
	runs = 0
	res = req(t, h, http.MethodGet, "/page")
	if res.StatusCode != http.StatusOK || runs != 1 {
		t.Fatalf("store-failure path: status = %d, runs = %d, want the page served", res.StatusCode, runs)
	}
}

func TestPageCacheMountedAsControllerMiddleware(t *testing.T) {
	version := "v1"
	runs := 0
	app := base.New()
	h := web.NewHandler(app, web.WithoutCsrf())
	ctrl := &cachedPageController{version: &version, runs: &runs}
	h.Ctrl(ctrl).Get("/cached", ctrl.ActionView)

	// The capture must ride the controller-middleware mount too: the
	// filter wraps the action inside withMiddleware, where the Context's
	// writer swap is the only way to see the action's output.
	if res := req(t, h, http.MethodGet, "/cached"); res.StatusCode != http.StatusOK {
		t.Fatalf("first render status = %d", res.StatusCode)
	}
	req(t, h, http.MethodGet, "/cached")
	if runs != 1 {
		t.Errorf("action ran %d times on the controller mount, want 1 (second request replays)", runs)
	}
}

// cachedPageController mounts the page cache the Yii2 way: as controller
// middleware around its own action.
type cachedPageController struct {
	web.Controller
	version *string
	runs    *int
}

func (c *cachedPageController) Middleware() []web.Middleware {
	return []web.Middleware{
		web.PageCache{Duration: time.Minute}.Middleware(),
	}
}

func (c *cachedPageController) ActionView(ctx *web.Context) error {
	*c.runs++
	ctx.Response().Header().Set("Content-Type", "text/html; charset=utf-8")
	_, err := ctx.Response().Write([]byte("<html><body>" + *c.version + "</body></html>"))
	return err
}

func TestPageCacheStandalonePassesThrough(t *testing.T) {
	served := false
	h := web.PageCache{Duration: time.Minute}.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		served = true
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("plain"))
	}))

	res := pageReq(t, h, http.MethodGet, "/anywhere", "", "")
	if !served || res.StatusCode != http.StatusOK {
		t.Fatalf("served = %v, status = %d, want a pass-through response", served, res.StatusCode)
	}
}

func TestCaptureWriterRecordsAndForwards(t *testing.T) {
	rec := httptest.NewRecorder()
	cw := web.NewCaptureWriter(rec)

	if cw.StatusCode() != http.StatusOK {
		t.Errorf("default status = %d, want 200", cw.StatusCode())
	}
	if got := cw.Unwrap(); got != rec {
		t.Error("Unwrap must expose the wrapped writer for http.ResponseController")
	}

	cw.Header().Set("Content-Type", "text/html; charset=utf-8")
	cw.WriteHeader(http.StatusOK)
	cw.WriteHeader(http.StatusInternalServerError) // duplicate: dropped
	_, err := cw.Write([]byte("<body>hi"))
	if err != nil {
		t.Fatalf("write: %v", err)
	}

	if cw.StatusCode() != http.StatusOK || len(cw.Body()) != len("<body>hi") {
		t.Errorf("status = %d, body = %q, want the first status and the full body", cw.StatusCode(), cw.Body())
	}
	if rec.Code != http.StatusOK || rec.Body.String() != "<body>hi" {
		t.Errorf("wrapped writer saw status %d body %q, want forwarded writes", rec.Code, rec.Body.String())
	}

	// Flush is observable and forwarded: the recorder records the flushed
	// state on its side (httptest marks it via Flushed on Result).
	if flushed, ok := any(cw).(http.Flusher); !ok {
		t.Error("CaptureWriter must satisfy http.Flusher directly")
	} else {
		flushed.Flush()
	}
	if !cw.Flushed() {
		t.Error("Flushed() must report the flush")
	}
	if err := http.NewResponseController(cw).Flush(); err != nil {
		t.Errorf("ResponseController.Flush through the wrapper: %v", err)
	}
}

func TestCaptureWriterRemembersWriteError(t *testing.T) {
	cw := web.NewCaptureWriter(failingWriter{})

	cw.WriteHeader(http.StatusOK)
	if _, err := cw.Write([]byte("doomed")); err == nil {
		t.Fatal("write error must propagate")
	}
	if cw.Err() == nil {
		t.Error("Err must remember the first write error")
	}
	if string(cw.Body()) != "doomed" {
		t.Errorf("captured body = %q, want the attempted bytes", cw.Body())
	}
}

// versionDependency is a test dependency reading a mutable version, the
// shape real applications use for dependencies that must observe live
// state (table versions, config revisions).
type versionDependency struct {
	version string
}

func (d *versionDependency) Capture(ctx context.Context) (string, error) { return d.version, nil }

func (d *versionDependency) Changed(ctx context.Context, fingerprint string) bool {
	return d.version != fingerprint
}

// recordingPageCache records the ttl and dependency of the GetOrSet calls
// passing through, then delegates to a real backend.
type recordingPageCache struct {
	cache.Cache
	ttl time.Duration
	dep cache.Dependency
}

func (r *recordingPageCache) GetOrSet(ctx context.Context, key string, dest any, build func(context.Context) (any, error), ttl time.Duration, dep cache.Dependency) error {
	r.ttl, r.dep = ttl, dep
	return r.Cache.GetOrSet(ctx, key, dest, build, ttl, dep)
}

// failingPageCache simulates backend trouble: runBuild decides whether the
// failure happens before the action (lookup) or after it (store).
type failingPageCache struct {
	runBuild bool
}

func (f *failingPageCache) Get(context.Context, string, any) (bool, error) {
	return false, errors.New("cache down")
}

func (f *failingPageCache) Set(context.Context, string, any, time.Duration) error {
	return errors.New("cache down")
}

func (f *failingPageCache) Delete(context.Context, string) error { return errors.New("cache down") }

func (f *failingPageCache) Exists(context.Context, string) (bool, error) {
	return false, errors.New("cache down")
}

func (f *failingPageCache) Flush(context.Context) error { return errors.New("cache down") }

func (f *failingPageCache) GetOrSet(ctx context.Context, key string, dest any, build func(context.Context) (any, error), ttl time.Duration, dep cache.Dependency) error {
	if f.runBuild {
		if _, err := build(ctx); err != nil {
			return err
		}
	}
	return errors.New("cache down")
}

// failingWriter rejects every write, standing in for a client that went
// away mid-response.
type failingWriter struct{}

func (failingWriter) Header() http.Header { return http.Header{} }

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("connection reset") }

func (failingWriter) WriteHeader(int) {}
