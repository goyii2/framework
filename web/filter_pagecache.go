package web

import (
	"bytes"
	"context"
	"errors"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/goyii2/framework/cache"
)

// CaptureWriter wraps an http.ResponseWriter and records the response while
// it flows through: the status code, the full body, whether the response was
// flushed, and the first client-side write error. Writes pass straight
// through to the wrapped writer, so the client is served live and the
// capture is a copy taken on the way. PageCache uses it to snapshot rendered
// pages; the debug toolbar reuses it to capture (and rewrite) response
// bodies.
type CaptureWriter struct {
	http.ResponseWriter

	status  int
	body    bytes.Buffer
	flushed bool
	err     error
	wrote   bool
}

// NewCaptureWriter wraps w and starts recording.
func NewCaptureWriter(w http.ResponseWriter) *CaptureWriter {
	return &CaptureWriter{ResponseWriter: w, status: http.StatusOK}
}

// WriteHeader records and forwards the status code. Duplicate calls are
// dropped — the first status line is the one the client sees.
func (cw *CaptureWriter) WriteHeader(code int) {
	if cw.wrote {
		return
	}
	cw.status, cw.wrote = code, true
	cw.ResponseWriter.WriteHeader(code)
}

// Write copies b into the capture and forwards it to the wrapped writer. A
// failing write (client gone away mid-response) is remembered and surfaces
// through Err; the capture keeps the bytes attempted so far.
func (cw *CaptureWriter) Write(b []byte) (int, error) {
	cw.wrote = true
	cw.body.Write(b) // bytes.Buffer writes never fail
	n, err := cw.ResponseWriter.Write(b)
	if err != nil && cw.err == nil {
		cw.err = err
	}
	return n, err
}

// Flush marks the response as streamed and flushes the wrapped writer when
// it supports it. A flushed response has left the buffering regime
// (progressive rendering, server-sent events), so a capture of it is
// inherently a prefix, which is why Flushed is part of the cacheability
// rules. Unlike the Context's writer this method is implemented directly
// (not only via Unwrap), so plain http.Flusher type assertions find it as
// well; when the wrapped writer lacks Flush support the call is a no-op
// marking rather than an error.
func (cw *CaptureWriter) Flush() {
	cw.flushed = true
	if f, ok := cw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap lets http.ResponseController reach the wrapped writer's optional
// interfaces (Hijacker, ...).
func (cw *CaptureWriter) Unwrap() http.ResponseWriter { return cw.ResponseWriter }

// StatusCode returns the status code written; 200 before anything was
// written.
func (cw *CaptureWriter) StatusCode() int { return cw.status }

// Body returns the captured body. The slice aliases the internal buffer;
// callers must not mutate it.
func (cw *CaptureWriter) Body() []byte { return cw.body.Bytes() }

// Flushed reports whether the response was flushed mid-stream.
func (cw *CaptureWriter) Flushed() bool { return cw.flushed }

// Err returns the first write error, nil when the whole response reached
// the wrapped writer.
func (cw *CaptureWriter) Err() error { return cw.err }

// PageCache caches whole pages: the first GET of a route renders through
// the action and the response — status, Content-Type, body — is stored in
// the application's cache component; later GETs replay the stored page
// without running the action. It corresponds to Yii2's
// yii\filters\PageCache, with storage and invalidation riding the
// cache.Cache contract (lifetime plus dependency) instead of a filter-local
// store.
//
// Only successful text pages take part: GET requests answering 200 with an
// explicit text/* Content-Type. Everything else passes straight through
// uncached — POST and other verbs (write actions never read the cache, so a
// replayed page can never answer a mutation), non-200 statuses, JSON and
// binary responses (no explicit Content-Type means net/http would sniff it
// at the socket, which a replay cannot reproduce), and flushed responses
// (streamed output is a prefix, not a page).
type PageCache struct {
	// Duration is how long a cached page stays live; it becomes the cache
	// entry's ttl. Zero falls back to the backend's default lifetime (see
	// the cache package).
	Duration time.Duration

	// Dependency invalidates entries when the data they were rendered from
	// changes; nil leaves the lifetime as the only bound. The dependency
	// must observe live state (a type whose Capture re-reads the world,
	// like cache.TimeDependency or a custom dependency querying the
	// database) — a dependency holding a fixed value captured once never
	// invalidates its own entries.
	Dependency cache.Dependency

	// VaryBy derives the request-dependent part of the cache key — a user
	// segment, a theme, a language. Requests differing in the VaryBy value
	// cache and replay separately; nil caches one page per route and
	// query.
	VaryBy func(*http.Request) string
}

// cachedPage is the value stored in the cache component: everything a
// replay needs, nothing more.
type cachedPage struct {
	Status      int    `json:"status"`
	ContentType string `json:"contentType"`
	Body        []byte `json:"body"`
}

// errPageNotCacheable aborts storing a rendered response that falls outside
// the cacheable set; the response is already committed, so the error only
// stops the cache write and is swallowed by the middleware.
var errPageNotCacheable = errors.New("web: page response not cacheable")

// Middleware returns the page caching as middleware.
func (f PageCache) Middleware() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Only GET participates. HEAD shares routes with GET but caching
			// it would poison the shared key with empty bodies (net/http
			// drops the body of HEAD responses), and replaying a body to a
			// HEAD client would be discarded anyway — so HEAD, like every
			// write verb, passes through untouched.
			if r.Method != http.MethodGet {
				next.ServeHTTP(w, r)
				return
			}
			c := FromContext(r.Context())
			if c == nil {
				// Reused outside the application pipeline there is no
				// application cache in reach; serve through uncached.
				next.ServeHTTP(w, r)
				return
			}
			if c.w.wrote {
				// A response already started (an outer filter committed a
				// body) cannot be captured coherently from byte zero.
				next.ServeHTTP(w, r)
				return
			}
			key := f.pageKey(r)

			// GetOrSet drives both directions: the hit path decodes the
			// stored page, the rebuild path renders the action through a
			// CaptureWriter and stores what came out. Rebuilds are not
			// deduplicated (see cache.Cache): concurrent misses may render
			// twice; every request still observes a consistent page.
			var page cachedPage
			rendered := false
			err := c.App().Cache().GetOrSet(r.Context(), key, &page, func(context.Context) (any, error) {
				rendered = true
				return f.render(c, next, r)
			}, f.Duration, f.Dependency)

			if rendered {
				// The response went out through the capture writer; a failed
				// store (backend down) or the not-cacheable marker changes
				// nothing the client saw.
				return
			}
			if err != nil {
				// The cache backend failed before the action ran (lookup or
				// dependency capture error): render uncached rather than
				// failing the page over cache trouble.
				next.ServeHTTP(w, r)
				return
			}
			// Hit: replay the stored page.
			if page.ContentType != "" {
				w.Header().Set("Content-Type", page.ContentType)
			}
			w.WriteHeader(page.Status)
			_, _ = w.Write(page.Body)
		})
	}
}

// render runs the action with the response captured: the Context's writer
// is swapped for one that records through a CaptureWriter, because actions
// write through the Context (c.w), not through the writer handed down the
// middleware chain — wrapping that writer alone would capture nothing. The
// swap is restored when the render ends; the unified error renderer runs
// later, outside the chain, and writes through the restored writer.
func (f PageCache) render(c *Context, next http.Handler, r *http.Request) (cachedPage, error) {
	prev := c.w
	cw := NewCaptureWriter(prev)
	c.w = &responseWriter{ResponseWriter: cw, status: http.StatusOK}
	defer func() { c.w = prev }()
	next.ServeHTTP(c.w, r)

	// An action that returned an error never seeds the cache, not even its
	// partial output — the unified error renderer decides what the client
	// sees once the chain unwinds.
	if c.err != nil {
		return cachedPage{}, errPageNotCacheable
	}
	if !pageCacheable(cw) {
		return cachedPage{}, errPageNotCacheable
	}
	return cachedPage{
		Status:      cw.StatusCode(),
		ContentType: cw.Header().Get("Content-Type"),
		Body:        cw.Body(),
	}, nil
}

// pageCacheable reports whether the captured response belongs in the page
// cache: a complete 200 whose media type is text/*.
func pageCacheable(cw *CaptureWriter) bool {
	return cw.StatusCode() == http.StatusOK &&
		cw.Err() == nil &&
		!cw.Flushed() &&
		strings.HasPrefix(normalizeMediaType(cw.Header().Get("Content-Type")), "text/")
}

// pageKey derives the cache key: the route path, the query string with its
// parameters sorted, and the VaryBy value.
func (f PageCache) pageKey(r *http.Request) string {
	key := r.URL.Path
	if q := normalizedQuery(r.URL.Query()); q != "" {
		key += "?" + q
	}
	if f.VaryBy != nil {
		key += "|" + f.VaryBy(r)
	}
	return key
}

// normalizedQuery renders q deterministically: parameter names and each
// name's values sorted, so "/list?sort=id&page=2" and
// "/list?page=2&sort=id" resolve to one entry.
func normalizedQuery(q url.Values) string {
	if len(q) == 0 {
		return ""
	}
	parts := make([]string, 0, len(q))
	for _, k := range slices.Sorted(maps.Keys(q)) {
		values := slices.Clone(q[k])
		slices.Sort(values)
		for _, v := range values {
			parts = append(parts, url.QueryEscape(k)+"="+url.QueryEscape(v))
		}
	}
	return strings.Join(parts, "&")
}
