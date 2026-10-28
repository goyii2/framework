package base

import (
	"errors"
	"sort"
	"strings"
	"sync"
)

// Errors reported when assembling the module registry; all of them signal
// setup mistakes made at boot time.
var (
	// ErrNilModule is returned when registering a nil module.
	ErrNilModule = errors.New("base: cannot register a nil module")
	// ErrForeignModule is returned when the module was created for another
	// application.
	ErrForeignModule = errors.New("base: module belongs to another application")
	// ErrInvalidModulePrefix is returned for prefixes that do not name a
	// non-empty path segment below the root.
	ErrInvalidModulePrefix = errors.New(`base: module prefix must start with "/" and name a non-empty segment, e.g. "/admin"`)
	// ErrDuplicateModulePrefix is returned when a module with the same prefix
	// is already registered.
	ErrDuplicateModulePrefix = errors.New("base: a module with this prefix is already registered")
	// ErrModuleNestingUnsupported is returned by Module.RegisterModule until
	// module nesting is implemented.
	ErrModuleNestingUnsupported = errors.New("base: nesting modules is not supported yet")
	// ErrInvalidControllerID is returned for controller ids that cannot serve
	// as a route segment.
	ErrInvalidControllerID = errors.New(`base: controller id must be non-empty and contain no "/"`)
	// ErrDuplicateControllerID is returned when a controller with the same id
	// is already registered in the module.
	ErrDuplicateControllerID = errors.New("base: a controller with this id is already registered in this module")
)

// Module partitions the application behind a route prefix (e.g. "/admin")
// with its own controller registry: controllers registered on a module are
// owned by it and resolve under its prefix. Components are not duplicated
// per module — they resolve through the owning application via App;
// module-scoped component overrides are a reserved later capability, as is
// nesting (see RegisterModule).
type Module struct {
	app    *Application
	prefix string

	mu          sync.RWMutex
	controllers map[string]any
	order       []string // registration order of controller ids
}

// NewModule creates a module bound to app under the given route prefix. A
// trailing "/" is trimmed; the result must start with "/" and name a
// non-empty segment ("" and "/" alone are rejected — the application itself
// is the root scope).
func NewModule(app *Application, prefix string) (*Module, error) {
	prefix = strings.TrimRight(prefix, "/")
	if len(prefix) < 2 || prefix[0] != '/' {
		return nil, ErrInvalidModulePrefix
	}
	return &Module{app: app, prefix: prefix}, nil
}

// App returns the application the module resolves components from.
func (m *Module) App() *Application { return m.app }

// Prefix returns the module route prefix, e.g. "/admin".
func (m *Module) Prefix() string { return m.prefix }

// RegisterController registers c under id as owned by this module. The id
// becomes the controller segment of conventional routes (e.g. "user" serves
// /admin/user/<action>); duplicate ids are rejected.
func (m *Module) RegisterController(id string, c any) error {
	if id == "" || strings.Contains(id, "/") {
		return ErrInvalidControllerID
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.controllers[id]; ok {
		return ErrDuplicateControllerID
	}
	if m.controllers == nil {
		m.controllers = make(map[string]any)
	}
	m.controllers[id] = c
	m.order = append(m.order, id)
	return nil
}

// Controller returns the controller registered under id in this module.
func (m *Module) Controller(id string) (any, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	c, ok := m.controllers[id]
	return c, ok
}

// ControllerIDs returns the registered controller ids in registration order.
func (m *Module) ControllerIDs() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]string(nil), m.order...)
}

// RegisterModule reserves sub-module registration: nesting modules under a
// module is planned but not implemented, so it always reports
// ErrModuleNestingUnsupported.
func (m *Module) RegisterModule(child *Module) error { return ErrModuleNestingUnsupported }

// moduleRegistry is the application's single-layer module table keyed by
// route prefix; safe for concurrent use.
type moduleRegistry struct {
	mu       sync.RWMutex
	byPrefix map[string]*Module
}

// register adds m to the table after ownership and prefix checks.
func (r *moduleRegistry) register(app *Application, m *Module) error {
	if m == nil {
		return ErrNilModule
	}
	if m.app != app {
		return ErrForeignModule
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.byPrefix == nil {
		r.byPrefix = make(map[string]*Module)
	}
	if _, ok := r.byPrefix[m.prefix]; ok {
		return ErrDuplicateModulePrefix
	}
	r.byPrefix[m.prefix] = m
	return nil
}

// lookup returns the module registered under prefix.
func (r *moduleRegistry) lookup(prefix string) (*Module, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	m, ok := r.byPrefix[prefix]
	return m, ok
}

// list returns all registered modules sorted by prefix.
func (r *moduleRegistry) list() []*Module {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ms := make([]*Module, 0, len(r.byPrefix))
	for _, m := range r.byPrefix {
		ms = append(ms, m)
	}
	sort.Slice(ms, func(i, j int) bool { return ms[i].prefix < ms[j].prefix })
	return ms
}
