package view

import (
	"fmt"
	"sort"
	"sync"

	"github.com/a-h/templ"
)

// Factory builds a view component from the data an action passes to
// web.Context.Render. The data type is a private contract between the
// registration site and the factory — the factory asserts the concrete type
// it expects, so the render path is a map lookup plus a function call with
// no reflection. The Typed helper derives the assertion from a typed
// constructor.
type Factory func(data any) templ.Component

// Registry is the named-view table behind web.Context.Render: view names
// (Yii2 route-id style, e.g. "post/view") map to factories. Register views
// while assembling the application; the table is then read concurrently by
// request handlers. The names "error/<status>" and "error" are reserved for
// the unified error handler's page overrides (see ErrorData).
type Registry struct {
	mu        sync.RWMutex
	factories map[string]Factory
}

// NewRegistry creates an empty view registry.
func NewRegistry() *Registry {
	return &Registry{factories: map[string]Factory{}}
}

// Register adds the factory under name. Registering is part of application
// assembly: a duplicate name, an empty name or a nil factory is a wiring
// mistake and panics like a duplicate route pattern would.
func (r *Registry) Register(name string, f Factory) {
	if name == "" {
		panic("view: register: name must not be empty")
	}
	if f == nil {
		panic(fmt.Sprintf("view: register %q: factory must not be nil", name))
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, dup := r.factories[name]; dup {
		panic(fmt.Sprintf("view: register %q: view already registered", name))
	}
	r.factories[name] = f
}

// Lookup returns the factory registered under name; ok is false for unknown
// names, which the caller turns into a server error (a missing view is a
// programming mistake, not a client error).
func (r *Registry) Lookup(name string) (f Factory, ok bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	f, ok = r.factories[name]
	return f, ok
}

// Names returns the registered view names in sorted order, for diagnostics
// and tests.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.factories))
	for name := range r.factories {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Typed adapts a typed view constructor into a Factory, asserting the data
// passed to Render. Render receives whatever the action hands over, so a
// mismatched type is a programming mistake caught the first time the view
// renders: Typed panics with both types named, which the request pipeline
// recovers into a logged 500.
func Typed[T any](fn func(data T) templ.Component) Factory {
	return func(data any) templ.Component {
		typed, ok := data.(T)
		if !ok {
			panic(fmt.Sprintf("view: data is %T, want %T", data, typed))
		}
		return fn(typed)
	}
}
