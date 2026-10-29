package vite

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
)

// DevProxy returns a middleware that forwards requests under the given path
// prefixes to the Vite dev server, so pages served by the Go app load their
// dev assets without CORS setup. Mount it only in development — it points
// at the dev server and nothing else:
//
//	if v.IsDev() {
//		handler.Use(v.DevProxy("/assets"))
//	}
//
// Prefixes match whole path segments: "/assets" covers "/assets" and
// "/assets/main.js" but not "/assetsfoo". No prefix (or "/") forwards
// everything.
//
// WebSocket upgrades pass through, which HMR needs: the stdlib proxy
// re-attaches the Upgrade/Connection headers on the outbound request and,
// on the dev server's 101 switch, hijacks both connections and pipes them
// bidirectionally. Responses stream unbuffered (negative FlushInterval), so
// hot updates are not delayed by proxy buffering.
//
// The returned function's shape is net/http middleware; it is assignable to
// web.Middleware, so it plugs into handler.Use and web.Chain unchanged.
func (v *Vite) DevProxy(prefixes ...string) func(http.Handler) http.Handler {
	target := v.devTarget()
	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(target)
			pr.SetXForwarded()
		},
		// Flush every chunk immediately: HMR pushes and dev-server
		// responses must not wait for proxy buffering.
		FlushInterval: -1,
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, err error) {
			w.WriteHeader(http.StatusBadGateway)
			fmt.Fprintf(w, "vite: dev server %s unreachable: %v", target.String(), err)
		},
	}
	prefixes = normalizePrefixes(prefixes)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if matchPrefix(r.URL.Path, prefixes) {
				proxy.ServeHTTP(w, r)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// devTarget parses DevServer into the URL the proxy forwards to, supplying
// an http scheme when only host:port was configured and falling back to the
// default dev server when nothing parses.
func (v *Vite) devTarget() *url.URL {
	origin := v.devOrigin()
	if !strings.Contains(origin, "://") {
		origin = "http://" + origin
	}
	target, err := url.Parse(origin)
	if err != nil || target.Host == "" {
		return &url.URL{Scheme: "http", Host: "localhost:5173"}
	}
	return target
}

// normalizePrefixes trims each prefix and drops its trailing slash; an
// empty result list forwards everything (matchPrefix treats it that way).
func normalizePrefixes(prefixes []string) []string {
	out := make([]string, 0, len(prefixes))
	for _, p := range prefixes {
		p = strings.TrimSuffix(strings.TrimSpace(p), "/")
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	return out
}

// matchPrefix reports whether path falls under one of the prefixes; an
// empty list matches every path.
func matchPrefix(path string, prefixes []string) bool {
	if len(prefixes) == 0 {
		return true
	}
	for _, p := range prefixes {
		if path == p || strings.HasPrefix(path, p+"/") {
			return true
		}
	}
	return false
}
