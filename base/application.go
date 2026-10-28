package base

import (
	"context"
	"sync/atomic"

	"github.com/goyii2/framework/config"
	"github.com/goyii2/framework/db"
)

// Application is the process-level component container. Components are
// strong-typed fields behind accessors: each is lazily initialized on first
// access (concurrency-safe) and replaceable via its Set method for tests or
// alternative implementations. Request-scoped code receives the application
// through the context (WithContext / FromContext); code without a request
// context uses the package-level Default instance.
type Application struct {
	cfg     config.Config
	log     *component[Log]
	db      *db.Component
	events  eventRegistry
	modules moduleRegistry

	controllers controllerRegistry
}

// New assembles an Application from cfg, filling zero values with framework
// defaults. Component construction is deferred to first access.
func New(cfg config.Config) *Application {
	cfg = config.WithDefaults(cfg)
	return &Application{
		cfg: cfg,
		log: newComponent(func() Log { return newDefaultLog(cfg.Log) }),
		db:  db.NewComponent(cfg.DB),
	}
}

// Config returns the application configuration.
func (a *Application) Config() config.Config { return a.cfg }

// Log returns the logging component, initializing it on first access.
func (a *Application) Log() Log { return a.log.get() }

// SetLog replaces the logging component (e.g. a test fake or a richer
// implementation); nil restores lazy initialization of the default.
func (a *Application) SetLog(l Log) { a.log.set(l) }

// DB returns the database connection component, opening it on first access.
// The connection comes from the configuration's db section; a failed open
// returns the error and is retried on the next call.
func (a *Application) DB() (*db.Conn, error) { return a.db.Get() }

// SetDB replaces the database connection (e.g. a test fake or a pre-opened
// connection); nil restores lazy initialization from the configuration.
func (a *Application) SetDB(c *db.Conn) { a.db.Set(c) }

// On subscribes h to the event name and returns a function that
// unsubscribes it again (idempotent). Handlers run in registration order
// when the event is triggered.
func (a *Application) On(name EventName, h Handler) func() {
	return a.events.on(name, h)
}

// Trigger dispatches the event name to all current subscribers, carrying the
// request context ctx and the optional event-specific data (nil when the
// event carries none).
func (a *Application) Trigger(ctx context.Context, name EventName, data any) {
	a.events.trigger(ctx, Event{Name: name, Data: data})
}

// RegisterModule adds m to the application's single-layer module registry so
// the router can resolve its prefix. Module prefixes are unique and modules
// must belong to this application.
func (a *Application) RegisterModule(m *Module) error { return a.modules.register(a, m) }

// Module returns the module registered under prefix.
func (a *Application) Module(prefix string) (*Module, bool) { return a.modules.lookup(prefix) }

// Modules returns all registered modules sorted by prefix.
func (a *Application) Modules() []*Module { return a.modules.list() }

// RegisterController registers c under id in the application's root scope:
// conventional routes without a module prefix (/{controller}/{action})
// resolve through this registry, while modules keep their own. Duplicate
// ids are rejected.
func (a *Application) RegisterController(id string, c any) error {
	return a.controllers.register(id, c)
}

// Controller returns the controller registered under id in the root scope.
func (a *Application) Controller(id string) (any, bool) { return a.controllers.get(id) }

// ControllerIDs returns the root-scope controller ids in registration order.
func (a *Application) ControllerIDs() []string { return a.controllers.ids() }

// appKey is the unexported context key under which the application travels.
type appKey struct{}

// WithContext returns a copy of ctx carrying the application, so that
// downstream code can retrieve it with FromContext.
func (a *Application) WithContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, appKey{}, a)
}

// FromContext returns the application carried by ctx; when ctx carries none
// (CLI, background jobs) it falls back to the package-level default
// instance.
func FromContext(ctx context.Context) *Application {
	if app, ok := ctx.Value(appKey{}).(*Application); ok && app != nil {
		return app
	}
	return Default()
}

// defaultApp holds the package-level default application.
var defaultApp atomic.Pointer[Application]

// Default returns the package-level default application used wherever no
// request-scoped application is available (CLI, background jobs). It is
// created lazily from framework defaults on first use; SetDefault installs an
// explicitly assembled application instead.
func Default() *Application {
	if app := defaultApp.Load(); app != nil {
		return app
	}
	app := New(config.Default())
	if defaultApp.CompareAndSwap(nil, app) {
		return app
	}
	return defaultApp.Load()
}

// SetDefault installs app as the package-level default; nil clears it so the
// next Default call rebuilds from framework defaults.
func SetDefault(app *Application) {
	defaultApp.Store(app)
}
