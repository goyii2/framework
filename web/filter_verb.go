package web

import (
	"net/http"
	"slices"
	"strings"
)

// VerbRule restricts one request path (exact match, query strings ignored)
// to a set of verbs.
type VerbRule struct {
	Path  string
	Verbs []string
}

// VerbFilter enforces a verb whitelist: a request whose method is not
// allowed is rejected with 405 and an Allow header listing the accepted
// verbs (sorted, comma-separated). The whitelist is strict — every accepted
// verb must be listed, including HEAD.
//
// With Rules set, only the listed paths are filtered and requests to other
// paths pass through unfiltered; without Rules, Allowed applies to every
// request the middleware wraps.
type VerbFilter struct {
	// Allowed lists the verbs accepted wherever this filter applies.
	Allowed []string
	// Rules scopes the filter per path; a path without a rule passes.
	Rules []VerbRule
}

// Middleware returns the verb check as middleware.
func (f VerbFilter) Middleware() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			allowed := f.allowedFor(r.URL.Path)
			if len(allowed) == 0 || slices.Contains(allowed, strings.ToUpper(r.Method)) {
				next.ServeHTTP(w, r)
				return
			}
			slices.Sort(allowed)
			w.Header().Set("Allow", strings.Join(uniq(allowed), ", "))
			writeError(w, r, MethodNotAllowed("method not allowed"))
		})
	}
}

// allowedFor resolves the whitelist for path: the first matching rule, or
// Allowed when no rules exist. An empty result means unfiltered.
func (f VerbFilter) allowedFor(path string) []string {
	if len(f.Rules) == 0 {
		return normalizeVerbs(f.Allowed)
	}
	for _, rule := range f.Rules {
		if rule.Path == path {
			return normalizeVerbs(rule.Verbs)
		}
	}
	return nil
}

// normalizeVerbs uppercases verbs; verbs are case-insensitive in HTTP.
func normalizeVerbs(verbs []string) []string {
	out := make([]string, len(verbs))
	for i, v := range verbs {
		out[i] = strings.ToUpper(v)
	}
	return out
}
