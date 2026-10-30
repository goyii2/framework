// Package session owns the session component: the manager wiring the
// request-scoped session, the backends persisting session records (file and
// redis), and the Identity contract stored identities fulfill. The web plane
// (the middleware attaching the session to its request Context) lives in the
// web package; this package stays importable by base, which holds the
// component slot, and by web, which runs it.
package session
