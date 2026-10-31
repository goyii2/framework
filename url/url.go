// Package url builds URLs in reverse from route names, mirroring Yii2's
// Url::to semantics for the route-array form: names registered in the
// route table render through their pattern (path parameters filled,
// leftovers become the query string), unregistered names are assembled by
// convention as "/"+name plus the query string.
package url

import (
	"errors"
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
	mu       sync.RWMutex
	named    map[string]string // route name -> path pattern (no method prefix)
	hostInfo string            // scheme://host[:port] prefix for absolute URLs; empty until SetHostInfo
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

// SetHostInfo sets the scheme-and-host prefix ("scheme://host[:port]") that
// absolute URL generation puts in front of generated URLs. It is written
// once at boot by wiring code and only read afterwards; a trailing slash is
// tolerated and normalized away, so "https://example.com/" and
// "https://example.com" yield the same URLs. A value without a scheme or
// without a host (or carrying userinfo, a path, a query) is a boot-time
// wiring mistake and panics, the same fail-fast philosophy as duplicate
// Add: continuing would hand half-configured absolute URLs to consumers
// with no request context to notice, such as mailers and OAuth callbacks.
func (m *Manager) SetHostInfo(raw string) {
	hostInfo, err := normalizeHostInfo(raw)
	if err != nil {
		panic(fmt.Sprintf("%s (boot-time wiring mistake)", err))
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.hostInfo = hostInfo
}

// normalizeHostInfo validates and normalizes a scheme-and-host prefix: a
// trailing slash is tolerated, anything beyond scheme://host[:port] — no
// scheme, no host, userinfo, a path, a query, a fragment — is rejected.
// Boot-time wiring (SetHostInfo) and per-build host overrides (Builder.Host)
// funnel through here so the accepted shape is identical everywhere.
func normalizeHostInfo(raw string) (string, error) {
	u, err := neturl.Parse(strings.TrimRight(raw, "/"))
	if err != nil || u.User != nil || u.Scheme == "" || u.Host == "" ||
		strings.TrimSuffix(u.Path, "/") != "" || u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("url: invalid host info %q: want scheme://host[:port]", raw)
	}
	return u.Scheme + "://" + u.Host, nil
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
	return renderPattern(name, pattern, params)
}

// absoluteURL builds the absolute URL for name: exactly the generation
// rules of URL (registered names render through their pattern, unregistered
// names assemble by convention) with the configured host info in front.
// Without host info it is an explicit error rather than a half URL —
// absolute generation only makes sense once wiring has supplied the scheme
// and host. It backs the builder's Absolute, which turns its failures into
// the "#" cannot-create marker.
func (m *Manager) absoluteURL(name string, params P) (string, error) {
	m.mu.RLock()
	hostInfo := m.hostInfo
	m.mu.RUnlock()
	if hostInfo == "" {
		return "", errors.New("url: host info not configured: set it with SetHostInfo before generating absolute URLs (boot-time wiring missing)")
	}
	rel, err := m.URL(name, params)
	if err != nil {
		return "", err
	}
	return hostInfo + rel, nil
}

// renderPattern fills the wildcards of a ServeMux path pattern with params
// and appends the leftovers as a sorted query string.
func renderPattern(name, pattern string, params P) (string, error) {
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

// defaultManager holds the package-level default manager behind To and Build.
var defaultManager atomic.Pointer[Manager]

// Default returns the package-level default manager used by To and Build;
// it is created lazily and stays empty until an application installs its
// route table with SetDefault.
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

// SetDefault installs m as the package-level default behind To and Build;
// nil resets it to a fresh empty manager.
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

// Builder is a chainable URL generator seeded by Build. Chaining is the Go
// rendering of Yii2's optional second Url::to argument: the choice between
// a relative and an absolute URL happens at the call site (how far the
// chain runs), while the value comes from configuration (the manager holds
// the host info). The intended lifetime is a single expression:
//
//	url.Build("post/view", url.P{"id": "7"}).Absolute()
//
// The name avoids calling the type URL, which would read as net/url.URL.
type Builder struct {
	m      *Manager
	name   string
	params P
	host   string // explicit scheme-and-host override for this build only; empty means the manager's host info
}

// Build starts URL generation for name through the default manager, the
// same table behind To. Finish the chain with String for a relative URL or
// Absolute for a full one.
func Build(name string, params P) Builder {
	return Builder{m: Default(), name: name, params: params}
}

// Host pins the scheme-and-host prefix for this build only, overriding the
// manager's host info — the cross-partition / multi-tenant case where one
// deployment must emit links on another deployment's domain. The value is
// validated when Absolute runs (same shape as SetHostInfo, trailing slash
// tolerated); an invalid one makes Absolute answer "#" rather than panic on
// the request path. An empty h clears a previous override.
func (b Builder) Host(h string) Builder {
	b.host = h
	return b
}

// String finishes the build with a relative URL: the same outcome as
// To(name, params), "#" when generation fails (a registered route lacking
// a path parameter). The host info and any Host override play no part.
func (b Builder) String() string {
	rel, err := b.m.URL(b.name, b.params)
	if err != nil {
		return "#"
	}
	return rel
}

// Absolute finishes the build with a full URL: the prefix from Host when
// one was given, otherwise the manager's host info, in front of the same
// relative URL String would produce. Any failure — host info not
// configured, an invalid Host value, a registered route lacking a path
// parameter — answers "#", the cannot-create marker, so URL assembly never
// panics on the request path.
func (b Builder) Absolute() string {
	if b.host == "" {
		u, err := b.m.absoluteURL(b.name, b.params)
		if err != nil {
			return "#"
		}
		return u
	}
	prefix, err := normalizeHostInfo(b.host)
	if err != nil {
		return "#"
	}
	rel, err := b.m.URL(b.name, b.params)
	if err != nil {
		return "#"
	}
	return prefix + rel
}
