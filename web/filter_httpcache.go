package web

import (
	"net/http"
	"strings"
	"time"
)

// HttpCache adds conditional-request caching: it derives validators (an
// ETag and/or a Last-Modified time) from the request before the action
// runs and, when the client already holds a fresh copy (If-None-Match /
// If-Modified-Since), answers 304 without running the action at all.
// Per RFC 9110, If-None-Match takes precedence: when it is present and
// does not match, If-Modified-Since is not consulted.
type HttpCache struct {
	// Etag derives the response entity tag from the request; empty skips
	// the ETag validator. Values are quoted automatically ("weak" W/
	// prefixes are preserved as given).
	Etag func(r *http.Request) string

	// LastModified reports the resource's modification time; the zero time
	// skips the validator. Times are truncated to whole seconds, the
	// granularity HTTP dates carry.
	LastModified func(r *http.Request) time.Time

	// CacheControl is written verbatim into the Cache-Control response
	// header (e.g. "public, max-age=3600"); empty omits the header.
	CacheControl string
}

// Middleware returns the conditional caching as middleware.
func (f HttpCache) Middleware() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			etag := ""
			if f.Etag != nil {
				etag = quoteETag(f.Etag(r))
			}
			var lastModified time.Time
			if f.LastModified != nil {
				lastModified = f.LastModified(r).UTC().Truncate(time.Second)
			}

			notModified := (etag != "" && ifNoneMatchSatisfied(r.Header.Get("If-None-Match"), etag)) ||
				(etag == "" && !lastModified.IsZero() && ifModifiedSinceSatisfied(r.Header.Get("If-Modified-Since"), lastModified))
			if notModified {
				f.setValidators(w.Header(), etag, lastModified)
				w.WriteHeader(http.StatusNotModified)
				return
			}

			f.setValidators(w.Header(), etag, lastModified)
			next.ServeHTTP(w, r)
		})
	}
}

// setValidators writes the available validators and the cache policy.
func (f HttpCache) setValidators(hdr http.Header, etag string, lastModified time.Time) {
	if etag != "" {
		hdr.Set("Etag", etag)
	}
	if !lastModified.IsZero() {
		hdr.Set("Last-Modified", lastModified.Format(http.TimeFormat))
	}
	if f.CacheControl != "" {
		hdr.Set("Cache-Control", f.CacheControl)
	}
}

// ifNoneMatchSatisfied compares the client's If-None-Match list with the
// response etag. Entries are matched exactly after normalizing weak
// prefixes; "*" matches any etag.
func ifNoneMatchSatisfied(header, etag string) bool {
	if header == "" {
		return false
	}
	for _, entry := range strings.Split(header, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "*" {
			return true
		}
		if strings.HasPrefix(entry, "W/") || strings.HasPrefix(entry, "w/") {
			entry = entry[2:]
		}
		if entry == etag {
			return true
		}
	}
	return false
}

// ifModifiedSinceSatisfied compares the client's If-Modified-Since date
// with the resource time: satisfied when the resource has not changed
// since that moment. Unparseable dates simply miss.
func ifModifiedSinceSatisfied(header string, lastModified time.Time) bool {
	if header == "" {
		return false
	}
	since, err := http.ParseTime(header)
	if err != nil {
		return false
	}
	return !lastModified.After(since)
}

// quoteETag wraps a bare etag value in double quotes, leaving already
// quoted or weak-tagged values untouched.
func quoteETag(etag string) string {
	if etag == "" {
		return ""
	}
	if strings.HasPrefix(etag, "W/") || strings.HasPrefix(etag, "w/") ||
		(strings.HasPrefix(etag, "\"") && strings.HasSuffix(etag, "\"")) {
		return etag
	}
	return "\"" + etag + "\""
}
