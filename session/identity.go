package session

// Identity is the contract every authenticated party fulfills. Applications
// implement it on their user model (or any lightweight principal); the
// authentication middlewares and the web User component carry instances, and
// access control derives the requester's roles from it in its default
// configuration. The roles accessor is the identity-side half of the
// authorization seam: an RBAC manager can replace how roles are resolved
// without identities changing shape.
type Identity interface {
	// GetID returns the unique, stable identifier (the value stored in the
	// session on login and resolved back through the manager's
	// IdentityFinder).
	GetID() string

	// GetName returns the human-facing name for display and logging.
	GetName() string

	// GetRoles returns the roles the identity holds, consumed by role-based
	// access control.
	GetRoles() []string
}
