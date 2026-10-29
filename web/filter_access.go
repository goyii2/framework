package web

import (
	"fmt"
	"net/http"
	"slices"
	"strings"
)

// AccessRule is one authorization decision. A rule applies to a request when
// the request's verb and path satisfy Verbs and Paths (empty means no
// constraint) and the requester holds one of Roles: the special role "?"
// matches guests only, "@" matches any authenticated requester, anything
// else matches a concrete role the requester holds — through the installed
// AccessChecker when AccessControl carries one, through the identity's own
// roles otherwise.
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
// is denied (403). Roles come from one of two backends: an AccessChecker
// (Auth, typically the rbac manager installed with SetAuthManager) checks
// each role against the authorization hierarchy; without one, roles resolve
// from the current identity by default, and RoleResolver replaces that
// source wholesale. Set Auth, RoleResolver or neither — Auth wins for
// concrete role names while "?" and "@" keep their guest/authenticated
// meaning either way.
type AccessControl struct {
	// Rules are evaluated in order; the first applicable rule decides.
	Rules []AccessRule

	// Auth checks role membership through an authorization backend (the
	// rbac manager); nil uses the role resolution below.
	Auth AccessChecker

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
			for _, rule := range f.Rules {
				if !ruleApplies(rule, r) {
					continue
				}
				held, err := f.anyRoleHeld(rule.Roles, c)
				if err != nil {
					// The backend failed mid-check: report the failure as
					// what it is (a server fault) instead of a denial that
					// would blame the requester.
					c.App().Log().Error("access check failed", "method", r.Method, "path", r.URL.Path, "err", err.Error())
					c.SetError(ServerError("authorization check failed"))
					return
				}
				if len(rule.Roles) > 0 && !held {
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

// requesterRoles resolves the roles the current requester holds through the
// callback path (the identity's roles, or the RoleResolver replacing it).
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

// anyRoleHeld reports whether the requester holds any of roles. "?" and "@"
// are decided from the authentication state alone; concrete roles go to the
// AccessChecker when one is set, otherwise to the callback path's role
// list. An error means the backend check could not run.
func (f AccessControl) anyRoleHeld(roles []string, c *Context) (bool, error) {
	if len(roles) == 0 {
		return true, nil
	}
	user := c.User()
	if user == nil || user.IsGuest() {
		// Guests hold the guest role and nothing else.
		return slices.Contains(roles, "?"), nil
	}
	if slices.Contains(roles, "@") {
		return true, nil
	}
	held := f.requesterRoles(c) // callback path; ignored when Auth is set
	for _, role := range roles {
		if role == "?" {
			continue
		}
		if f.Auth != nil {
			ok, err := f.Auth.CheckAccess(user.Identity().GetID(), role, nil)
			if err != nil {
				return false, fmt.Errorf("access check for %q: %w", role, err)
			}
			if ok {
				return true, nil
			}
			continue
		}
		if slices.Contains(held, role) {
			return true, nil
		}
	}
	return false, nil
}

// ruleApplies reports whether the rule's verb and path constraints cover the
// request (empty constraints cover everything).
func ruleApplies(rule AccessRule, r *http.Request) bool {
	if len(rule.Verbs) > 0 && !slices.ContainsFunc(rule.Verbs, func(v string) bool {
		return strings.EqualFold(v, r.Method)
	}) {
		return false
	}
	if len(rule.Paths) > 0 && !slices.Contains(rule.Paths, r.URL.Path) {
		return false
	}
	return true
}
