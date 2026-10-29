package web

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/goyii2/framework/config"
	"github.com/redis/go-redis/v9"
)

// SessionStore persists session records between requests. Implementations
// receive the record's remaining lifetime with every call so the lifetime
// stays owned by the session manager; a store may also apply it server-side
// (redis TTL) or check it against the record's age (file modification time).
// All three methods must be safe for concurrent use.
type SessionStore interface {
	// Load returns the stored data for id; ok is false when the id is
	// unknown or its record expired. An expired record is best removed.
	Load(ctx context.Context, id string, ttl time.Duration) (data map[string]any, ok bool, err error)
	// Save stores data under id, replacing any previous record, and
	// refreshes the record's lifetime to ttl.
	Save(ctx context.Context, id string, data map[string]any, ttl time.Duration) error
	// Destroy removes the record for id; removing an unknown id is not an
	// error.
	Destroy(ctx context.Context, id string) error
}

// Session is the request-scoped session: a key/value bag whose contents
// survive across requests through the configured backend. The manager
// middleware creates one per request and hands it out through
// Context.Session. The session opens lazily: nothing touches the backend (and
// no cookie is issued) until the request actually reads or writes a value.
type Session struct {
	mgr *SessionManager
	w   http.ResponseWriter

	mu          sync.Mutex
	id          string // current id; "" until assigned
	incomingID  string // id the request cookie carried; "" when absent
	data        map[string]any
	loaded      bool
	dirty       bool
	regenerated bool
	destroyed   bool
}

// ID returns the session id, assigning a fresh cryptographic one when the
// session has none yet (first use). Assignment alone persists nothing; the
// record lands in the backend only when values were written.
func (s *Session) ID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureID()
	return s.id
}

// Get returns the value stored under key (nil when absent).
func (s *Session) Get(key string) any {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureLoaded()
	return s.data[key]
}

// GetString returns the value stored under key as a string; any other shape
// (or absence) yields "".
func (s *Session) GetString(key string) string {
	v, _ := s.Get(key).(string)
	return v
}

// Set stores value under key for subsequent requests. Writing to a
// destroyed session is a no-op: the session stays dead for the rest of the
// request. Sessions should be mutated before the action writes its
// response: the id cookie is published the moment it changes, and headers
// cannot travel with a body already sent.
func (s *Session) Set(key string, value any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.destroyed {
		return
	}
	s.ensureLoaded()
	s.ensureID()
	s.data[key] = value
	s.dirty = true
	s.publishCookie()
}

// Remove drops key from the session; absent keys are ignored.
func (s *Session) Remove(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureLoaded()
	if _, ok := s.data[key]; !ok {
		return
	}
	delete(s.data, key)
	s.dirty = true
}

// RegenerateID assigns the session a fresh id while keeping its data. Binding
// a privilege change (login) to a new id is the session-fixation defense: an
// id an attacker planted on the victim before login never gains the logged-in
// state. The old record is destroyed once the request completes.
func (s *Session) RegenerateID() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.destroyed {
		return
	}
	// Pull the record under the current id before leaving it behind: the
	// data moves to the fresh id on the next save.
	s.ensureLoaded()
	s.id = newSessionID()
	s.regenerated = true
	s.publishCookie()
}

// Destroy invalidates the session entirely: the backend record (current and
// incoming id) is removed and the cookie is cleared once the request
// completes. A destroyed session stays empty for the remainder of the
// request.
func (s *Session) Destroy() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.destroyed {
		return
	}
	s.destroyed = true
	s.data = map[string]any{}
	s.publishCookie()
}

// ensureLoaded pulls the stored record into memory on first access. Called
// with s.mu held.
func (s *Session) ensureLoaded() {
	if s.loaded {
		return
	}
	s.loaded = true
	if s.id == "" {
		s.data = map[string]any{}
		return
	}
	data, ok, err := s.mgr.Store.Load(context.Background(), s.id, s.mgr.Timeout)
	switch {
	case err != nil:
		// A failing read degrades to an empty session: requests proceed as
		// guests rather than erroring out mid-action.
		slog.Error("session load failed", "err", err.Error(), "session", s.id)
		s.data = map[string]any{}
	case ok:
		if data == nil {
			data = map[string]any{}
		}
		s.data = data
	default:
		// Unknown or expired id: start fresh, but keep the id so the request
		// keeps answering to it until a write assigns a fresh record.
		s.data = map[string]any{}
	}
}

// ensureID assigns a fresh id when the session has none. Called with s.mu
// held.
func (s *Session) ensureID() {
	if s.id == "" {
		s.id = newSessionID()
	}
}

// publishCookie announces the current id (or the session's end) as a cookie
// header on the response. Headers only reach the client when set before the
// body starts, so this runs synchronously with each mutation rather than at
// request end. Called with s.mu held.
func (s *Session) publishCookie() {
	if s.w == nil {
		return
	}
	if s.destroyed {
		if s.incomingID != "" {
			setSessionCookie(s.w.Header(), s.mgr, "", -1)
		}
		return
	}
	if s.id != s.incomingID {
		setSessionCookie(s.w.Header(), s.mgr, s.id, 0)
	}
}

// SessionManager wires the request session: it reads the session cookie,
// opens the request-scoped Session, then persists changes as the request
// completes. The HTTP entry point installs it in the global chain by
// default, built from the application's session configuration; replacing it
// (or its fields) before serving traffic is the customization point — most
// notably IdentityFinder, which resolves stored identity ids back to
// identities for the request User.
type SessionManager struct {
	// Store persists the session records (file or redis backend).
	Store SessionStore

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
	// User identity resolution; without it stored logins resolve to guests.
	IdentityFinder func(id string) (Identity, bool)
}

// NewSessionManager builds the manager from the application session
// configuration: backend "redis" opens its own redis connection from the
// section's values, anything else falls back to the file backend under the
// configured path.
func NewSessionManager(cfg config.Session) *SessionManager {
	return newSessionManager(cfg, nil)
}

// NewSessionManagerWithRedis builds the manager like NewSessionManager but
// takes the redis connection from client instead of opening one — the HTTP
// entry point uses this to share the application's connection across every
// redis-backed component. A nil client opens one from the section's values
// exactly like NewSessionManager.
func NewSessionManagerWithRedis(cfg config.Session, client redis.UniversalClient) *SessionManager {
	return newSessionManager(cfg, client)
}

// newSessionManager assembles the manager; a nil client means the redis
// backend opens its own connection from the session section.
func newSessionManager(cfg config.Session, client redis.UniversalClient) *SessionManager {
	timeout := time.Duration(cfg.Timeout) * time.Second
	if timeout <= 0 {
		timeout = 1440 * time.Second
	}
	m := &SessionManager{
		CookieName: defaultString(cfg.Name, "goyii-session"),
		Timeout:    timeout,
		HttpOnly:   true,
		SameSite:   http.SameSiteLaxMode,
	}
	if cfg.Backend == "redis" {
		if client == nil {
			client = redis.NewClient(&redis.Options{
				Addr:     cfg.Redis.Addr,
				Password: cfg.Redis.Password,
				DB:       cfg.Redis.DB,
			})
		}
		m.Store = NewRedisSessionStore(client, "goyii:session:")
	} else {
		dir := cfg.Path
		if dir == "" {
			dir = filepath.Join(os.TempDir(), "goyii-session")
		}
		m.Store = NewFileSessionStore(dir)
	}
	return m
}

// Middleware opens the request session and creates the request User. Without
// a request Context in reach (middleware reused standalone) it passes
// through: there is nothing to attach the session to.
func (m *SessionManager) Middleware() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c := FromContext(r.Context())
			if c == nil {
				next.ServeHTTP(w, r)
				return
			}
			s := newSession(m, w, r)
			c.session = s
			c.user = newUser(m, s, c.auth)
			defer m.finish(c, s)
			next.ServeHTTP(w, r)
		})
	}
}

// finish runs as the request unwinds and owns the backend side of the
// session lifecycle: destroyed sessions drop their records, dirty sessions
// persist. Failures are logged — the response is already committed or about
// to be, and failing the request after the action ran serves nobody.
func (m *SessionManager) finish(c *Context, s *Session) {
	s.mu.Lock()
	defer s.mu.Unlock()
	log := c.App().Log()

	destroy := func(id string) {
		if err := m.Store.Destroy(context.Background(), id); err != nil {
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
		log.Error("session save failed", "err", err.Error(), "session", s.id)
		return
	}
	if s.regenerated && s.incomingID != "" && s.incomingID != s.id {
		destroy(s.incomingID)
	}
}

// newSession builds the request-scoped session from the request's cookie.
// Cookie values the id validator rejects (wrong shape) open a fresh session
// instead of reaching the store: they can never become record keys.
func newSession(m *SessionManager, w http.ResponseWriter, r *http.Request) *Session {
	s := &Session{mgr: m, w: w}
	if ck, err := r.Cookie(m.CookieName); err == nil && validSessionID(ck.Value) {
		s.id = ck.Value
		s.incomingID = ck.Value
	}
	return s
}

// setSessionCookie writes the session cookie header, replacing any value
// previously published for the same cookie name (a rotation followed by a
// destroy leaves exactly one authoritative header). maxAge < 0 clears the
// cookie; 0 omits the attribute, making it a browser-session cookie.
func setSessionCookie(h http.Header, m *SessionManager, value string, maxAge int) {
	prefix := m.CookieName + "="
	remaining := h.Values("Set-Cookie")
	kept := make([]string, 0, len(remaining))
	for _, v := range remaining {
		if !strings.HasPrefix(v, prefix) {
			kept = append(kept, v)
		}
	}
	h.Del("Set-Cookie")
	for _, v := range kept {
		h.Add("Set-Cookie", v)
	}
	h.Add("Set-Cookie", (&http.Cookie{
		Name:     m.CookieName,
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: m.HttpOnly,
		Secure:   m.Secure,
		SameSite: m.SameSite,
	}).String())
}

// newSessionID mints 32 bytes of cryptographic randomness, URL-safe base64
// encoded (43 characters).
func newSessionID() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		// The system CSPRNG failing means the host is out of entropy
		// sources; refusing to run beats issuing guessable session ids.
		panic("web: csprng unavailable: " + err.Error())
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// validSessionID reports whether v has the shape of a minted id: only
// URL-safe base64 characters, at least 22 characters (16 decoded bytes),
// at most 128. Ids double as file names in the file backend, so the shape
// check also bars path fragments from forged cookies.
func validSessionID(v string) bool {
	if len(v) < 22 || len(v) > 128 {
		return false
	}
	for _, c := range v {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '-', c == '_':
		default:
			return false
		}
	}
	return true
}

// marshalSession encodes data for storage.
func marshalSession(data map[string]any) ([]byte, error) {
	return json.Marshal(data)
}

// unmarshalSession decodes stored bytes; absent bodies decode to nil.
func unmarshalSession(b []byte) (map[string]any, error) {
	var data map[string]any
	if len(b) == 0 {
		return nil, nil
	}
	if err := json.Unmarshal(b, &data); err != nil {
		return nil, err
	}
	return data, nil
}
