package queue

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/goyii2/framework/db"
	"github.com/goyii2/framework/migrate"
)

// newTestDB opens a file-backed sqlite database, applies the queue table
// migration (default table when name is empty) and returns the driver.
func newTestDB(t *testing.T, table string) *DBDriver {
	t.Helper()
	dsn := filepath.Join(t.TempDir(), "queue.db")
	conn, err := db.New(db.Config{Driver: "sqlite", DSN: dsn})
	if err != nil {
		t.Fatalf("connect sqlite: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	applyQueueMigration(t, conn, table)
	return NewDB(conn.DB, table)
}

// applyQueueMigration runs the queue table migration over conn's raw handle.
func applyQueueMigration(t *testing.T, conn *db.Conn, table string) {
	t.Helper()
	sqlDB, err := conn.DB.DB()
	if err != nil {
		t.Fatalf("raw database handle: %v", err)
	}
	dialect, ok := migrate.DialectByName("sqlite")
	if !ok {
		t.Fatal("sqlite dialect missing")
	}
	mctx := migrate.NewMigrationContext(sqlDB, dialect)
	if err := NewMigration(table).Up(context.Background(), mctx); err != nil {
		t.Fatalf("apply queue table migration: %v", err)
	}
}

// dueWaitingCount peeks how many waiting rows are due right now, without
// claiming anything — the test-side observation of the delay/backoff state.
func dueWaitingCount(t *testing.T, d *DBDriver) int64 {
	t.Helper()
	var n int64
	if err := d.gorm.Table(d.table).Where("status = ? AND due_at <= ?", statusWaiting, time.Now().UnixNano()).
		Count(&n).Error; err != nil {
		t.Fatalf("count due rows: %v", err)
	}
	return n
}

// newDBQueue wires a Queue with the counted job over a fresh db driver.
func newDBQueue(t *testing.T) (*Queue, *DBDriver) {
	t.Helper()
	resetSink()
	reg, err := NewRegistry(&countedJob{})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	d := newTestDB(t, "")
	return NewWithDriver(d, reg), d
}

// mustPush pushes job through q, failing the test on error.
func mustPush(t *testing.T, q *Queue, job Job, opts ...PushOption) int64 {
	t.Helper()
	id, err := q.Push(context.Background(), job, opts...)
	if err != nil {
		t.Fatalf("Push: %v", err)
	}
	return id
}

// waitFor repeatedly calls probe until it returns true or the deadline
// passes; it reports the last value.
func waitFor(what string, deadline time.Duration, probe func() bool) bool {
	end := time.Now().Add(deadline)
	for time.Now().Before(end) {
		if probe() {
			return true
		}
		time.Sleep(2 * time.Millisecond)
	}
	return probe()
}

// TestDBMigrationCreatesUsableTable verifies the shipped migration creates
// the table (default name and a custom one) the driver actually works over.
func TestDBMigrationCreatesUsableTable(t *testing.T) {
	for _, table := range []string{"", "custom_queue"} {
		name := table
		if name == "" {
			name = DefaultTableName
		}
		t.Run(name, func(t *testing.T) {
			d := newTestDB(t, table)
			ctx := context.Background()

			m := &Message{Name: "counted", Payload: []byte(`{}`), DueAt: time.Now(), TTR: time.Minute}
			if err := d.Push(ctx, m); err != nil || m.ID == 0 {
				t.Fatalf("Push over migrated table: id=%d err=%v", m.ID, err)
			}
			got, err := d.Reserve(ctx)
			if err != nil || got == nil || got.ID != m.ID {
				t.Fatalf("Reserve: %v %v", got, err)
			}
			if err := d.Complete(ctx, got); err != nil {
				t.Fatalf("Complete: %v", err)
			}
		})
	}
}

// TestDBPushReserveComplete verifies the basic lifecycle: push assigns an id,
// reserve claims exactly one due message, complete removes it.
func TestDBPushReserveComplete(t *testing.T) {
	d := newTestDB(t, "")
	ctx := context.Background()
	now := time.Now()

	m1 := &Message{Name: "counted", Payload: []byte(`{"value":"a"}`), DueAt: now, TTR: time.Minute}
	if err := d.Push(ctx, m1); err != nil {
		t.Fatalf("push: %v", err)
	}
	m2 := &Message{Name: "counted", Payload: []byte(`{"value":"b"}`), DueAt: now, TTR: time.Minute}
	if err := d.Push(ctx, m2); err != nil {
		t.Fatalf("push: %v", err)
	}
	if m1.ID == 0 || m2.ID == 0 || m1.ID == m2.ID {
		t.Fatalf("ids = %d, %d; want distinct non-zero", m1.ID, m2.ID)
	}

	first, err := d.Reserve(ctx)
	if err != nil || first == nil {
		t.Fatalf("Reserve: %v %v", first, err)
	}
	if first.Name != "counted" || string(first.Payload) == "" {
		t.Errorf("reserved message = %+v", first)
	}
	// While first is held, the other due message is still claimable — and
	// only that one: the held reservation is invisible.
	second, err := d.Reserve(ctx)
	if err != nil || second == nil {
		t.Fatalf("second Reserve: %v %v", second, err)
	}
	if second.ID == first.ID {
		t.Error("the held reservation was handed out again")
	}
	if third, err := d.Reserve(ctx); err != nil || third != nil {
		t.Fatalf("third Reserve with both held: %v %v — both reservations must be invisible", third, err)
	}
	if err := d.Complete(ctx, first); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if err := d.Complete(ctx, second); err != nil {
		t.Fatalf("Complete second: %v", err)
	}
	if m, err := d.Reserve(ctx); err != nil || m != nil {
		t.Fatalf("Reserve after completes: %v %v, want nil", m, err)
	}
}

// Timing margins: this suite runs against real wall-clock polling, so every
// delay/TTR/backoff below is at least 400ms — comfortably above sqlite write
// latency on slow disks (fsync spikes over 200ms have been observed) — and
// every wait deadline is several multiples of the margin.

// TestDBDelayHoldsUntilDue verifies a delayed push is not reservable before
// its due time and becomes reservable after.
func TestDBDelayHoldsUntilDue(t *testing.T) {
	d := newTestDB(t, "")
	ctx := context.Background()

	if err := d.Push(ctx, &Message{Name: "counted", Payload: []byte(`{}`), DueAt: time.Now().Add(400 * time.Millisecond), TTR: time.Minute}); err != nil {
		t.Fatalf("Push: %v", err)
	}
	if m, err := d.Reserve(ctx); err != nil || m != nil {
		t.Fatalf("Reserve before due: %v %v — the delayed message must not be claimable", m, err)
	}
	if !waitFor("delayed message becomes due", 3*time.Second, func() bool {
		m, err := d.Reserve(ctx)
		return err == nil && m != nil
	}) {
		t.Fatal("delayed message never became due")
	}
}

// TestDBPriorityOrdersReserve verifies due messages reserve highest priority
// first, then earliest due.
func TestDBPriorityOrdersReserve(t *testing.T) {
	d := newTestDB(t, "")
	ctx := context.Background()
	now := time.Now()

	for _, p := range []int{1, 10, 5} {
		if err := d.Push(ctx, &Message{Name: "counted", Payload: []byte(`{}`), Priority: p, DueAt: now, TTR: time.Minute}); err != nil {
			t.Fatalf("Push: %v", err)
		}
	}
	var order []int
	for i := 0; i < 3; i++ {
		m, err := d.Reserve(ctx)
		if err != nil || m == nil {
			t.Fatalf("Reserve %d: %v %v", i, m, err)
		}
		order = append(order, m.Priority)
	}
	if fmt.Sprint(order) != "[10 5 1]" {
		t.Errorf("reserve order = %v, want [10 5 1]", order)
	}
}

// TestDBReserveIsExclusiveAcrossWorkers verifies two drivers over one table
// (the two-process view) hand every message to exactly one of them.
func TestDBReserveIsExclusiveAcrossWorkers(t *testing.T) {
	dsn := filepath.Join(t.TempDir(), "queue.db")
	conn, err := db.New(db.Config{Driver: "sqlite", DSN: dsn})
	if err != nil {
		t.Fatalf("connect sqlite: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	applyQueueMigration(t, conn, "")

	a, b := NewDB(conn.DB, ""), NewDB(conn.DB, "")
	ctx := context.Background()
	for i := 0; i < 20; i++ {
		if err := a.Push(ctx, &Message{Name: "counted", Payload: []byte(`{}`), DueAt: time.Now(), TTR: time.Minute}); err != nil {
			t.Fatalf("Push: %v", err)
		}
	}

	// Workers claim concurrently; every message must be claimed once.
	var (
		wg      sync.WaitGroup
		claimed = make(chan int64, 64)
	)
	for _, w := range []*DBDriver{a, b, a, b} {
		w := w
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				m, err := w.Reserve(ctx)
				if err != nil {
					t.Errorf("Reserve: %v", err)
					return
				}
				if m == nil {
					return
				}
				claimed <- m.ID
			}
		}()
	}
	wg.Wait()
	close(claimed)

	seen := make(map[int64]int)
	for id := range claimed {
		seen[id]++
	}
	if len(seen) != 20 {
		t.Fatalf("%d distinct messages claimed, want 20", len(seen))
	}
	for id, n := range seen {
		if n != 1 {
			t.Errorf("message %d claimed %d times, want exactly 1", id, n)
		}
	}
}

// TestDBEndToEndRetryAndBackoff drives the queue over the db driver: a job
// failing once requeues with the custom backoff delay, is not due before the
// delay elapses, then succeeds and completes.
func TestDBEndToEndRetryAndBackoff(t *testing.T) {
	q, d := newDBQueue(t)
	ctx := context.Background()

	mustPush(t, q, &countedJob{Failures: 1},
		WithAttempts(2),
		WithBackoff(func(int) time.Duration { return 400 * time.Millisecond }),
		WithTTR(time.Minute),
	)

	n, err := q.Run(ctx)
	if err != nil || n != 1 {
		t.Fatalf("Run 1 = %d, %v; want 1", n, err)
	}
	if sink.count() != 1 {
		t.Fatalf("runs = %d, want 1", sink.count())
	}
	if got := dueWaitingCount(t, d); got != 0 {
		t.Fatalf("due rows right after failure = %d, want 0 (backoff window)", got)
	}
	if !waitFor("retry becomes due", 3*time.Second, func() bool {
		return dueWaitingCount(t, d) == 1
	}) {
		t.Fatal("retry never became due after its backoff delay")
	}
	if _, err := q.Run(ctx); err != nil {
		t.Fatalf("Run 2: %v", err)
	}
	if sink.count() != 2 {
		t.Errorf("runs = %d, want 2", sink.count())
	}
	if got := dueWaitingCount(t, d); got != 0 {
		t.Errorf("due rows after completion = %d, want 0", got)
	}
}

// TestDBAttemptsExhaustedKeptFailedAndRetryFailed verifies the terminal
// failed state survives (kept rows), RetryFailed re-pushes with the attempt
// count reset, and the re-pushed message starts a fresh attempt cycle.
func TestDBAttemptsExhaustedKeptFailedAndRetryFailed(t *testing.T) {
	q, d := newDBQueue(t)
	ctx := context.Background()

	mustPush(t, q, &countedJob{Failures: 99}, WithAttempts(2), WithBackoff(func(int) time.Duration { return 400 * time.Millisecond }))

	if _, err := q.Run(ctx); err != nil {
		t.Fatalf("Run 1: %v", err)
	}
	if !waitFor("retry due", 3*time.Second, func() bool {
		return dueWaitingCount(t, d) == 1
	}) {
		t.Fatal("retry never became due")
	}
	if _, err := q.Run(ctx); err != nil {
		t.Fatalf("Run 2: %v", err)
	}
	if sink.count() != 2 {
		t.Fatalf("runs = %d, want 2 (attempts exhausted)", sink.count())
	}

	// The failed message is kept but not runnable.
	if m, err := d.Reserve(ctx); err != nil || m != nil {
		t.Fatalf("Reserve of failed message: %v %v, want nil (kept but not runnable)", m, err)
	}
	n, err := q.RetryFailed(ctx)
	if err != nil || n != 1 {
		t.Fatalf("RetryFailed = %d, %v; want 1", n, err)
	}
	if sink.count() != 2 {
		t.Errorf("runs after RetryFailed = %d, want still 2 (retry only requeues)", sink.count())
	}
	// The re-pushed cycle begins fresh: first failure requeues for a retry.
	if _, err := q.Run(ctx); err != nil {
		t.Fatalf("Run 3: %v", err)
	}
	if sink.count() != 3 {
		t.Errorf("runs after re-push = %d, want 3", sink.count())
	}
	if !waitFor("re-pushed retry due", 3*time.Second, func() bool {
		return dueWaitingCount(t, d) == 1
	}) {
		t.Fatal("re-pushed retry never became due")
	}
	m, err := d.Reserve(ctx)
	if err != nil || m == nil {
		t.Fatalf("Reserve of requeued retry: %v %v", m, err)
	}
	if m.Attempt != 1 {
		t.Errorf("requeued attempt = %d, want 1 (attempt count reset by RetryFailed)", m.Attempt)
	}
}

// TestDBTTRReclaimRequeuesReservation verifies a reservation whose TTR
// elapsed returns to waiting by itself — the crashed-worker recovery path.
func TestDBTTRReclaimRequeuesReservation(t *testing.T) {
	d := newTestDB(t, "")
	ctx := context.Background()

	if err := d.Push(ctx, &Message{Name: "counted", Payload: []byte(`{}`), DueAt: time.Now(), TTR: 400 * time.Millisecond}); err != nil {
		t.Fatalf("Push: %v", err)
	}
	first, err := d.Reserve(ctx)
	if err != nil || first == nil {
		t.Fatalf("Reserve: %v %v", first, err)
	}
	// Held reservation, no Complete: nothing is claimable while the TTR runs.
	if m, err := d.Reserve(ctx); err != nil || m != nil {
		t.Fatalf("Reserve while reservation alive: %v %v", m, err)
	}
	// Past the TTR the reservation returns to waiting and is claimable again.
	// The probe holds the reclaimed reservation, so capture it there.
	var reclaimed *Message
	if !waitFor("reservation reclaimed", 3*time.Second, func() bool {
		m, err := d.Reserve(ctx)
		if err == nil && m != nil {
			reclaimed = m
			return true
		}
		return false
	}) {
		t.Fatal("expired reservation was never reclaimed")
	}
	if reclaimed.ID != first.ID {
		t.Fatalf("reserved id %d after reclaim, want the reclaimed %d", reclaimed.ID, first.ID)
	}
	if reclaimed.Attempt != first.Attempt {
		t.Errorf("attempt changed across reclaim: %d -> %d", first.Attempt, reclaimed.Attempt)
	}
}

// TestDBRunThroughQueueDeliversPayload verifies the payload round-trips
// through the row and reaches the rebuilt job.
func TestDBRunThroughQueueDeliversPayload(t *testing.T) {
	q, _ := newDBQueue(t)
	ctx := context.Background()

	mustPush(t, q, &countedJob{Value: "db", N: 7})
	if n, err := q.Run(ctx); err != nil || n != 1 {
		t.Fatalf("Run = %d, %v; want 1", n, err)
	}
	runs := sink.recorded()
	if len(runs) != 1 || runs[0].Value != "db" || runs[0].N != 7 {
		t.Errorf("executed with payload %+v, want one run {db 7}", runs)
	}
}

// TestDBCustomTableNameIsolation verifies two drivers with different table
// names on one database keep independent queues.
func TestDBCustomTableNameIsolation(t *testing.T) {
	dsn := filepath.Join(t.TempDir(), "queue.db")
	conn, err := db.New(db.Config{Driver: "sqlite", DSN: dsn})
	if err != nil {
		t.Fatalf("connect sqlite: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	applyQueueMigration(t, conn, "queue_a")
	applyQueueMigration(t, conn, "queue_b")

	a, b := NewDB(conn.DB, "queue_a"), NewDB(conn.DB, "queue_b")
	ctx := context.Background()
	if err := a.Push(ctx, &Message{Name: "counted", Payload: []byte(`{}`), DueAt: time.Now(), TTR: time.Minute}); err != nil {
		t.Fatalf("Push a: %v", err)
	}
	if m, err := b.Reserve(ctx); err != nil || m != nil {
		t.Fatalf("Reserve b sees %v %v; want nil (different table)", m, err)
	}
	if m, err := a.Reserve(ctx); err != nil || m == nil {
		t.Fatalf("Reserve a: %v %v", m, err)
	}
}

// TestDBConcurrentQueuePush verifies concurrent pushes through one Queue stay
// safe and every message lands and drains.
func TestDBConcurrentQueuePush(t *testing.T) {
	q, _ := newDBQueue(t)
	ctx := context.Background()

	const writers, each = 8, 10
	var wg sync.WaitGroup
	var pushed atomic.Int64
	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < each; i++ {
				if _, err := q.Push(ctx, &countedJob{}); err != nil {
					t.Errorf("Push: %v", err)
					return
				}
				pushed.Add(1)
			}
		}()
	}
	wg.Wait()

	n, err := q.Run(ctx)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if int64(n) != pushed.Load() {
		t.Errorf("Run drained %d, pushed %d", n, pushed.Load())
	}
}
