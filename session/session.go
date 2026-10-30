package session

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"sync"
)

// Session is the request-scoped session: a key/value bag whose contents
// survive across requests through the configured backend. The web middleware
// creates one per request (Manager.Begin) and hands it out through the
// request Context. The session opens lazily: nothing touches the backend (and
// no cookie is issued) until the request actually reads or writes a value.
type Session struct {
	mgr *Manager
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

// All returns a copy of the session's current values, loading the stored
// record on first access like Get does; an empty map when nothing is stored
// or the session was destroyed. It is the read-only snapshot the debug panel
// renders — mutating the copy affects no live state.
func (s *Session) All() map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureLoaded()
	out := make(map[string]any, len(s.data))
	for k, v := range s.data {
		out[k] = v
	}
	return out
}

// Set stores value under key for subsequent requests. Writing to a destroyed
// session is a no-op: the session stays dead for the rest of the request.
// Sessions should be mutated before the action writes its response: the id
// cookie is published the moment it changes, and headers cannot travel with a
// body already sent.
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
// completes. A destroyed session stays empty for the remainder of the request.
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

// setSessionCookie writes the session cookie header, replacing any value
// previously published for the same cookie name (a rotation followed by a
// destroy leaves exactly one authoritative header). maxAge < 0 clears the
// cookie; 0 omits the attribute, making it a browser-session cookie.
func setSessionCookie(h http.Header, m *Manager, value string, maxAge int) {
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
		panic("session: csprng unavailable: " + err.Error())
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// validSessionID reports whether v has the shape of a minted id: only
// URL-safe base64 characters, at least 22 characters (16 decoded bytes), at
// most 128. Ids double as file names in the file backend, so the shape check
// also bars path fragments from forged cookies.
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
