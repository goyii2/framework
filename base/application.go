package base

import (
	"context"
	"sync/atomic"

	"github.com/goyii2/framework/config"
)

// Application is the process-level component container. Components are
// strong-typed fields behind accessors: each is lazily initialized on first
// access (concurrency-safe) and replaceable via its Set method for tests or
// alternative implementations. Request-scoped code receives the application
// through the context (WithContext / FromContext); code without a request
// context uses the package-level Default instance.
type Application struct {
	cfg config.Config
	log *component[Log]
}

// New assembles an Application from cfg, filling zero values with framework
// defaults. Component construction is deferred to first access.
func New(cfg config.Config) *Application {
	cfg = config.WithDefaults(cfg)
	return &Application{
		cfg: cfg,
		log: newComponent(func() Log { return newDefaultLog(cfg.Log) }),
	}
}

// Config returns the application configuration.
func (a *Application) Config() config.Config { return a.cfg }

// Log returns the logging component, initializing it on first access.
func (a *Application) Log() Log { return a.log.get() }

// SetLog replaces the logging component (e.g. a test fake or a richer
// implementation); nil restores lazy initialization of the default.
func (a *Application) SetLog(l Log) { a.log.set(l) }

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
