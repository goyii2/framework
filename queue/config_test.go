package queue

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/goyii2/framework/db"
)

// newConfigDB opens a file-backed sqlite database with the queue table
// migration applied — the connection the db driver tests inject.
func newConfigDB(t *testing.T) *db.Conn {
	t.Helper()
	conn, err := db.New(db.Config{Driver: "sqlite", DSN: filepath.Join(t.TempDir(), "queue.db")})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	applyQueueMigration(t, conn, "")
	return conn
}

// TestNewZeroConfigIsSync verifies the zero-value Config builds the sync
// queue: inline execution, an empty registry and the one-second default poll.
func TestNewZeroConfigIsSync(t *testing.T) {
	q, err := New(Config{})
	if err != nil {
		t.Fatalf("New(zero Config) error = %v", err)
	}
	if _, ok := q.Driver().(*Sync); !ok {
		t.Fatalf("zero Config driver is %T, want *Sync", q.Driver())
	}
	if q.Jobs() == nil {
		t.Error("Jobs() returned nil; a registry must always exist")
	}
	if q.poll != time.Second {
		t.Errorf("zero Config poll = %v, want 1s", q.poll)
	}
}

// TestNewDispatch verifies the three drivers resolve from Config, driver
// spellings tolerate case and whitespace, and the db table name defaults.
func TestNewDispatch(t *testing.T) {
	conn := newConfigDB(t)

	sync, err := New(Config{Driver: "  Sync "})
	if err != nil {
		t.Fatalf("New(sync) error = %v", err)
	}
	if _, ok := sync.Driver().(*Sync); !ok {
		t.Fatalf("sync driver is %T, want *Sync", sync.Driver())
	}

	dbq, err := New(Config{Driver: "db"}, WithDB(conn))
	if err != nil {
		t.Fatalf("New(db) error = %v", err)
	}
	drv, ok := dbq.Driver().(*DBDriver)
	if !ok {
		t.Fatalf("db driver is %T, want *DBDriver", dbq.Driver())
	}
	if drv.Table() != "queue" {
		t.Errorf("default table = %q, want queue", drv.Table())
	}

	named, err := New(Config{Driver: "db", TableName: "jobs"}, WithDBFunc(func() (*db.Conn, error) {
		return conn, nil
	}))
	if err != nil {
		t.Fatalf("New(db, jobs) error = %v", err)
	}
	if drv := named.Driver().(*DBDriver); drv.Table() != "jobs" {
		t.Errorf("named table = %q, want jobs", drv.Table())
	}

	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	rq, err := New(Config{Driver: "redis"}, WithRedis(client))
	if err != nil {
		t.Fatalf("New(redis) error = %v", err)
	}
	if _, ok := rq.Driver().(*RedisDriver); !ok {
		t.Fatalf("redis driver is %T, want *RedisDriver", rq.Driver())
	}
}

// TestNewDbDriverPushesAndRuns verifies the db driver built through New
// works end to end: push stores, Run executes from the registry.
func TestNewDbDriverPushesAndRuns(t *testing.T) {
	resetSink()
	conn := newConfigDB(t)
	q, err := New(Config{Driver: "db", PollIntervalMs: 5}, WithDB(conn))
	if err != nil {
		t.Fatalf("New(db) error = %v", err)
	}
	if err := q.Jobs().Register(&countedJob{}); err != nil {
		t.Fatalf("register: %v", err)
	}

	if _, err := q.Push(context.Background(), &countedJob{Value: "x", N: 1}); err != nil {
		t.Fatalf("Push: %v", err)
	}
	n, err := q.Run(context.Background())
	if err != nil || n != 1 {
		t.Fatalf("Run = %d, %v; want 1, nil", n, err)
	}
	if got := sink.count(); got != 1 {
		t.Errorf("job ran %d times, want 1", got)
	}
}

// TestNewPollIntervalFromConfig verifies the listen poll interval follows
// Config's PollIntervalMs and falls back to one second when unset.
func TestNewPollIntervalFromConfig(t *testing.T) {
	q, err := New(Config{PollIntervalMs: 5})
	if err != nil {
		t.Fatalf("New error = %v", err)
	}
	if q.poll != 5*time.Millisecond {
		t.Errorf("poll = %v, want 5ms", q.poll)
	}
	negative, err := New(Config{PollIntervalMs: -1})
	if err != nil {
		t.Fatalf("New error = %v", err)
	}
	if negative.poll != time.Second {
		t.Errorf("poll = %v, want 1s", negative.poll)
	}
}

// TestNewRejectsUnknownDriver verifies a misspelled driver fails construction.
func TestNewRejectsUnknownDriver(t *testing.T) {
	_, err := New(Config{Driver: "bogus"})
	if err == nil || !strings.Contains(err.Error(), "unknown driver") {
		t.Fatalf("New(bogus) err = %v, want unknown-driver error", err)
	}
}

// TestNewDbDriverRequiresConnection verifies the db driver without a
// connection — no option, a nil connection or a failing lazy open — fails
// construction.
func TestNewDbDriverRequiresConnection(t *testing.T) {
	if _, err := New(Config{Driver: "db"}); err == nil ||
		!strings.Contains(err.Error(), "requires the database connection") {
		t.Fatalf("New(db) without WithDB: err = %v, want requires-connection error", err)
	}
	if _, err := New(Config{Driver: "db"}, WithDB(nil)); err == nil {
		t.Fatal("New(db) with a nil connection must fail")
	}
	boom := errors.New("no server")
	if _, err := New(Config{Driver: "db"}, WithDBFunc(func() (*db.Conn, error) { return nil, boom })); err == nil ||
		!errors.Is(err, boom) {
		t.Fatalf("New(db) with a failing open: err = %v, want wrapped boom", err)
	}
}

// TestNewRedisDriverRequiresClient verifies the redis driver without a
// client — no option or a nil client — fails construction.
func TestNewRedisDriverRequiresClient(t *testing.T) {
	if _, err := New(Config{Driver: "redis"}); err == nil ||
		!strings.Contains(err.Error(), "requires a redis client") {
		t.Fatalf("New(redis) without WithRedis: err = %v, want requires-client error", err)
	}
	if _, err := New(Config{Driver: "redis"}, WithRedis(nil)); err == nil {
		t.Fatal("New(redis) with a nil client must fail")
	}
}

// TestNewIgnoresForeignDependencies verifies the sync queue never resolves
// the db or redis dependencies: their lazy providers may fail or be absent —
// a sync application keeps its zero external dependencies.
func TestNewIgnoresForeignDependencies(t *testing.T) {
	mustNotOpen := func() (*db.Conn, error) { return nil, errors.New("must not open a database") }
	mustNotDial := func() redis.UniversalClient { panic("must not resolve redis") }
	if _, err := New(Config{}, WithDBFunc(mustNotOpen), WithRedisFunc(mustNotDial)); err != nil {
		t.Fatalf("New(sync) with foreign dependencies: err = %v", err)
	}
}

// TestNewWithDriverKeepsProgrammaticConstruction verifies the escape hatch
// applies the same options (dispatcher, poll interval) around a custom
// driver.
func TestNewWithDriverKeepsProgrammaticConstruction(t *testing.T) {
	log := &eventLog{}
	q := NewWithDriver(NewSync(), nil, WithDispatcher(log.dispatch), WithPollInterval(time.Millisecond))
	if q.poll != time.Millisecond {
		t.Errorf("poll = %v, want 1ms", q.poll)
	}
	if q.events == nil {
		t.Error("WithDispatcher must install the dispatcher")
	}
}
