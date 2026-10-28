// Package url builds URLs in reverse from route names, mirroring Yii2's
// Url::to semantics for the route-array form: names registered in the
// route table render through their pattern (path parameters filled,
// leftovers become the query string), unregistered names are assembled by
// convention as "/"+name plus the query string.
package url

import (
	"fmt"
	neturl "net/url"
	"strings"
	"sync"
	"sync/atomic"
)

// P is the parameter set for URL generation: parameters named by the
// pattern's wildcards fill the path, the remainder becomes the query
// string. It is a dedicated type rather than a bare map so call sites read
// as URL data.
type P map[string]string

// Manager is the named-route table powering reverse URL generation. The
// handler fills it through Route.Name; lookups are O(1) map reads and safe
// for concurrent use.
type Manager struct {
	mu    sync.RWMutex
	named map[string]string // route name -> path pattern (no method prefix)
}

// NewManager returns an empty manager.
func NewManager() *Manager { return &Manager{} }

// Add registers pattern under name. Registering the same name twice panics:
// like a duplicate net/http pattern it is a boot-time wiring mistake that
// must stop the process instead of silently shadowing a route.
func (m *Manager) Add(name, pattern string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.named[name]; ok {
		panic("url: route name already registered: " + name)
	}
	if m.named == nil {
		m.named = make(map[string]string)
	}
	m.named[name] = pattern
}

// Has reports whether name is registered.
func (m *Manager) Has(name string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.named[name]
	return ok
}

// URL builds the URL for name. Registered names render through their
// pattern; a wildcard without a matching parameter is an error. Names not
// in the table never fail: they assemble by convention as "/"+name plus
// the query string, so plain route ids (e.g. "post/view") work like
// Yii2's Url::to.
func (m *Manager) URL(name string, params P) (string, error) {
	m.mu.RLock()
	pattern, ok := m.named[name]
	m.mu.RUnlock()
	if !ok {
		return convention(name, params), nil
	}
	return build(name, pattern, params)
}

// build fills the wildcards of a ServeMux path pattern with params and
// appends the leftovers as a sorted query string.
func build(name, pattern string, params P) (string, error) {
	rest := make(map[string]string, len(params))
	for k, v := range params {
		rest[k] = v
	}
	var b strings.Builder
	for i, seg := range strings.Split(pattern, "/") {
		if i > 0 {
			b.WriteByte('/')
		}
		id, _ := wildcardName(seg)
		if id == "" {
			b.WriteString(seg)
			continue
		}
		v, ok := rest[id]
		if !ok {
			return "", fmt.Errorf("url: route %q requires parameter %q", name, id)
		}
		delete(rest, id)
		b.WriteString(neturl.PathEscape(v))
	}
	if q := query(rest); q != "" {
		b.WriteByte('?')
		b.WriteString(q)
	}
	return b.String(), nil
}

// convention assembles the URL for an unregistered route id: the id
// becomes the path and the parameters the query string — the fallback
// Yii2's UrlManager applies when no rule matches the route.
func convention(name string, params P) string {
	var b strings.Builder
	b.WriteByte('/')
	b.WriteString(strings.Trim(name, "/"))
	rest := make(map[string]string, len(params))
	for k, v := range params {
		rest[k] = v
	}
	if q := query(rest); q != "" {
		b.WriteByte('?')
		b.WriteString(q)
	}
	return b.String()
}

// query encodes the leftover parameters with keys sorted, so generated
// URLs are deterministic regardless of map iteration order.
func query(params map[string]string) string {
	if len(params) == 0 {
		return ""
	}
	vs := make(neturl.Values, len(params))
	for k, v := range params {
		vs[k] = []string{v}
	}
	return vs.Encode()
}

// wildcardName extracts the parameter name from a ServeMux pattern
// segment: "{id}" and "{id:[0-9]+}" yield "id", the multi-segment
// "{path...}" yields "path"; literal segments yield "".
func wildcardName(seg string) (string, bool) {
	if len(seg) < 2 || seg[0] != '{' || seg[len(seg)-1] != '}' {
		return "", false
	}
	inner := seg[1 : len(seg)-1]
	multi := strings.HasSuffix(inner, "...")
	inner = strings.TrimSuffix(inner, "...")
	if i := strings.IndexByte(inner, ':'); i >= 0 {
		inner = inner[:i]
	}
	if inner == "" {
		return "", false
	}
	return inner, multi
}

// defaultManager holds the package-level default manager behind To.
var defaultManager atomic.Pointer[Manager]

// Default returns the package-level default manager used by To; it is
// created lazily and stays empty until an application installs its route
// table with SetDefault.
func Default() *Manager {
	if m := defaultManager.Load(); m != nil {
		return m
	}
	m := NewManager()
	if defaultManager.CompareAndSwap(nil, m) {
		return m
	}
	return defaultManager.Load()
}

// SetDefault installs m as the package-level default behind To; nil resets
// it to a fresh empty manager.
func SetDefault(m *Manager) { defaultManager.Store(m) }

// To builds the URL for name through the default manager: registered names
// render through their pattern, unregistered names assemble by convention.
// When generation fails (a registered route lacks a path parameter) it
// returns "#", Yii2's cannot-create marker, rather than a broken URL.
func To(name string, params P) string {
	u, err := Default().URL(name, params)
	if err != nil {
		return "#"
	}
	return u
}
