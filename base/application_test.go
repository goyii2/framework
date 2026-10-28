package base

import (
	"context"
	"log/slog"
	"sync"
	"testing"

	"github.com/goyii2/framework/config"
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
	if _, ok := app.Log().(*defaultLog); !ok {
		t.Error("SetLog(nil) must restore the default log component")
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

	if Default() != Default() {
		t.Error("Default() must return the same instance on repeated calls")
	}

	app := New(config.Config{App: config.App{Name: "cli"}})
	SetDefault(app)
	if Default() != app {
		t.Error("SetDefault must install the given application as the package default")
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
