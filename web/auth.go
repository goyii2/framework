package web

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

// HttpBasic authenticates requests through the Authorization basic scheme:
// credentials arrive in the header and are resolved by the injected
// Authenticate function — the seam applications fill with their own lookup
// (user table, LDAP, ...). Credential comparisons inside that function
// should use ConstantTimeEqual (or crypto/subtle directly) to avoid leaking
// positions of mismatch; the middleware itself compares nothing.
//
// Unauthenticated requests are rejected with 401 and a WWW-Authenticate
// challenge naming the realm, per the basic scheme. A verified identity is
// bound to the request User as a transient identity: visible to actions and
// access control, stored in no session.
type HttpBasic struct {
	// Realm names the protection space in the WWW-Authenticate challenge;
	// default "api".
	Realm string

	// Authenticate resolves a username/password pair to an identity; ok is
	// false for unknown users or wrong passwords. Without it every request
	// is rejected: the middleware never trusts credentials it cannot check.
	Authenticate func(username, password string) (Identity, bool)
}

// Middleware returns the basic authentication as middleware.
func (f HttpBasic) Middleware() Middleware {
	realm := defaultString(f.Realm, "api")
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			username, password, ok := r.BasicAuth()
			var identity Identity
			if ok && f.Authenticate != nil {
				identity, ok = f.Authenticate(username, password)
			} else {
				ok = false
			}
			if !ok {
				w.Header().Set("WWW-Authenticate", `Basic realm="`+escapeRealm(realm)+`", charset="UTF-8"`)
				writeError(w, r, Unauthorized("authentication required"))
				return
			}
			if c := FromContext(r.Context()); c != nil {
				c.ensureUser().setTransient(identity)
			}
			next.ServeHTTP(w, r)
		})
	}
}

// HttpBearer authenticates requests through the Authorization bearer scheme:
// the token after the scheme keyword is resolved by the injected
// Authenticate function. StaticBearerTokens provides the framework-side
// implementation for a fixed token table; applications with dynamic tokens
// (database-issued, JWT, ...) inject their own resolver instead.
//
// Unauthenticated requests are rejected with 401 and a WWW-Authenticate
// bearer challenge. As with HttpBasic, a verified identity is bound to the
// request User transiently.
type HttpBearer struct {
	// Realm names the protection space in the WWW-Authenticate challenge;
	// default "api".
	Realm string

	// Authenticate resolves a bearer token to an identity; ok is false for
	// unknown or malformed tokens. Without it every request is rejected.
	Authenticate func(token string) (Identity, bool)
}

// Middleware returns the bearer authentication as middleware.
func (f HttpBearer) Middleware() Middleware {
	realm := defaultString(f.Realm, "api")
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, ok := bearerToken(r)
			var identity Identity
			if ok && f.Authenticate != nil {
				identity, ok = f.Authenticate(token)
			} else {
				ok = false
			}
			if !ok {
				w.Header().Set("WWW-Authenticate", `Bearer realm="`+escapeRealm(realm)+`"`)
				writeError(w, r, Unauthorized("authentication required"))
				return
			}
			if c := FromContext(r.Context()); c != nil {
				c.ensureUser().setTransient(identity)
			}
			next.ServeHTTP(w, r)
		})
	}
}

// StaticBearerTokens builds a bearer Authenticate from a static token ->
// identity table. Every presented token is compared against every stored
// token in constant time with no early exit, so the timing of a request
// reveals neither which entry matched nor whether the token was close to
// one. Lookup cost grows with the table, never with the token's position.
func StaticBearerTokens(tokens map[string]Identity) func(token string) (Identity, bool) {
	type entry struct {
		token    string
		identity Identity
	}
	entries := make([]entry, 0, len(tokens))
	for token, identity := range tokens {
		entries = append(entries, entry{token: token, identity: identity})
	}
	return func(presented string) (Identity, bool) {
		match := -1
		for i, e := range entries {
			if subtle.ConstantTimeCompare([]byte(presented), []byte(e.token)) == 1 {
				match = i
			}
		}
		if match < 0 {
			return nil, false
		}
		return entries[match].identity, true
	}
}

// ConstantTimeEqual compares two secrets in constant time: strings of equal
// length are compared without leaking mismatch positions, anything else
// (including two empty strings — no secret at all) compares unequal. It is
// the comparison to reach for inside custom Authenticate functions.
func ConstantTimeEqual(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// bearerToken extracts the token from a bearer Authorization header. The
// scheme keyword is matched case-insensitively; anything else (missing
// header, other schemes, no token after the scheme) reports false.
func bearerToken(r *http.Request) (string, bool) {
	header := r.Header.Get("Authorization")
	scheme, token, ok := strings.Cut(header, " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return "", false
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return "", false
	}
	return token, true
}

// escapeRealm neutralizes quotes and backslashes so a configured realm
// cannot break out of the quoted-string around it.
func escapeRealm(realm string) string {
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(realm)
}
