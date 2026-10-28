package web

import (
	"net/http"
	"slices"
	"strings"
)

// AccessRule is one authorization decision. A rule applies to a request when
// the request's verb and path satisfy Verbs and Paths (empty means no
// constraint) and the requester holds one of Roles: the special role "?"
// matches guests only, "@" matches any authenticated requester, anything
// else matches a concrete role the requester holds.
type AccessRule struct {
	// Allow is the decision: true lets the request through, false rejects
	// it with 403.
	Allow bool

	// Roles lists the roles the rule applies to; empty applies to anyone.
	Roles []string

	// Verbs restricts the rule to requests carrying one of these HTTP
	// methods (compared case-insensitively); empty applies to every verb.
	Verbs []string

	// Paths restricts the rule to these exact request paths; empty applies
	// to every path.
	Paths []string
}

// AccessControl authorizes requests by roles: rules are evaluated in order
// and the first applicable rule's Allow decides; a request no rule matches
// is denied (403). This is the role-callback form — roles come from the
// current identity by default. RoleResolver replaces that source wholesale
// (an RBAC manager plugs in there without call sites or rules changing).
type AccessControl struct {
	// Rules are evaluated in order; the first applicable rule decides.
	Rules []AccessRule

	// RoleResolver returns the roles the current requester holds; nil uses
	// the default: ["?"] for guests, ["@"] plus the identity's own roles
	// for authenticated requests.
	RoleResolver func(c *Context) []string
}

// Middleware returns the access check as middleware.
func (f AccessControl) Middleware() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c := FromContext(r.Context())
			if c == nil {
				// No request Context means no identity to authorize; denying
				// is the only safe default outside the pipeline.
				http.Error(w, "web: access control requires the application handler", http.StatusForbidden)
				return
			}
			roles := f.requesterRoles(c)
			for _, rule := range f.Rules {
				if !ruleMatches(rule, r, roles) {
					continue
				}
				if rule.Allow {
					next.ServeHTTP(w, r)
					return
				}
				writeError(w, r, Forbidden("you are not allowed to perform this action"))
				return
			}
			// No rule applied: default deny.
			writeError(w, r, Forbidden("you are not allowed to perform this action"))
		})
	}
}

// requesterRoles resolves the roles the current requester holds.
func (f AccessControl) requesterRoles(c *Context) []string {
	if f.RoleResolver != nil {
		return f.RoleResolver(c)
	}
	user := c.User()
	if user == nil || user.IsGuest() {
		return []string{"?"}
	}
	roles := append([]string{"@"}, user.Identity().GetRoles()...)
	slices.Sort(roles)
	return roles
}

// ruleMatches reports whether the rule applies to the request: verb and
// path unconstrained or satisfied, and one of its roles held.
func ruleMatches(rule AccessRule, r *http.Request, roles []string) bool {
	if len(rule.Verbs) > 0 && !slices.ContainsFunc(rule.Verbs, func(v string) bool {
		return strings.EqualFold(v, r.Method)
	}) {
		return false
	}
	if len(rule.Paths) > 0 && !slices.Contains(rule.Paths, r.URL.Path) {
		return false
	}
	if len(rule.Roles) == 0 {
		return true
	}
	return slices.ContainsFunc(rule.Roles, func(role string) bool {
		return slices.Contains(roles, role)
	})
}
