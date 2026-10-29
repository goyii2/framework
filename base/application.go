package base

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/goyii2/framework/cache"
	"github.com/goyii2/framework/config"
	"github.com/goyii2/framework/db"
	"github.com/goyii2/framework/i18n"
	"github.com/goyii2/framework/log"
	"github.com/goyii2/framework/mail"
	"github.com/goyii2/framework/mutex"
)

// Application is the process-level component container. Components are
// strong-typed fields behind accessors: each is lazily initialized on first
// access (concurrency-safe) and replaceable via its Set method for tests or
// alternative implementations. Request-scoped code receives the application
// through the context (WithContext / FromContext); code without a request
// context uses the package-level Default instance.
type Application struct {
	cfg       config.Config
	log       *component[Log]
	db        *db.Component
	redisComp *component[redis.UniversalClient]
	cacheComp *component[cache.Cache]
	mutexComp *component[mutex.Mutex]
	i18nComp  *component[*i18n.MessageSource]
	mailComp  *component[mail.Mailer]
	events    eventRegistry
	modules   moduleRegistry

	controllers controllerRegistry
}

// New assembles an Application from cfg, filling zero values with framework
// defaults. Component construction is deferred to first access. Unless the
// log configuration disables it, the database connection is opened with the
// GORM query logging adapter routing queries, slow queries and errors into
// the log component (category "db").
func New(cfg config.Config) *Application {
	cfg = config.WithDefaults(cfg)
	logComp := newComponent(func() Log { return newDefaultLog(cfg.Log) })

	var dbOpts []db.Option
	if !cfg.Log.Gorm.Disable {
		slow := time.Duration(cfg.Log.Gorm.SlowThresholdMs) * time.Millisecond
		dbOpts = append(dbOpts, db.WithGormLogger(db.NewGormLogger(func() *log.Logger {
			return loggerOf(logComp)
		}, slow)))
	}
	redisComp := newComponent(func() redis.UniversalClient { return newDefaultRedis(cfg.Redis) })
	return &Application{
		cfg:       cfg,
		log:       logComp,
		db:        db.NewComponent(cfg.DB, dbOpts...),
		redisComp: redisComp,
		cacheComp: newComponent(func() cache.Cache { return newDefaultCache(cfg.Cache, redisComp) }),
		mutexComp: newComponent(func() mutex.Mutex { return newDefaultMutex(cfg.Mutex, redisComp) }),
		i18nComp:  newComponent(func() *i18n.MessageSource { return newDefaultI18n(cfg) }),
		mailComp:  newComponent(func() mail.Mailer { return newDefaultMail(cfg, logComp) }),
	}
}

// Config returns the application configuration.
func (a *Application) Config() config.Config { return a.cfg }

// Log returns the logging component, initializing it on first access.
func (a *Application) Log() Log { return a.log.get() }

// Logger returns the logging component under its full API (category
// scoping, target management, profiling). It is the concrete default
// implementation; a foreign component installed via SetLog is wrapped, with
// delivery forwarded to it.
func (a *Application) Logger() *log.Logger { return loggerOf(a.log) }

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
	return i18n.WithSource(ctx, a.i18nComp.get())
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
