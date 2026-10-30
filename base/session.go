package base

import (
	"github.com/goyii2/framework/session"
)

// Session returns the session component — the manager the web entry point
// installs its request-session middleware from. The fallback is the file
// backend under the os temporary directory; applications with real cookie
// names, paths or the redis backend install the manager their configuration
// selects (session.New) with SetSession before serving.
func (a *Application) Session() *session.Manager { return a.session }

// SetSession replaces the session component; nil restores the file-backend
// fallback. Configure the manager's cookie attributes and IdentityFinder
// before serving traffic.
func (a *Application) SetSession(m *session.Manager) {
	if m == nil {
		m = newDefaultSession()
	}
	a.session = m
}

// newDefaultSession builds the fallback session component: the file backend
// with the default cookie and lifetime. The zero-value session.Config
// selects it, so this constructor cannot fail; a failure here is a framework
// bug and panics.
func newDefaultSession() *session.Manager {
	m, err := session.New(session.Config{})
	if err != nil {
		panic("goyii: building the default session component: " + err.Error())
	}
	return m
}
