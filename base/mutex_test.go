package base

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"

	"github.com/goyii2/framework/config"
	"github.com/goyii2/framework/mutex"
)

// TestApplicationMutexComponent verifies the component semantics: built
// lazily on first access, handed out unchanged afterwards, replaceable, and
// reset by a nil replacement.
func TestApplicationMutexComponent(t *testing.T) {
	app := New(config.Config{})

	if app.mutexComp.val.Load() != nil {
		t.Fatal("the mutex component must not initialize before first use")
	}

	m := app.Mutex()
	if m == nil {
		t.Fatal("Mutex() returned nil")
	}
	if app.Mutex() != m {
		t.Error("repeated Mutex() returned a different instance")
	}

	fake := mutex.NewProcess()
	app.SetMutex(fake)
	if app.Mutex() != fake {
		t.Error("SetMutex must replace what Mutex returns")
	}

	app.SetMutex(nil)
	if app.Mutex() == fake {
		t.Error("SetMutex(nil) must restore lazy initialization from the configuration")
	}
}

// TestApplicationMutexDefaultProcessZeroRedisOverhead verifies the default
// driver is the process mutex and that it never touches the redis
// component: an application that only ever locks in-process keeps its zero
// redis overhead.
func TestApplicationMutexDefaultProcessZeroRedisOverhead(t *testing.T) {
	app := New(config.Config{})

	if _, ok := app.Mutex().(*mutex.Process); !ok {
		t.Fatalf("default mutex driver is %T, want *mutex.Process", app.Mutex())
	}
	if app.redisComp.val.Load() != nil {
		t.Error("the process mutex must not initialize the redis component")
	}
}

// TestApplicationMutexRedisUsesSharedConnection verifies the redis driver
// draws the application's shared redis connection (the top-level section,
// not one of its own) and that its locks are visible on that server.
func TestApplicationMutexRedisUsesSharedConnection(t *testing.T) {
	mr := miniredis.RunT(t)
	app := New(config.Config{
		Redis: config.Redis{Addr: mr.Addr()},
		Mutex: config.Mutex{Driver: "redis"},
	})

	ctx := context.Background()
	release, err := app.Mutex().Lock(ctx, "job", time.Minute)
	if err != nil {
		t.Fatalf("Lock through the mutex component: %v", err)
	}
	if !mr.Exists("goyii:mutex:job") {
		t.Fatal("lock key missing on the configured server")
	}
	if app.redisComp.val.Load() == nil {
		t.Error("the redis driver must draw the shared redis connection")
	}
	release()
	if mr.Exists("goyii:mutex:job") {
		t.Fatal("release must delete the lock key")
	}
}

// TestApplicationMutexUnknownDriverFailsOnUse verifies a misspelled driver
// surfaces at first acquisition instead of breaking application assembly.
func TestApplicationMutexUnknownDriverFailsOnUse(t *testing.T) {
	app := New(config.Config{Mutex: config.Mutex{Driver: "bogus"}})

	if _, err := app.Mutex().Lock(context.Background(), "job", time.Minute); err == nil || !strings.Contains(err.Error(), "unknown driver") {
		t.Fatalf("Lock through a bad driver: err=%v, want unknown-driver error", err)
	}
}
