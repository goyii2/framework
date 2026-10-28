package web

import (
	"net/http"
	"slices"
	"strconv"
	"strings"
)

// Format pairs a media type with the response format name negotiated for
// it ("application/json" -> "json", "text/html" -> "html", ...). Format
// names are free-form; the unified error renderer understands "html" and
// renders JSON for anything else.
type Format struct {
	Type   string
	Format string
}

// ContentNegotiator agrees with the client how responses are shaped and
// which request bodies are readable, in both directions of one exchange:
//
//   - Response side: the Accept header is matched against ResponseFormats
//     (in order, honoring q-values; wildcards accept the earliest offered
//     format). The winner is published on the Context as its
//     ResponseFormat and drives unified error rendering; an Accept header
//     matching none of them is rejected 406.
//
//   - Request side: when RequestFormats is non-empty, a request carrying a
//     body method (POST/PUT/PATCH/DELETE) with a Content-Type absent from
//     it is rejected 415. A request without a Content-Type passes — it
//     carries nothing to parse.
type ContentNegotiator struct {
	// ResponseFormats lists acceptable response media types in preference
	// order; empty means {"application/json": json, "text/html": html}.
	ResponseFormats []Format

	// RequestFormats lists the request body media types the endpoint reads;
	// empty disables the request-side check.
	RequestFormats []Format
}

// Middleware returns the negotiation as middleware.
func (f ContentNegotiator) Middleware() Middleware {
	response := f.ResponseFormats
	if len(response) == 0 {
		response = []Format{{Type: "application/json", Format: "json"}, {Type: "text/html", Format: "html"}}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			types := make([]string, len(response))
			byType := make(map[string]string, len(response))
			for i, fmt := range response {
				types[i] = normalizeMediaType(fmt.Type)
				byType[types[i]] = fmt.Format
			}
			w.Header().Add("Vary", "Accept")

			media, ok := negotiateMediaType(r.Header.Get("Accept"), types)
			if !ok {
				writeError(w, r, NotAcceptable("none of the acceptable content types is supported"))
				return
			}
			if c := FromContext(r.Context()); c != nil {
				c.format = byType[media]
			}

			if !f.requestTypeAllowed(r) {
				writeError(w, r, UnsupportedMediaType("content type is not supported"))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// requestTypeAllowed checks the request's Content-Type against
// RequestFormats for body-carrying methods; requests without a Content-Type
// or with an empty RequestFormats pass.
func (f ContentNegotiator) requestTypeAllowed(r *http.Request) bool {
	if len(f.RequestFormats) == 0 {
		return true
	}
	if !slices.Contains([]string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete}, r.Method) {
		return true
	}
	contentType := r.Header.Get("Content-Type")
	if strings.TrimSpace(contentType) == "" {
		return true
	}
	media := normalizeMediaType(contentType)
	return slices.ContainsFunc(f.RequestFormats, func(fmt Format) bool {
		return normalizeMediaType(fmt.Type) == media
	})
}

// normalizeMediaType lowercases a media type and strips parameters
// ("text/html; charset=utf-8" -> "text/html").
func normalizeMediaType(v string) string {
	media, _, _ := strings.Cut(strings.TrimSpace(strings.ToLower(v)), ";")
	return strings.TrimSpace(media)
}

// negotiateMediaType picks the offered media type best matching an Accept
// header. Ranges are examined in quality order with list position breaking
// ties; a wildcard range accepts the earliest offered type. An empty or
// malformed header expresses no preference and accepts the first offered
// type. No acceptable match reports false.
func negotiateMediaType(accept string, offered []string) (string, bool) {
	if len(offered) == 0 {
		return "", false
	}
	if accept == "" {
		return offered[0], true
	}

	type mediaRange struct {
		typ, sub string
		q        float64
	}
	var ranges []mediaRange
	for _, part := range strings.Split(accept, ",") {
		media, params, _ := strings.Cut(strings.TrimSpace(part), ";")
		typ, sub, ok := strings.Cut(media, "/")
		mr := mediaRange{typ: strings.TrimSpace(typ), sub: strings.TrimSpace(sub), q: 1}
		for params != "" {
			var kv string
			kv, params, _ = strings.Cut(params, ";")
			k, v, _ := strings.Cut(strings.TrimSpace(kv), "=")
			if strings.EqualFold(strings.TrimSpace(k), "q") {
				if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
					mr.q = f
				}
			}
		}
		if !ok || mr.typ == "" || mr.sub == "" {
			continue
		}
		ranges = append(ranges, mr)
	}
	if len(ranges) == 0 {
		return offered[0], true
	}
	slices.SortStableFunc(ranges, func(a, b mediaRange) int {
		if a.q > b.q {
			return -1
		}
		if a.q < b.q {
			return 1
		}
		return 0
	})
	for _, mr := range ranges {
		for _, o := range offered {
			typ, sub, _ := strings.Cut(o, "/")
			if (mr.typ == "*" || mr.typ == typ) && (mr.sub == "*" || mr.sub == sub) {
				return o, true
			}
		}
	}
	return "", false
}
