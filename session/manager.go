package session

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// Config carries the session component values. The zero value selects the
// file backend with the framework defaults (cookie "goyii-session", 24-minute
// records, the os temporary directory for storage); the redis backend
// requires a client injected with WithRedis.
type Config struct {
	Name    string `yaml:"name"`    // session cookie name; default goyii-session
	Backend string `yaml:"backend"` // file | redis; default file
	Path    string `yaml:"path"`    // file backend storage directory
	Timeout int    `yaml:"timeout"` // seconds; default 1440 (24 minutes)
}

// Option configures a Manager at construction; options carry cross-component
// dependencies (WithRedis), values live in Config.
type Option func(*options)

type options struct {
	redis redis.UniversalClient
}

// WithRedis hands the redis backend the connection session records are stored
// on — typically the application's shared connection, so sessions share a
// database with the other redis-backed components. The file backend ignores
// it.
func WithRedis(client redis.UniversalClient) Option {
	return func(o *options) { o.redis = client }
}

// New builds the session manager Config selects: the file backend (the
// zero-value default) persists records under Path, the redis backend stores
// on the client injected with WithRedis and refuses construction without one.
// Cookie attributes get their defensive defaults (HttpOnly, SameSite=Lax);
// callers adjust them on the returned Manager before serving traffic.
func New(cfg Config, opts ...Option) (*Manager, error) {
	var o options
	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}
	timeout := time.Duration(cfg.Timeout) * time.Second
	if timeout <= 0 {
		timeout = 1440 * time.Second
	}
	m := &Manager{
		CookieName: orDefault(cfg.Name, "goyii-session"),
		Timeout:    timeout,
		HttpOnly:   true,
		SameSite:   http.SameSiteLaxMode,
	}
	switch strings.ToLower(strings.TrimSpace(cfg.Backend)) {
	case "", "file":
		dir := cfg.Path
		if dir == "" {
			dir = filepath.Join(os.TempDir(), "goyii-session")
		}
		m.Store = NewFileStore(dir)
	case "redis":
		if o.redis == nil {
			return nil, errors.New("session: redis backend requires a redis client (pass one with WithRedis)")
		}
		m.Store = NewRedisStore(o.redis, "goyii:session:")
	default:
		return nil, fmt.Errorf("session: unknown backend %q (want file or redis)", cfg.Backend)
	}
	return m, nil
}

// orDefault returns value when non-empty, otherwise fallback.
func orDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

// Manager wires the request session: it reads the session cookie, opens the
// request-scoped Session, then persists changes as the request completes. The
// web entry point installs the manager's middleware in the global chain from
// the application's session component; adjusting its fields before serving
// traffic is the customization point — most notably IdentityFinder, which
// resolves stored identity ids back to identities for the request User.
type Manager struct {
	// Store persists the session records (file or redis backend).
	Store Store

	// CookieName is the cookie carrying the session id; default
	// "goyii-session".
	CookieName string

	// Timeout is the record's lifetime, refreshed on every write (sliding
	// expiration); default 24 minutes.
	Timeout time.Duration

	// HttpOnly hides the session cookie from scripts (default true), Secure
	// restricts it to HTTPS, SameSite defaults to Lax — standard defenses
	// against theft and cross-site sending of the id.
	HttpOnly bool
	Secure   bool
	SameSite http.SameSite

	// IdentityFinder resolves a stored identity id back to the identity for
	// the web User identity resolution; without it stored logins resolve to
	// guests.
	IdentityFinder func(id string) (Identity, bool)
}

// Begin opens the request-scoped session: the id cookie the request carried
// (when its shape is valid) becomes the session's incoming id. The caller
// publishes the session into its request state and runs Finish as the request
// unwinds.
func (m *Manager) Begin(w http.ResponseWriter, r *http.Request) *Session {
	s := &Session{mgr: m, w: w}
	if ck, err := r.Cookie(m.CookieName); err == nil && validSessionID(ck.Value) {
		s.id = ck.Value
		s.incomingID = ck.Value
	}
	return s
}

// ErrorLogger receives Finish failures. The application's log component (and
// anything else with an slog-shaped Error) satisfies it.
type ErrorLogger interface {
	Error(msg string, args ...any)
}

// Finish runs the backend side of the session lifecycle as the request
// unwinds: destroyed sessions drop their records, dirty sessions persist.
// Failures are reported to log (nil swallows them) — the response is already
// committed or about to, and failing the request after the action ran serves
// nobody.
func (m *Manager) Finish(log ErrorLogger, s *Session) {
	s.mu.Lock()
	defer s.mu.Unlock()

	destroy := func(id string) {
		if err := m.Store.Destroy(context.Background(), id); err != nil && log != nil {
			log.Error("session destroy failed", "err", err.Error(), "session", id)
		}
	}

	if s.destroyed {
		if s.incomingID != "" {
			destroy(s.incomingID)
		}
		if s.id != "" && s.id != s.incomingID {
			destroy(s.id)
		}
		return
	}

	if !s.dirty {
		return
	}
	s.ensureID()
	if err := m.Store.Save(context.Background(), s.id, s.data, m.Timeout); err != nil {
		if log != nil {
			log.Error("session save failed", "err", err.Error(), "session", s.id)
		}
		return
	}
	if s.regenerated && s.incomingID != "" && s.incomingID != s.id {
		destroy(s.incomingID)
	}
}
