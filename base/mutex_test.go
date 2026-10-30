package base

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/goyii2/framework/mutex"
)

// TestApplicationMutexDefaultIsProcess verifies the default mutex component
// is the process driver, replaceable and restored by a nil replacement.
func TestApplicationMutexDefaultIsProcess(t *testing.T) {
	app := New()

	if _, ok := app.Mutex().(*mutex.Process); !ok {
		t.Fatalf("default mutex driver is %T, want *mutex.Process", app.Mutex())
	}
	if app.Mutex() != app.Mutex() {
		t.Error("repeated Mutex() returned a different instance")
	}

	fake := mutex.NewProcess()
	app.SetMutex(fake)
	if app.Mutex() != fake {
		t.Error("SetMutex must replace what Mutex returns")
	}

	app.SetMutex(nil)
	if _, ok := app.Mutex().(*mutex.Process); !ok {
		t.Error("SetMutex(nil) must restore the process fallback")
	}
}

// TestApplicationMutexRedisDrawsSharedConnection verifies the redis mutex the
// application entry installs draws the shared connection and its locks are
// visible on that server.
func TestApplicationMutexRedisDrawsSharedConnection(t *testing.T) {
	mr := miniredis.RunT(t)
	app := New()
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	app.SetRedis(client)

	m, err := mutex.New(mutex.Config{Driver: "redis"}, mutex.WithRedis(app.Redis()))
	if err != nil {
		t.Fatalf("mutex.New(redis): %v", err)
	}
	app.SetMutex(m)

	ctx := context.Background()
	release, err := app.Mutex().Lock(ctx, "job", time.Minute)
	if err != nil {
		t.Fatalf("Lock through the mutex component: %v", err)
	}
	if !mr.Exists("goyii:mutex:job") {
		t.Fatal("lock key missing on the configured server")
	}
	release()
	if mr.Exists("goyii:mutex:job") {
		t.Fatal("release must delete the lock key")
	}
}
