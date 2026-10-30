package base

import (
	"context"
	"sync/atomic"

	"github.com/redis/go-redis/v9"

	"github.com/goyii2/framework/cache"
	"github.com/goyii2/framework/db"
	"github.com/goyii2/framework/i18n"
	"github.com/goyii2/framework/mail"
	"github.com/goyii2/framework/mutex"
	"github.com/goyii2/framework/queue"
	"github.com/goyii2/framework/session"
)

// Application is the process-level component container. Components are
// strong-typed fields behind zero-error accessors: the constructor installs
// fallback defaults for every component that has one, the application entry
// replaces them (Set methods) with the components it built from its own
// configuration, and db / redis have no fallback — accessing them without
// wiring one is a programmer error and panics with a message saying so.
// Request-scoped code receives the application through the context
// (WithContext / FromContext); code without a request context uses the
// package-level Default instance.
type Application struct {
	name  string
	env   string
	debug bool

	log     Log
	db      *db.Conn
	redis   redis.UniversalClient
	cache   cache.Cache
	mutex   mutex.Mutex
	mail    mail.Mailer
	queue   *queue.Queue
	session *session.Manager
	i18n    *i18n.MessageSource

	events      eventRegistry
	modules     moduleRegistry
	controllers controllerRegistry
}

// Option configures an Application at construction. Options carry the
// application's identity only; components are installed with the Set methods
// after their construction succeeded in the application entry.
type Option func(*Application)

// WithApp sets the application's identity: its name, the environment it runs
// in (typically config.Env()) and the debug flag steering the error pages and
// the debug panel. Empty values keep the defaults ("myapp", "dev").
func WithApp(name, env string, debug bool) Option {
	return func(a *Application) {
		if name != "" {
			a.name = name
		}
		if env != "" {
			a.env = env
		}
		a.debug = debug
	}
}

// New assembles an Application. Everything except the database and redis
// connections gets a working fallback default (console logging, memory cache,
// process mutex, log mailer, file sessions, sync queue, empty message
// source); the application entry replaces what its configuration says to
// replace. The database and redis connections are never built here — opening
// them can fail, so construction belongs to the caller (see ConnectDB and the
// Set methods).
func New(opts ...Option) *Application {
	a := &Application{name: "myapp", env: "dev"}
	for _, opt := range opts {
		if opt != nil {
			opt(a)
		}
	}
	a.log = newDefaultLog()
	a.cache = newDefaultCache()
	a.mutex = newDefaultMutex()
	a.mail = newDefaultMail(a)
	a.session = newDefaultSession()
	a.i18n = newDefaultI18n()
	a.queue = a.newDefaultQueue()
	return a
}

// Name returns the application's name (debug pages and log context).
func (a *Application) Name() string { return a.name }

// Env returns the environment the application runs in (typically selected by
// the project's configuration loading; "dev" when nothing set one).
func (a *Application) Env() string { return a.env }

// Debug reports whether the application runs in debug mode: error responses
// reveal their real error text and the debug panel mounts.
func (a *Application) Debug() bool { return a.debug }

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

// RegisterModule adds m to the application's module table so the router can
// resolve its prefix. The table is flat over full prefixes (nested children
// registered through Module.RegisterModule land here too), prefix uniqueness
// is global, and modules must belong to this application.
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
// downstream code can retrieve it with FromContext. The application's i18n
// message source rides along under its own key, so translation helpers
// (i18n.T, i18n.F) resolve it without depending on this package.
func (a *Application) WithContext(ctx context.Context) context.Context {
	ctx = context.WithValue(ctx, appKey{}, a)
	return i18n.WithSource(ctx, a.i18n)
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
// created lazily with the fallback components only on first use; SetDefault
// installs an explicitly assembled application instead.
func Default() *Application {
	if app := defaultApp.Load(); app != nil {
		return app
	}
	app := New()
	if defaultApp.CompareAndSwap(nil, app) {
		return app
	}
	return defaultApp.Load()
}

// SetDefault installs app as the package-level default; nil clears it so the
// next Default call rebuilds with the fallback components.
func SetDefault(app *Application) {
	defaultApp.Store(app)
}
