package base

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/alicebob/miniredis/v2"

	"github.com/goyii2/framework/config"
	"github.com/goyii2/framework/migrate"
	"github.com/goyii2/framework/queue"
)

// appJob is the job the base tests push through the application queue. Its
// observations live in package-level counters because the worker rebuilds a
// fresh instance from the payload.
type appJob struct {
	Value string `json:"value"`
}

var appRuns atomic.Int32

func (*appJob) Name() string { return "app" }

func (*appJob) Execute(context.Context) error {
	appRuns.Add(1)
	return nil
}

// TestApplicationQueueComponent verifies the component semantics: built
// lazily on first access, handed out unchanged afterwards, replaceable, and
// reset by a nil replacement.
func TestApplicationQueueComponent(t *testing.T) {
	app := New(config.Config{})

	if app.queueComp.val.Load() != nil {
		t.Fatal("the queue component must not initialize before first use")
	}

	q := app.Queue()
	if q == nil {
		t.Fatal("Queue() returned nil")
	}
	if app.Queue() != q {
		t.Error("repeated Queue() returned a different instance")
	}

	fake := queue.NewWithDriver(queue.NewSync(), nil)
	app.SetQueue(fake)
	if app.Queue() != fake {
		t.Error("SetQueue must replace what Queue returns")
	}

	app.SetQueue(nil)
	if app.Queue() == fake {
		t.Error("SetQueue(nil) must restore lazy initialization from the configuration")
	}
}

// TestApplicationQueueDefaultSyncZeroOverhead verifies the default driver is
// sync and touches neither the database nor redis: an application that never
// configures queue keeps its zero external dependencies.
func TestApplicationQueueDefaultSyncZeroOverhead(t *testing.T) {
	app := New(config.Config{})
	appRuns.Store(0)
	if err := app.Queue().Jobs().Register(&appJob{}); err != nil {
		t.Fatalf("register: %v", err)
	}

	if _, err := app.Queue().Push(context.Background(), &appJob{Value: "x"}); err != nil {
		t.Fatalf("Push through the sync default: %v", err)
	}
	if appRuns.Load() != 1 {
		t.Fatalf("sync push executed %d times, want 1", appRuns.Load())
	}
	if _, ok := app.Queue().Driver().(*queue.Sync); !ok {
		t.Fatalf("default queue driver is %T, want *queue.Sync", app.Queue().Driver())
	}
	if app.redisComp.val.Load() != nil {
		t.Error("the sync queue must not initialize the redis component")
	}
}

// TestApplicationQueueDbDriver verifies the db driver draws the application's
// database connection and works end to end: push, worker drain, registry
// rebuild from the application's registration.
func TestApplicationQueueDbDriver(t *testing.T) {
	dsn := filepath.Join(t.TempDir(), "queue.db")
	app := New(config.Config{
		DB:    config.DB{Driver: "sqlite", DSN: dsn},
		Queue: config.Queue{Driver: "db", TableName: "jobs", PollIntervalMs: 5},
	})

	// Create the table the way applications do: the shipped migration.
	conn, err := app.DB()
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() }) // release the file before TempDir cleanup
	sqlDB, err := conn.DB.DB()
	if err != nil {
		t.Fatalf("raw handle: %v", err)
	}
	dialect, ok := migrate.DialectByName("sqlite")
	if !ok {
		t.Fatal("sqlite dialect missing")
	}
	if err := queue.NewMigration("jobs").Up(context.Background(), migrate.NewMigrationContext(sqlDB, dialect)); err != nil {
		t.Fatalf("apply queue migration: %v", err)
	}

	appRuns.Store(0)
	if err := app.Queue().Jobs().Register(&appJob{}); err != nil {
		t.Fatalf("register: %v", err)
	}
	if _, err := app.Queue().Push(context.Background(), &appJob{Value: "deferred"}); err != nil {
		t.Fatalf("Push: %v", err)
	}
	n, err := app.Queue().Run(context.Background())
	if err != nil || n != 1 {
		t.Fatalf("Run = %d, %v; want 1", n, err)
	}
	if appRuns.Load() != 1 {
		t.Errorf("job executed %d times, want 1", appRuns.Load())
	}
}

// TestApplicationQueueRedisUsesSharedConnection verifies the redis driver
// draws the application's shared redis connection and its messages land
// under the queue prefix on that server.
func TestApplicationQueueRedisUsesSharedConnection(t *testing.T) {
	mr := miniredis.RunT(t)
	app := New(config.Config{
		Redis: config.Redis{Addr: mr.Addr()},
		Queue: config.Queue{Driver: "redis"},
	})
	if err := app.Queue().Jobs().Register(&appJob{}); err != nil {
		t.Fatalf("register: %v", err)
	}

	ctx := context.Background()
	if _, err := app.Queue().Push(ctx, &appJob{Value: "x"}); err != nil {
		t.Fatalf("Push: %v", err)
	}
	found := false
	for _, key := range mr.Keys() {
		if strings.HasPrefix(key, "goyii:queue:") {
			found = true
		}
	}
	if !found {
		t.Fatalf("no message landed under the goyii:queue: prefix (keys: %v)", mr.Keys())
	}
	if app.redisComp.val.Load() == nil {
		t.Error("the redis queue must draw the shared redis connection")
	}
}

// TestApplicationQueueUnknownDriverFailsOnUse verifies a misspelled driver
// surfaces at first use instead of breaking application assembly.
func TestApplicationQueueUnknownDriverFailsOnUse(t *testing.T) {
	app := New(config.Config{Queue: config.Queue{Driver: "bogus"}})

	if _, err := app.Queue().Push(context.Background(), &appJob{}); err == nil || !strings.Contains(err.Error(), "unknown driver") {
		t.Fatalf("Push through a bad driver: err=%v, want unknown-driver error", err)
	}
}

// TestApplicationQueueDbWithoutConnectionFailsOnUse verifies the db driver
// with an unopenable database reports the underlying error at first use.
func TestApplicationQueueDbWithoutConnectionFailsOnUse(t *testing.T) {
	app := New(config.Config{Queue: config.Queue{Driver: "db"}}) // no DSN

	if _, err := app.Queue().Push(context.Background(), &appJob{}); err == nil {
		t.Fatal("Push through the db driver without a database must fail")
	}
}

// TestApplicationQueueEventsFlowThroughBase verifies the five queue events
// dispatch through the application's own On/Trigger registry, with the queue
// package's event-name constants converting to base.EventName.
func TestApplicationQueueEventsFlowThroughBase(t *testing.T) {
	app := New(config.Config{})
	if err := app.Queue().Jobs().Register(&appJob{}); err != nil {
		t.Fatalf("register: %v", err)
	}

	var events []EventName
	off := app.On(queue.EventBeforePush, func(_ context.Context, e Event) {
		events = append(events, e.Name)
	})
	defer off()
	app.On(queue.EventAfterPush, func(_ context.Context, e Event) {
		events = append(events, e.Name)
	})
	app.On(queue.EventBeforeExec, func(_ context.Context, e Event) {
		events = append(events, e.Name)
	})
	app.On(queue.EventAfterExec, func(_ context.Context, e Event) {
		events = append(events, e.Name)
	})

	ctx := app.WithContext(context.Background())
	if _, err := app.Queue().Push(ctx, &appJob{}); err != nil {
		t.Fatalf("Push: %v", err)
	}
	want := []EventName{queue.EventBeforePush, queue.EventBeforeExec, queue.EventAfterExec, queue.EventAfterPush}
	if fmt.Sprint(events) != fmt.Sprint(want) {
		t.Fatalf("events = %v, want %v", events, want)
	}

	// The push events carry the job's push data.
	var got *queue.PushEventData
	app.On(queue.EventAfterPush, func(_ context.Context, e Event) {
		if d, ok := e.Data.(queue.PushEventData); ok {
			got = &d
		}
	})
	if _, err := app.Queue().Push(ctx, &appJob{}); err != nil {
		t.Fatalf("Push 2: %v", err)
	}
	if got == nil || got.Job != "app" {
		t.Errorf("AfterPush data = %+v, want job name app", got)
	}
}
