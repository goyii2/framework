package web

import (
	"net/http"

	"github.com/goyii2/framework/session"
)

// sessionMiddleware opens the request session and creates the request User:
// the manager comes from the application's session component (the entry
// point installs this middleware in the global chain). Without a request
// Context in reach (middleware reused standalone) it passes through: there
// is nothing to attach the session to.
func sessionMiddleware(m *session.Manager) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c := FromContext(r.Context())
			if c == nil {
				next.ServeHTTP(w, r)
				return
			}
			s := m.Begin(w, r)
			c.session = s
			c.user = newUser(m, s, c.auth)
			defer m.Finish(c.App().Log(), s)
			next.ServeHTTP(w, r)
		})
	}
}
