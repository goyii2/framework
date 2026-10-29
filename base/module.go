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
	// ErrInvalidSubmodulePrefix is returned when registering a module under a
	// parent whose prefix it does not extend by exactly one segment.
	ErrInvalidSubmodulePrefix = errors.New("base: submodule prefix must extend the parent prefix by exactly one segment")
	// ErrInvalidControllerID is returned for controller ids that cannot serve
	// as a route segment.
	ErrInvalidControllerID = errors.New(`base: controller id must be non-empty and contain no "/"`)
	// ErrDuplicateControllerID is returned when a controller with the same id
	// is already registered in the module.
	ErrDuplicateControllerID = errors.New("base: a controller with this id is already registered in this module")
)

// Module partitions the application behind a route prefix (e.g. "/admin")
// with its own controller registry: controllers registered on a module are
// owned by it and resolve under its prefix. Modules nest (see
// RegisterModule) — a child module's prefix extends its parent's, and
// controller lookup follows the longest matching prefix down the module
// tree. Components are not duplicated per module — they resolve through the
// owning application via App; module-scoped component overrides remain a
// reserved later capability.
type Module struct {
	app      *Application
	prefix   string
	parent   *Module
	children moduleRegistry

	controllers controllerRegistry
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
	return m.controllers.register(id, c)
}

// Controller returns the controller registered under id in this module.
func (m *Module) Controller(id string) (any, bool) {
	return m.controllers.get(id)
}

// ControllerIDs returns the registered controller ids in registration order.
func (m *Module) ControllerIDs() []string {
	return m.controllers.ids()
}

// controllerRegistry is the controller table shared by the application
// (the root scope) and its modules: ids are unique per scope, kept in
// registration order, and safe for concurrent use.
type controllerRegistry struct {
	mu          sync.RWMutex
	controllers map[string]any
	order       []string // registration order of controller ids
}

// register adds c under id after validating the id and its uniqueness.
func (r *controllerRegistry) register(id string, c any) error {
	if id == "" || strings.Contains(id, "/") {
		return ErrInvalidControllerID
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.controllers[id]; ok {
		return ErrDuplicateControllerID
	}
	if r.controllers == nil {
		r.controllers = make(map[string]any)
	}
	r.controllers[id] = c
	r.order = append(r.order, id)
	return nil
}

// get returns the controller registered under id.
func (r *controllerRegistry) get(id string) (any, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.controllers[id]
	return c, ok
}

// ids returns the registered controller ids in registration order.
func (r *controllerRegistry) ids() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]string(nil), r.order...)
}

// RegisterModule adds child as a submodule of m: the child must belong to
// the same application and its prefix must extend m's prefix by exactly one
// segment (e.g. "/admin" takes "/admin/report" but not "/admin/report/deep"
// — that depth nests through the intermediate module). The child also lands
// in the application's flat module table, so prefix uniqueness is global
// and the router resolves it under its full prefix. Registering a module
// directly on the application keeps it a top-level module with no parent.
func (m *Module) RegisterModule(child *Module) error {
	if child == nil {
		return ErrNilModule
	}
	if child.app != m.app {
		return ErrForeignModule
	}
	rel, ok := strings.CutPrefix(child.prefix, m.prefix+"/")
	if !ok || rel == "" || strings.Contains(rel, "/") {
		return ErrInvalidSubmodulePrefix
	}
	// The application's table is the global uniqueness check; the parent
	// link is only set once the child is committed there, so failures leave
	// no partially attached module behind.
	if err := m.app.modules.register(m.app, child); err != nil {
		return err
	}
	child.parent = m
	return m.children.register(m.app, child)
}

// Parent returns the module this one was nested under, nil for top-level
// modules registered on the application itself.
func (m *Module) Parent() *Module { return m.parent }

// Modules returns the modules nested directly below this one, sorted by
// prefix.
func (m *Module) Modules() []*Module { return m.children.list() }

// moduleRegistry is the module table shared by the application (the flat
// table of all registered prefixes) and by each module (its direct
// children); keyed by route prefix, safe for concurrent use.
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
