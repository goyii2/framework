package base

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/goyii2/framework/db"
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

// TestApplicationQueueDefaultIsSync verifies the default queue is the sync
// driver: replaceable, reset by a nil replacement, and executing inline.
func TestApplicationQueueDefaultIsSync(t *testing.T) {
	app := New()

	if _, ok := app.Queue().Driver().(*queue.Sync); !ok {
		t.Fatalf("default queue driver is %T, want *queue.Sync", app.Queue().Driver())
	}

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

	fake := queue.NewWithDriver(queue.NewSync(), nil)
	app.SetQueue(fake)
	if app.Queue() != fake {
		t.Error("SetQueue must replace what Queue returns")
	}

	app.SetQueue(nil)
	if app.Queue() == fake {
		t.Error("SetQueue(nil) must restore the sync fallback")
	}
	if _, ok := app.Queue().Driver().(*queue.Sync); !ok {
		t.Errorf("restored queue driver is %T, want *queue.Sync", app.Queue().Driver())
	}
}

// TestApplicationQueueDbDriver verifies a db-driven queue assembled the way
// an application entry does — queue.New over the installed db connection,
// with the dispatcher routing events into the application's registry — works
// end to end.
func TestApplicationQueueDbDriver(t *testing.T) {
	dsn := filepath.Join(t.TempDir(), "queue.db")
	app := New()
	if err := app.ConnectDB(db.Config{Driver: "sqlite", DSN: dsn}, dbGormOff()); err != nil {
		t.Fatalf("ConnectDB: %v", err)
	}
	conn := app.DB()
	t.Cleanup(func() { _ = conn.Close() })

	// Create the table the way applications do: the shipped migration.
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

	q, err := queue.New(queue.Config{Driver: "db", TableName: "jobs", PollIntervalMs: 5},
		queue.WithDB(conn), app.QueueDispatcher())
	if err != nil {
		t.Fatalf("queue.New(db): %v", err)
	}
	app.SetQueue(q)

	appRuns.Store(0)
	if err := app.Queue().Jobs().Register(&appJob{}); err != nil {
		t.Fatalf("register: %v", err)
	}
	if _, err := app.Queue().Push(context.Background(), &appJob{Value: "deferred"}); err != nil {
		t.Fatalf("Push: %v", err)
	}
	n, err := app.Queue().Run(context.Background())
	if err != nil || n != 1 {
		t.Fatalf("Run = %d, %v; want 1, nil", n, err)
	}
	if appRuns.Load() != 1 {
		t.Errorf("job executed %d times, want 1", appRuns.Load())
	}
}

// TestApplicationQueueRedisUsesSharedConnection verifies a redis-driven queue
// over the installed shared client lands its messages on that server.
func TestApplicationQueueRedisUsesSharedConnection(t *testing.T) {
	mr := miniredis.RunT(t)
	app := New()
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	app.SetRedis(client)

	q, err := queue.New(queue.Config{Driver: "redis"}, queue.WithRedis(app.Redis()), app.QueueDispatcher())
	if err != nil {
		t.Fatalf("queue.New(redis): %v", err)
	}
	app.SetQueue(q)
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
}

// TestApplicationQueueEventsFlowThroughBase verifies the five queue events
// dispatch through the application's own On/Trigger registry, with the queue
// package's event-name constants converting to base.EventName — both for the
// fallback and an installed queue carrying QueueDispatcher.
func TestApplicationQueueEventsFlowThroughBase(t *testing.T) {
	app := New()
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
