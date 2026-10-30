package base

import (
	"context"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/redis/go-redis/v9"

	"github.com/goyii2/framework/cache"
	"github.com/goyii2/framework/db"
	"github.com/goyii2/framework/log"
	"github.com/goyii2/framework/mail"
	"github.com/goyii2/framework/mutex"
	"github.com/goyii2/framework/queue"
	"github.com/goyii2/framework/session"
)

func TestNewAppliesIdentity(t *testing.T) {
	app := New(WithApp("demo", "prod", true))
	if app.Name() != "demo" {
		t.Errorf("Name() = %q, want demo", app.Name())
	}
	if app.Env() != "prod" {
		t.Errorf("Env() = %q, want prod", app.Env())
	}
	if !app.Debug() {
		t.Error("Debug() = false, want true")
	}
}

func TestNewIdentityDefaults(t *testing.T) {
	app := New()
	if app.Name() != "myapp" {
		t.Errorf("default Name() = %q, want myapp", app.Name())
	}
	if app.Env() != "dev" {
		t.Errorf("default Env() = %q, want dev", app.Env())
	}
	if app.Debug() {
		t.Error("default Debug() = true, want false")
	}
	// WithApp keeps defaults for empty values.
	partial := New(WithApp("", "", true))
	if partial.Name() != "myapp" || partial.Env() != "dev" || !partial.Debug() {
		t.Errorf("empty identity values must keep defaults, got %q/%q/%v", partial.Name(), partial.Env(), partial.Debug())
	}
}

// TestNewInstallsFallbackDefaults verifies every component with a fallback
// is eagerly available right after construction and hands out the same
// instance on every access.
func TestNewInstallsFallbackDefaults(t *testing.T) {
	app := New()

	if l := app.Log(); l == nil {
		t.Fatal("Log() = nil after construction")
	} else if app.Log() != l {
		t.Error("repeated Log() returned a different instance")
	}
	if !app.Log().Enabled(slog.LevelInfo) || app.Log().Enabled(slog.LevelDebug) {
		t.Error("default log level must be info (console)")
	}

	if _, ok := app.Cache().(*cache.Memory); !ok {
		t.Errorf("default cache = %T, want *cache.Memory", app.Cache())
	}
	if _, ok := app.Mutex().(*mutex.Process); !ok {
		t.Errorf("default mutex = %T, want *mutex.Process", app.Mutex())
	}
	if _, ok := app.Mailer().(*mail.LogMailer); !ok {
		t.Errorf("default mailer = %T, want *mail.LogMailer", app.Mailer())
	}
	if _, ok := app.Queue().Driver().(*queue.Sync); !ok {
		t.Errorf("default queue driver = %T, want *queue.Sync", app.Queue().Driver())
	}
	if app.Session() == nil {
		t.Fatal("Session() = nil after construction")
	}
	if _, ok := app.Session().Store.(*session.FileStore); !ok {
		t.Errorf("default session backend = %T, want *session.FileStore", app.Session().Store)
	}
	if app.I18n() == nil {
		t.Fatal("I18n() = nil after construction")
	}
	if langs := app.I18n().Languages(); len(langs) != 0 {
		t.Errorf("default i18n source languages = %v, want none", langs)
	}
}

// TestApplicationDBPanicsWhenNotWired verifies the db slot has no fallback:
// access before wiring is a programmer error with a message naming the fix.
func TestApplicationDBPanicsWhenNotWired(t *testing.T) {
	app := New()

	assertPanics(t, "DB", func() { _ = app.DB() }, "SetDB")

	conn, err := db.New(db.Config{Driver: "sqlite", DSN: filepath.Join(t.TempDir(), "app.db")})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	app.SetDB(conn)
	if app.DB() != conn {
		t.Error("SetDB must install the connection DB returns")
	}
	app.SetDB(nil)
	assertPanics(t, "DB after SetDB(nil)", func() { _ = app.DB() }, "SetDB")
}

// TestApplicationRedisPanicsWhenNotWired verifies the redis slot has no
// fallback either.
func TestApplicationRedisPanicsWhenNotWired(t *testing.T) {
	app := New()

	assertPanics(t, "Redis", func() { _ = app.Redis() }, "SetRedis")

	client := redis.NewClient(&redis.Options{Addr: "127.0.0.1:0"})
	t.Cleanup(func() { _ = client.Close() })
	app.SetRedis(client)
	if app.Redis() != client {
		t.Error("SetRedis must install the client Redis returns")
	}
	app.SetRedis(nil)
	assertPanics(t, "Redis after SetRedis(nil)", func() { _ = app.Redis() }, "SetRedis")
}

// assertPanics checks fn panics with a message containing want.
func assertPanics(t *testing.T, what string, fn func(), want string) {
	t.Helper()
	defer func() {
		recovered := recover()
		if recovered == nil {
			t.Fatalf("%s did not panic without wiring", what)
		}
		msg, _ := recovered.(string)
		if !strings.Contains(msg, want) {
			t.Errorf("%s panic message = %q, want it to mention %q", what, msg, want)
		}
	}()
	fn()
}

func TestApplicationLogReplacedByConfiguredLogger(t *testing.T) {
	app := New()

	// A logger built from a configuration replaces the console default and
	// drives the level accordingly.
	l, err := log.New(log.Config{Level: "warn"})
	if err != nil {
		t.Fatalf("log.New: %v", err)
	}
	app.SetLog(l)
	if app.Log() != Log(l) {
		t.Error("SetLog must install the logger Log returns")
	}
	if app.Log().Enabled(slog.LevelInfo) {
		t.Error("configured level warn must disable info records")
	}
	if !app.Log().Enabled(slog.LevelWarn) {
		t.Error("configured level warn must keep warn records enabled")
	}

	// A minimal fake is wrapped, not lost, under the full Logger API.
	fake := &countingLog{}
	app.SetLog(fake)
	wrapped := app.Logger()
	wrapped.Category("web").Info("wrapped call")
	if calls := fake.snapshot(); len(calls) != 1 || calls[0] != "info:wrapped call" {
		t.Errorf("wrapped logger calls = %v, want [info:wrapped call]", calls)
	}

	app.SetLog(nil)
	if _, ok := app.Log().(*log.Logger); !ok {
		t.Error("SetLog(nil) must restore the console default")
	}
	if !app.Log().Enabled(slog.LevelInfo) {
		t.Error("restored console default must log at info")
	}
}

func TestApplicationLoggerReturnsRichComponent(t *testing.T) {
	app := New()

	l := app.Logger()
	if l == nil {
		t.Fatal("Logger() returned nil")
	}
	if app.Log() != Log(l) {
		t.Error("Log() and Logger() must expose the same component instance")
	}
}

func TestApplicationConnectDB(t *testing.T) {
	app := New()
	dsn := filepath.Join(t.TempDir(), "app.db")

	if err := app.ConnectDB(db.Config{Driver: "sqlite", DSN: dsn}, log.GormConfig{}); err != nil {
		t.Fatalf("ConnectDB: %v", err)
	}
	conn := app.DB()
	t.Cleanup(func() { _ = conn.Close() })
	if err := conn.Exec("CREATE TABLE connect_demo (id INTEGER PRIMARY KEY)").Error; err != nil {
		t.Fatalf("create table through the installed connection: %v", err)
	}

	// A failed open keeps the previous state instead of half-installing.
	before := app.DB()
	if err := app.ConnectDB(db.Config{Driver: "sqlite"}, log.GormConfig{}); err == nil {
		t.Fatal("ConnectDB without a DSN must fail")
	}
	if app.DB() != before {
		t.Error("failed ConnectDB must leave the installed connection in place")
	}
}

// TestApplicationConnectDBRoutesGormQueryLogs verifies the connection
// ConnectDB opens carries the GORM adapter routing queries into the log
// component under the db category.
func TestApplicationConnectDBRoutesGormQueryLogs(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "db.log")
	logger, err := log.New(log.Config{
		Level:   "debug",
		Targets: []log.TargetConfig{{Type: "file", Path: logPath}},
	})
	if err != nil {
		t.Fatalf("log.New: %v", err)
	}
	app := New()
	app.SetLog(logger)

	if err := app.ConnectDB(db.Config{Driver: "sqlite", DSN: filepath.Join(t.TempDir(), "gorm.db")}, log.GormConfig{SlowThresholdMs: 200}); err != nil {
		t.Fatalf("ConnectDB: %v", err)
	}
	conn := app.DB()
	t.Cleanup(func() { _ = app.Logger().Close() })
	t.Cleanup(func() { _ = conn.Close() })
	if err := conn.Exec("CREATE TABLE gorm_log_demo (id INTEGER PRIMARY KEY)").Error; err != nil {
		t.Fatalf("create table: %v", err)
	}
	if err := conn.Exec("INSERT INTO gorm_log_demo (id) VALUES (1)").Error; err != nil {
		t.Fatalf("insert: %v", err)
	}

	data, err := readFile(logPath)
	if err != nil {
		t.Fatalf("reading log file: %v", err)
	}
	if !strings.Contains(data, "[db] query") || !strings.Contains(data, "INSERT INTO gorm_log_demo") {
		t.Errorf("queries must be logged under the db category, got:\n%s", data)
	}
}

// TestApplicationConnectDBDisabledAdapterKeepsDbRecordsOut verifies a
// disabled GORM adapter keeps query records away from the log targets.
func TestApplicationConnectDBDisabledAdapterKeepsDbRecordsOut(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "db-off.log")
	logger, err := log.New(log.Config{
		Level:   "debug",
		Targets: []log.TargetConfig{{Type: "file", Path: logPath}},
	})
	if err != nil {
		t.Fatalf("log.New: %v", err)
	}
	app := New()
	app.SetLog(logger)

	if err := app.ConnectDB(db.Config{Driver: "sqlite", DSN: filepath.Join(t.TempDir(), "off.db")}, log.GormConfig{Disable: true}); err != nil {
		t.Fatalf("ConnectDB: %v", err)
	}
	conn := app.DB()
	t.Cleanup(func() { _ = app.Logger().Close() })
	t.Cleanup(func() { _ = conn.Close() })
	if err := conn.Exec("CREATE TABLE gorm_log_off (id INTEGER PRIMARY KEY)").Error; err != nil {
		t.Fatalf("create table: %v", err)
	}

	// With the adapter off nothing reaches the targets: the log file may
	// not even exist; when it does, it must be free of db records.
	data, err := readFile(logPath)
	if err != nil {
		return
	}
	if strings.Contains(data, "[db]") {
		t.Errorf("disabled adapter must keep db records out of the targets, got:\n%s", data)
	}
}

func TestApplicationContextRoundTrip(t *testing.T) {
	app := New()
	ctx := app.WithContext(context.Background())
	if got := FromContext(ctx); got != app {
		t.Error("FromContext did not return the application carried by ctx")
	}
}

func TestFromContextForeignValueFallsBackToDefault(t *testing.T) {
	SetDefault(nil)
	defer SetDefault(nil)

	ctx := context.WithValue(context.Background(), appKey{}, "not an application")
	if got := FromContext(ctx); got != Default() {
		t.Error("FromContext with a foreign context value must fall back to the default instance")
	}
}

func TestFromContextFallsBackToDefault(t *testing.T) {
	SetDefault(nil)
	defer SetDefault(nil)

	d := Default()
	if got := FromContext(context.Background()); got != d {
		t.Error("FromContext without an application in ctx must fall back to the default instance")
	}
}

func TestDefaultIsSingletonAndReplaceable(t *testing.T) {
	SetDefault(nil)
	defer SetDefault(nil)

	first, second := Default(), Default()
	if first != second {
		t.Error("Default() must return the same instance on repeated calls")
	}

	app := New(WithApp("cli", "", false))
	SetDefault(app)
	if Default() != app {
		t.Error("SetDefault must install the given application as the package default")
	}
}

func TestApplicationControllerRegistry(t *testing.T) {
	app := New()

	if err := app.RegisterController("post", struct{}{}); err != nil {
		t.Fatalf("registering controller: %v", err)
	}
	if err := app.RegisterController("post", struct{}{}); err != ErrDuplicateControllerID {
		t.Errorf("duplicate registration error = %v, want ErrDuplicateControllerID", err)
	}
	if err := app.RegisterController("", struct{}{}); err != ErrInvalidControllerID {
		t.Errorf("empty id error = %v, want ErrInvalidControllerID", err)
	}
	if err := app.RegisterController("a/b", struct{}{}); err != ErrInvalidControllerID {
		t.Errorf("id containing '/' error = %v, want ErrInvalidControllerID", err)
	}

	c, ok := app.Controller("post")
	if !ok || c == nil {
		t.Error("Controller(post) did not return the registered controller")
	}
	if _, ok := app.Controller("missing"); ok {
		t.Error("Controller(missing) must report not found")
	}
	if got := app.ControllerIDs(); len(got) != 1 || got[0] != "post" {
		t.Errorf("ControllerIDs() = %v, want [post]", got)
	}
}

// TestApplicationConcurrentReads exercises accessors, context lookup and the
// package default from many goroutines; run with -race. Component
// replacement (Set) belongs to assembly, before traffic, so it stays out.
func TestApplicationConcurrentReads(t *testing.T) {
	app := New(WithApp("demo", "", false))
	SetDefault(app)
	t.Cleanup(func() { SetDefault(nil) })

	ctx := app.WithContext(context.Background())

	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			a := FromContext(ctx)
			_ = a.Name()
			_ = a.Log().Enabled(slog.LevelDebug)
			_ = a.Cache()
			_ = a.Queue()
			_ = Default()
		}()
	}
	wg.Wait()
}
