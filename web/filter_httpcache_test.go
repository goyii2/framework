package web_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/goyii2/framework/base"
	"github.com/goyii2/framework/web"
)

// modifiedAt is the fixed modification time the test resource reports.
var modifiedAt = time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)

// httpCacheApp mounts the cache filter over an action recording it ran.
func httpCacheApp(t *testing.T, cache web.HttpCache, actionRan *bool) *web.Handler {
	t.Helper()
	app := base.New()
	h := web.NewHandler(app, web.WithoutCsrf())
	h.Use(cache.Middleware())
	h.Get("/report", func(c *web.Context) error {
		*actionRan = true
		return c.JSON(map[string]string{"report": "contents"})
	})
	return h
}

// httpCacheFilter derives both validators from the request path.
func httpCacheFilter() web.HttpCache {
	return web.HttpCache{
		Etag:         func(*http.Request) string { return "v1" },
		LastModified: func(*http.Request) time.Time { return modifiedAt },
		CacheControl: "public, max-age=3600",
	}
}

func TestHttpCacheSetsValidatorsOnFreshFetch(t *testing.T) {
	var ran bool
	h := httpCacheApp(t, httpCacheFilter(), &ran)

	res := req(t, h, http.MethodGet, "/report")
	if res.StatusCode != http.StatusOK || !ran {
		t.Fatalf("status = %d, ran = %v, want a served response", res.StatusCode, ran)
	}
	for header, want := range map[string]string{
		"Etag":          `"v1"`,
		"Last-Modified": modifiedAt.Format(http.TimeFormat),
		"Cache-Control": "public, max-age=3600",
	} {
		if got := res.Header.Get(header); got != want {
			t.Errorf("%s = %q, want %q", header, got, want)
		}
	}
}

func TestHttpCacheIfNoneMatchAnswers304WithoutAction(t *testing.T) {
	var ran bool
	h := httpCacheApp(t, httpCacheFilter(), &ran)

	r := httptest.NewRequest(http.MethodGet, "/report", nil)
	r.Header.Set("If-None-Match", `"v1"`)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	res := rec.Result()

	if res.StatusCode != http.StatusNotModified {
		t.Fatalf("status = %d, want 304", res.StatusCode)
	}
	if ran {
		t.Error("action ran although the client's copy was fresh")
	}
	if raw, _ := io.ReadAll(res.Body); len(raw) != 0 {
		t.Errorf("304 body = %q, want empty", raw)
	}
	if got := res.Header.Get("Etag"); got != `"v1"` {
		t.Errorf("304 Etag = %q, want the validator echoed", got)
	}
}

func TestHttpCacheWeakIfNoneMatchStillMatches(t *testing.T) {
	var ran bool
	h := httpCacheApp(t, httpCacheFilter(), &ran)

	r := httptest.NewRequest(http.MethodGet, "/report", nil)
	r.Header.Set("If-None-Match", `W/"v1"`)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)

	if rec.Code != http.StatusNotModified {
		t.Fatalf("status = %d, want 304 (weak tags compare equal)", rec.Code)
	}
}

func TestHttpCacheIfNoneMatchTakesPrecedenceOverIfModifiedSince(t *testing.T) {
	var ran bool
	h := httpCacheApp(t, httpCacheFilter(), &ran)

	// The etag does not match but If-Modified-Since would satisfy: per
	// RFC 9110 the mismatching If-None-Match wins and the resource is
	// re-served.
	r := httptest.NewRequest(http.MethodGet, "/report", nil)
	r.Header.Set("If-None-Match", `"v999"`)
	r.Header.Set("If-Modified-Since", modifiedAt.Format(http.TimeFormat))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)

	if rec.Code != http.StatusOK || !ran {
		t.Fatalf("status = %d, ran = %v, want 200 with the action served", rec.Code, ran)
	}
}

func TestHttpCacheIfModifiedSinceAnswers304(t *testing.T) {
	var ran bool
	h := httpCacheApp(t, web.HttpCache{
		LastModified: func(*http.Request) time.Time { return modifiedAt },
		CacheControl: "no-cache",
	}, &ran)

	since := modifiedAt.Format(http.TimeFormat)
	r := httptest.NewRequest(http.MethodGet, "/report", nil)
	r.Header.Set("If-Modified-Since", since)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	res := rec.Result()

	if res.StatusCode != http.StatusNotModified || ran {
		t.Fatalf("status = %d, ran = %v, want 304 without the action", res.StatusCode, ran)
	}
	if got := res.Header.Get("Cache-Control"); got != "no-cache" {
		t.Errorf("304 Cache-Control = %q, want the policy on 304 responses too", got)
	}

	// A client holding an older copy gets the full response.
	older := modifiedAt.Add(-time.Hour).Format(http.TimeFormat)
	r = httptest.NewRequest(http.MethodGet, "/report", nil)
	r.Header.Set("If-Modified-Since", older)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 for an outdated copy", rec.Code)
	}

	// Unparseable conditional headers are ignored, not honored.
	r = httptest.NewRequest(http.MethodGet, "/report", nil)
	r.Header.Set("If-Modified-Since", "not a date")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 for an unparseable date", rec.Code)
	}
}

func TestHttpCacheDynamicEtagPerRequest(t *testing.T) {
	// The etag varies with the request (per-path content versions), proving
	// the filter derives validators per request rather than once at boot.
	var ran bool
	h := httpCacheApp(t, web.HttpCache{
		Etag: func(r *http.Request) string { return r.URL.Path + "-tag" },
	}, &ran)

	r := httptest.NewRequest(http.MethodGet, "/report", nil)
	r.Header.Set("If-None-Match", `"/report-tag"`)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	if rec.Code != http.StatusNotModified {
		t.Fatalf("status = %d, want 304 for the matching derived etag", rec.Code)
	}
}
