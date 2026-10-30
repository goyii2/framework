package web

import (
	"context"
	"errors"
	"sync"

	"github.com/goyii2/framework/session"
)

// errLoginRequiresIdentity and errLoginRequiresSession are the two ways
// Login refuses: binding nothing is meaningless, and without the session
// middleware there is nowhere for the login to live.
var (
	errLoginRequiresIdentity = errors.New("web: login requires an identity")
	errLoginRequiresSession  = errors.New("web: login requires the session middleware")
)

// sessionKeyIdentityID is the session key carrying the logged-in identity's
// id between requests.
const sessionKeyIdentityID = "__id"

// User is the request-scoped authentication state, the counterpart of the
// session: Login binds an identity to the session (rotating the session id),
// Logout invalidates it, and Identity resolves the current identity — from
// the session through the manager's IdentityFinder, or the transient one an
// authentication middleware established. Can asks the authorization backend
// (the AccessChecker wired on the handler) what the identity holds.
// Reach it through Context.User.
type User struct {
	mgr     *session.Manager
	session *session.Session
	// checker is the authorization backend Can consults; installed from the
	// request Context when the User is created.
	checker AccessChecker

	mu       sync.Mutex
	identity Identity
	resolved bool
}

// newUser builds the request User bound to its session and authorization
// backend.
func newUser(mgr *session.Manager, session *session.Session, checker AccessChecker) *User {
	return &User{mgr: mgr, session: session, checker: checker}
}

// Identity returns the authenticated identity for this request; nil means
// the requester is a guest. Resolution is lazy and cached: the session's
// stored id travels through the manager's IdentityFinder exactly once. An id
// that no longer resolves (a since-deleted account) degrades to a guest and
// drops the stale key rather than failing the request.
func (u *User) Identity() Identity {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.resolved {
		return u.identity
	}
	u.resolved = true
	if u.session == nil || u.mgr == nil || u.mgr.IdentityFinder == nil {
		return nil
	}
	id := u.session.GetString(sessionKeyIdentityID)
	if id == "" {
		return nil
	}
	identity, ok := u.mgr.IdentityFinder(id)
	if !ok {
		u.session.Remove(sessionKeyIdentityID)
		return nil
	}
	u.identity = identity
	return u.identity
}

// IsGuest reports whether the request is unauthenticated.
func (u *User) IsGuest() bool { return u.Identity() == nil }

// Login binds identity to the session. The session id rotates as part of
// logging in — the defense against session fixation: any id an attacker
// planted on the victim before login is discarded together with its record,
// and the logged-in state only ever lives under a fresh, unpredictable id.
// The identity is immediately current for the rest of the request.
func (u *User) Login(identity Identity) error {
	if identity == nil {
		return errLoginRequiresIdentity
	}
	if u.session == nil {
		return errLoginRequiresSession
	}
	u.session.RegenerateID()
	u.session.Set(sessionKeyIdentityID, identity.GetID())
	u.mu.Lock()
	u.identity = identity
	u.resolved = true
	u.mu.Unlock()
	return nil
}

// Logout invalidates the login: the session is destroyed (record removed,
// cookie cleared) and the identity is forgotten — also a transient one
// established by a stateless authenticator this request.
func (u *User) Logout() {
	u.mu.Lock()
	u.identity = nil
	u.resolved = true
	u.mu.Unlock()
	if u.session != nil {
		u.session.Destroy()
	}
}

// setTransient binds identity for this request only, with no session
// involved — how the stateless authentication middlewares (HTTP basic,
// bearer) publish the identity they verified from the request itself.
func (u *User) setTransient(identity Identity) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.identity = identity
	u.resolved = true
}

// ensureUser returns the request's User, creating a session-less one when
// the session middleware did not run (an application with stateless
// authenticators only): transient identities keep working, Login reports
// its missing-session error instead of silently dropping the login, and Can
// keeps the request's authorization backend either way.
func (c *Context) ensureUser() *User {
	if c.user == nil {
		c.user = &User{checker: c.auth}
	}
	return c.user
}

// identityKey is the unexported context key under which WithIdentity
// carries an explicitly bound identity — the current-user seam for callers
// outside an HTTP request.
type identityKey struct{}

// WithIdentity returns a copy of ctx carrying identity as the current
// identity. It is how code without a request of its own — console
// commands, background jobs, tests — publishes the operator to everything
// resolving the user through the context (Blameable-style auditing, for
// instance). Inside a request the session-based identity wins only where
// WithIdentity was not used; an explicitly bound identity always takes
// precedence, so a request handler can act on behalf of a specific user.
func WithIdentity(ctx context.Context, identity Identity) context.Context {
	return context.WithValue(ctx, identityKey{}, identity)
}

// IdentityFromContext resolves the current identity for ctx: the explicit
// value WithIdentity put there, otherwise the request-scoped one from the
// web.Context the HTTP pipeline carries (session login or a stateless
// authenticator's transient identity). Nil when neither is present — a
// guest or a context with no user notion at all.
func IdentityFromContext(ctx context.Context) Identity {
	if identity, ok := ctx.Value(identityKey{}).(Identity); ok && identity != nil {
		return identity
	}
	if c := FromContext(ctx); c != nil {
		if u := c.User(); u != nil {
			return u.Identity()
		}
	}
	return nil
}
