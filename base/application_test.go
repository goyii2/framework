package base

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/goyii2/framework/config"
	"github.com/goyii2/framework/db"
	"github.com/goyii2/framework/log"
)

func TestNewAssemblesApplication(t *testing.T) {
	app := New(config.Config{App: config.App{Name: "demo"}})
	if got := app.Config().App.Name; got != "demo" {
		t.Errorf("Config().App.Name = %q, want %q", got, "demo")
	}
	if got := app.Config().Log.Level; got != "info" {
		t.Errorf("New must fill framework defaults, Log.Level = %q, want %q", got, "info")
	}
}

func TestApplicationLogLazyDefault(t *testing.T) {
	app := New(config.Config{Log: config.Log{Level: "warn"}})

	first := app.Log()
	if first == nil {
		t.Fatal("Log() returned nil before initialization")
	}
	if app.Log() != first {
		t.Error("repeated Log() returned a different instance")
	}
	if first.Enabled(slog.LevelInfo) {
		t.Error("configured level warn must disable info records")
	}
	if !first.Enabled(slog.LevelWarn) {
		t.Error("configured level warn must keep warn records enabled")
	}
}

func TestApplicationSetLogFake(t *testing.T) {
	app := New(config.Default())

	fake := &countingLog{}
	app.SetLog(fake)
	app.Log().Info("hello", "key", "value")

	calls := fake.snapshot()
	if len(calls) != 1 || calls[0] != "info:hello" {
		t.Fatalf("fake log calls = %v, want [info:hello]", calls)
	}

	app.SetLog(nil)
	if _, ok := app.Log().(*log.Logger); !ok {
		t.Error("SetLog(nil) must restore the default log component")
	}
}

func TestApplicationDBComponent(t *testing.T) {
	dsn := filepath.Join(t.TempDir(), "app.db")
	app := New(config.Config{DB: config.DB{Driver: "sqlite", DSN: dsn}})

	first, err := app.DB()
	if err != nil {
		t.Fatalf("first DB() error = %v", err)
	}
	if err := first.Exec("CREATE TABLE app_demo (id INTEGER PRIMARY KEY)").Error; err != nil {
		t.Fatalf("create table through the application connection: %v", err)
	}
	again, err := app.DB()
	if err != nil {
		t.Fatalf("second DB() error = %v", err)
	}
	if again != first {
		t.Error("repeated DB() returned a different connection")
	}

	replacement, err := db.Connect(config.DB{Driver: "sqlite", DSN: dsn})
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	app.SetDB(replacement)
	if got, _ := app.DB(); got != replacement {
		t.Error("SetDB must replace the connection")
	}

	app.SetDB(nil)
	fresh, err := app.DB()
	if err != nil {
		t.Fatalf("DB() after SetDB(nil) error = %v", err)
	}
	if fresh == replacement {
		t.Error("SetDB(nil) must restore lazy initialization from the configuration")
	}
	var count int64
	if err := fresh.Raw("SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'app_demo'").Scan(&count).Error; err != nil {
		t.Fatalf("querying the re-opened connection: %v", err)
	}
	if count != 1 {
		t.Errorf("table app_demo visible through re-opened connection: count = %d, want 1", count)
	}

	// Release every connection the component held so Windows can remove the
	// temporary database file.
	_ = first.Close()
	_ = replacement.Close()
	_ = fresh.Close()
}

func TestApplicationDBWithoutDSNFailsWithConfigHint(t *testing.T) {
	app := New(config.Config{})
	_, err := app.DB()
	if err == nil {
		t.Fatal("DB() without a DSN returned nil error")
	}
	if !strings.Contains(err.Error(), db.DSNEnvVar) {
		t.Errorf("error %q must name the %s fallback", err, db.DSNEnvVar)
	}
}

func TestApplicationContextRoundTrip(t *testing.T) {
	app := New(config.Default())
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

	app := New(config.Config{App: config.App{Name: "cli"}})
	SetDefault(app)
	if Default() != app {
		t.Error("SetDefault must install the given application as the package default")
	}
}

func TestApplicationLoggerReturnsRichComponent(t *testing.T) {
	app := New(config.Default())

	l := app.Logger()
	if l == nil {
		t.Fatal("Logger() returned nil")
	}
	if app.Log() != l {
		t.Error("Log() and Logger() must expose the same component instance")
	}

	// A foreign minimal implementation is wrapped, not lost: delivery still
	// reaches it under the full API.
	fake := &countingLog{}
	app.SetLog(fake)
	wrapped := app.Logger()
	if wrapped == l {
		t.Error("SetLog must be reflected by Logger()")
	}
	wrapped.Category("web").Info("wrapped call")
	if calls := fake.snapshot(); len(calls) != 1 || calls[0] != "info:wrapped call" {
		t.Errorf("wrapped logger calls = %v, want [info:wrapped call]", calls)
	}
}

func TestApplicationGormQueryLoggingEnabledByDefault(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "db.log")
	app := New(config.Config{
		DB: config.DB{Driver: "sqlite", DSN: filepath.Join(t.TempDir(), "app.db")},
		Log: config.Log{
			Level:   "debug",
			Targets: []config.LogTarget{{Type: "file", Path: logPath}},
		},
	})

	conn, err := app.DB()
	if err != nil {
		t.Fatalf("DB(): %v", err)
	}
	// Release the log file before the temp dir cleanup so Windows can
	// remove it; registered first, runs last.
	t.Cleanup(func() { _ = app.Logger().Close() })
	t.Cleanup(func() { _ = conn.Close() })
	if err := conn.Exec("CREATE TABLE gorm_log_demo (id INTEGER PRIMARY KEY)").Error; err != nil {
		t.Fatalf("create table: %v", err)
	}
	if err := conn.Exec("INSERT INTO gorm_log_demo (id) VALUES (1)").Error; err != nil {
		t.Fatalf("insert: %v", err)
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("reading log file: %v", err)
	}
	if !strings.Contains(string(data), "[db] query") || !strings.Contains(string(data), "INSERT INTO gorm_log_demo") {
		t.Errorf("queries must be logged under the db category, got:\n%s", data)
	}
}

func TestApplicationGormQueryLoggingDisabled(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "db-off.log")
	app := New(config.Config{
		DB: config.DB{Driver: "sqlite", DSN: filepath.Join(t.TempDir(), "off.db")},
		Log: config.Log{
			Level:   "debug",
			Targets: []config.LogTarget{{Type: "file", Path: logPath}},
			Gorm:    config.GormLog{Disable: true},
		},
	})

	conn, err := app.DB()
	if err != nil {
		t.Fatalf("DB(): %v", err)
	}
	t.Cleanup(func() { _ = app.Logger().Close() })
	t.Cleanup(func() { _ = conn.Close() })
	if err := conn.Exec("CREATE TABLE gorm_log_off (id INTEGER PRIMARY KEY)").Error; err != nil {
		t.Fatalf("create table: %v", err)
	}

	// With the adapter off nothing reaches the targets: the log file may
	// not even exist; when it does, it must be free of db records.
	data, err := os.ReadFile(logPath)
	if os.IsNotExist(err) {
		return
	}
	if err != nil {
		t.Fatalf("reading log file: %v", err)
	}
	if strings.Contains(string(data), "[db]") {
		t.Errorf("disabled adapter must keep db records out of the targets, got:\n%s", data)
	}
}

func TestApplicationControllerRegistry(t *testing.T) {
	app := New(config.Default())

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

// TestApplicationConcurrentUse exercises lazy initialization, component
// replacement, context lookup and the package default from many goroutines;
// run with -race.
func TestApplicationConcurrentUse(t *testing.T) {
	app := New(config.Default())
	SetDefault(app)
	t.Cleanup(func() { SetDefault(nil) })

	ctx := app.WithContext(context.Background())

	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			a := FromContext(ctx)
			_ = a.Config().Env
			_ = a.Log().Enabled(slog.LevelDebug)
			if i%4 == 0 {
				a.SetLog(&countingLog{})
			}
			_ = Default()
		}(i)
	}
	wg.Wait()
}
