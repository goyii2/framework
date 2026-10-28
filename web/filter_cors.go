package web

import (
	"net/http"
	"slices"
	"strconv"
	"strings"
)

// Cors answers cross-origin requests following the CORS protocol. A
// preflight (OPTIONS with an Access-Control-Request-Method header) from an
// allowed origin is answered 204 with the access-control headers and never
// reaches the action; a preflight the policy rejects is answered 403. A
// simple cross-origin request from an allowed origin passes through with
// the access-control response headers attached; from a disallowed origin it
// passes through without any (the browser's same-origin default applies),
// because unlike a preflight it is an ordinary request the server may still
// legitimately serve.
//
// Requests without an Origin header carry no CORS semantics and pass
// through untouched.
type Cors struct {
	// Origins lists the allowed Origin header values. An empty list, or an
	// entry "*", allows every origin. With Credentials on, "*" is not
	// usable per the spec, so the actual Origin is echoed instead.
	Origins []string

	// Methods lists the verbs allowed in preflight requests. Empty allows
	// any requested verb (the preflight echoes it back).
	Methods []string

	// Headers lists the request headers allowed in preflight requests.
	// Empty allows any requested header (echoed back).
	Headers []string

	// ExposeHeaders lists response headers the client may read
	// cross-origin; empty omits the header.
	ExposeHeaders []string

	// Credentials adds Access-Control-Allow-Credentials: true so cookies
	// and HTTP authentication travel cross-origin.
	Credentials bool

	// MaxAge is the number of seconds clients may cache the preflight
	// result; 0 omits Access-Control-Max-Age.
	MaxAge int
}

// Middleware returns the CORS handling as middleware.
func (f Cors) Middleware() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin == "" {
				next.ServeHTTP(w, r)
				return
			}
			preflight := r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != ""

			if preflight {
				// A preflight the policy rejects carries no CORS headers at
				// all, so the browser reads it as a plain failure.
				hdr := w.Header()
				hdr.Add("Vary", "Origin")
				hdr.Add("Vary", "Access-Control-Request-Method")
				hdr.Add("Vary", "Access-Control-Request-Headers")
				if !f.originAllowed(origin) {
					writeError(w, r, Forbidden("origin not allowed by the CORS policy"))
					return
				}
				if !f.methodAllowed(r.Header.Get("Access-Control-Request-Method")) ||
					!f.headersAllowed(r.Header.Get("Access-Control-Request-Headers")) {
					writeError(w, r, Forbidden("preflight request rejected by the CORS policy"))
					return
				}
				f.setSimpleHeaders(hdr, origin)
				hdr.Set("Access-Control-Allow-Methods", f.allowMethodsValue(r))
				hdr.Set("Access-Control-Allow-Headers", f.allowHeadersValue(r))
				if f.MaxAge > 0 {
					hdr.Set("Access-Control-Max-Age", strconv.Itoa(f.MaxAge))
				}
				w.WriteHeader(http.StatusNoContent)
				return
			}

			if !f.originAllowed(origin) {
				// Ordinary request from a foreign origin: serve it, but
				// without any access-control headers the browser keeps it
				// unreadable cross-origin.
				next.ServeHTTP(w, r)
				return
			}
			f.setSimpleHeaders(w.Header(), origin)
			next.ServeHTTP(w, r)
		})
	}
}

// setSimpleHeaders writes the access-control headers shared by preflight
// and ordinary cross-origin responses.
func (f Cors) setSimpleHeaders(hdr http.Header, origin string) {
	hdr.Add("Vary", "Origin")
	if f.Credentials {
		hdr.Set("Access-Control-Allow-Credentials", "true")
		hdr.Set("Access-Control-Allow-Origin", origin)
		return
	}
	hdr.Set("Access-Control-Allow-Origin", f.allowOriginValue(origin))
	if len(f.ExposeHeaders) > 0 {
		hdr.Set("Access-Control-Expose-Headers", strings.Join(f.ExposeHeaders, ", "))
	}
}

// originAllowed reports whether origin may make cross-origin requests: an
// empty list or a "*" entry allows any, otherwise exact (case-insensitive)
// membership.
func (f Cors) originAllowed(origin string) bool {
	if len(f.Origins) == 0 {
		return true
	}
	return slices.ContainsFunc(f.Origins, func(o string) bool {
		return o == "*" || strings.EqualFold(o, origin)
	})
}

// allowOriginValue picks the Access-Control-Allow-Origin value: "*" when
// the policy is wildcard, the caller's origin otherwise.
func (f Cors) allowOriginValue(origin string) string {
	if slices.Contains(f.Origins, "*") || len(f.Origins) == 0 {
		return "*"
	}
	return origin
}

// methodAllowed reports whether a preflight's requested verb passes: any
// verb when Methods is empty, membership otherwise.
func (f Cors) methodAllowed(method string) bool {
	if len(f.Methods) == 0 {
		return true
	}
	return slices.ContainsFunc(f.Methods, func(m string) bool {
		return strings.EqualFold(m, method)
	})
}

// headersAllowed reports whether every header a preflight asks to send is
// allowed: any header when Headers is empty, membership (case-insensitive,
// comma-separated list) otherwise.
func (f Cors) headersAllowed(requested string) bool {
	if strings.TrimSpace(requested) == "" || len(f.Headers) == 0 {
		return true
	}
	for _, h := range strings.Split(requested, ",") {
		h = strings.TrimSpace(h)
		if h == "" {
			continue
		}
		if !slices.ContainsFunc(f.Headers, func(a string) bool {
			return strings.EqualFold(a, h)
		}) {
			return false
		}
	}
	return true
}

// allowMethodsValue answers the preflight's Access-Control-Allow-Methods:
// the configured list verbatim, or the requested verb when unrestricted.
func (f Cors) allowMethodsValue(r *http.Request) string {
	if len(f.Methods) == 0 {
		return r.Header.Get("Access-Control-Request-Method")
	}
	return strings.Join(f.Methods, ", ")
}

// allowHeadersValue answers the preflight's Access-Control-Allow-Headers:
// the configured list verbatim, or the requested headers when unrestricted.
func (f Cors) allowHeadersValue(r *http.Request) string {
	if len(f.Headers) == 0 {
		return r.Header.Get("Access-Control-Request-Headers")
	}
	return strings.Join(f.Headers, ", ")
}
