package queue

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// newTestRedis starts a miniredis with a redis queue driver bound to it.
func newTestRedis(t *testing.T) (*RedisDriver, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return NewRedis(client, ""), mr
}

// newRedisQueue wires a Queue with the counted job over a fresh miniredis.
func newRedisQueue(t *testing.T) (*Queue, *RedisDriver) {
	t.Helper()
	resetSink()
	reg, err := NewRegistry(&countedJob{})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	d, _ := newTestRedis(t)
	return NewWithDriver(d, reg), d
}

// dueCount reports how many waiting messages are due right now, across all
// priorities — the test-side observation of the delay/backoff state.
func (d *RedisDriver) dueCount(t *testing.T) int64 {
	t.Helper()
	ctx := context.Background()
	prios, err := d.client.SMembers(ctx, d.prefix+"prios").Result()
	if err != nil {
		t.Fatalf("SMEMBERS prios: %v", err)
	}
	var n int64
	for _, p := range prios {
		c, err := d.client.ZCount(ctx, d.prefix+"waiting:"+p, "-inf", fmt.Sprintf("%d", time.Now().UnixMilli())).Result()
		if err != nil {
			t.Fatalf("ZCOUNT waiting:%s: %v", p, err)
		}
		n += c
	}
	return n
}

// TestRedisPushReserveComplete verifies the lifecycle: push assigns an id,
// reserve claims exactly one due message, complete removes every trace.
func TestRedisPushReserveComplete(t *testing.T) {
	d, mr := newTestRedis(t)
	ctx := context.Background()

	m1 := &Message{Name: "counted", Payload: []byte(`{"value":"a"}`), DueAt: time.Now(), TTR: time.Minute}
	if err := d.Push(ctx, m1); err != nil {
		t.Fatalf("Push: %v", err)
	}
	m2 := &Message{Name: "counted", Payload: []byte(`{"value":"b"}`), DueAt: time.Now(), TTR: time.Minute}
	if err := d.Push(ctx, m2); err != nil {
		t.Fatalf("Push: %v", err)
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
	second, err := d.Reserve(ctx)
	if err != nil || second == nil || second.ID == first.ID {
		t.Fatalf("second Reserve: %v %v — must be the other due message", second, err)
	}
	if third, err := d.Reserve(ctx); err != nil || third != nil {
		t.Fatalf("third Reserve with both held: %v %v", third, err)
	}
	if err := d.Complete(ctx, first); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if mr.Exists(d.prefix + msgKey(1)) {
		t.Error("complete must delete the message payload")
	}
	if third, err := d.Reserve(ctx); err != nil || third != nil {
		t.Fatalf("Reserve after complete: %v %v, want nil (second is still held)", third, err)
	}
}

// TestRedisDelayHoldsUntilDue verifies a delayed push is not reservable
// before its due time and becomes reservable after.
func TestRedisDelayHoldsUntilDue(t *testing.T) {
	d, _ := newTestRedis(t)
	ctx := context.Background()

	if err := d.Push(ctx, &Message{Name: "counted", Payload: []byte(`{}`), DueAt: time.Now().Add(60 * time.Millisecond), TTR: time.Minute}); err != nil {
		t.Fatalf("Push: %v", err)
	}
	if m, err := d.Reserve(ctx); err != nil || m != nil {
		t.Fatalf("Reserve before due: %v %v — the delayed message must not be claimable", m, err)
	}
	if !waitFor("delayed message becomes due", 2*time.Second, func() bool {
		m, err := d.Reserve(ctx)
		return err == nil && m != nil
	}) {
		t.Fatal("delayed message never became due")
	}
}

// TestRedisPriorityOrdersReserve verifies due messages reserve highest
// priority first, then earliest due.
func TestRedisPriorityOrdersReserve(t *testing.T) {
	d, _ := newTestRedis(t)
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

// TestRedisReserveIsExclusiveAcrossWorkers verifies two drivers over one
// server (the two-process view) hand every message to exactly one of them.
func TestRedisReserveIsExclusiveAcrossWorkers(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	a, b := NewRedis(client, ""), NewRedis(client, "")
	ctx := context.Background()
	for i := 0; i < 20; i++ {
		if err := a.Push(ctx, &Message{Name: "counted", Payload: []byte(`{}`), DueAt: time.Now(), TTR: time.Minute}); err != nil {
			t.Fatalf("Push: %v", err)
		}
	}

	var (
		wg      sync.WaitGroup
		claimed = make(chan int64, 64)
	)
	for _, w := range []*RedisDriver{a, b, a, b} {
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

// TestRedisEndToEndRetryAndBackoff drives the queue over the redis driver:
// a failing job requeues with the custom backoff delay, is not due before the
// delay elapses, then succeeds and completes.
func TestRedisEndToEndRetryAndBackoff(t *testing.T) {
	q, d := newRedisQueue(t)
	ctx := context.Background()

	mustPush(t, q, &countedJob{Failures: 1},
		WithAttempts(2),
		WithBackoff(func(int) time.Duration { return 80 * time.Millisecond }),
		WithTTR(time.Minute),
	)

	n, err := q.Run(ctx)
	if err != nil || n != 1 {
		t.Fatalf("Run 1 = %d, %v; want 1", n, err)
	}
	if sink.count() != 1 {
		t.Fatalf("runs = %d, want 1", sink.count())
	}
	if got := d.dueCount(t); got != 0 {
		t.Fatalf("due right after failure = %d, want 0 (backoff window)", got)
	}
	if !waitFor("retry becomes due", 2*time.Second, func() bool {
		return d.dueCount(t) == 1
	}) {
		t.Fatal("retry never became due after its backoff delay")
	}
	if _, err := q.Run(ctx); err != nil {
		t.Fatalf("Run 2: %v", err)
	}
	if sink.count() != 2 {
		t.Errorf("runs = %d, want 2", sink.count())
	}
	if got := d.dueCount(t); got != 0 {
		t.Errorf("due after completion = %d, want 0", got)
	}
}

// TestRedisAttemptsExhaustedKeptFailedAndRetryFailed verifies the terminal
// failed state survives on the server, RetryFailed re-pushes with the attempt
// count reset, and the re-pushed message starts a fresh attempt cycle.
func TestRedisAttemptsExhaustedKeptFailedAndRetryFailed(t *testing.T) {
	q, d := newRedisQueue(t)
	ctx := context.Background()

	mustPush(t, q, &countedJob{Failures: 99}, WithAttempts(2), WithBackoff(func(int) time.Duration { return 20 * time.Millisecond }))

	if _, err := q.Run(ctx); err != nil {
		t.Fatalf("Run 1: %v", err)
	}
	if !waitFor("retry due", 2*time.Second, func() bool {
		return d.dueCount(t) == 1
	}) {
		t.Fatal("retry never became due")
	}
	if _, err := q.Run(ctx); err != nil {
		t.Fatalf("Run 2: %v", err)
	}
	if sink.count() != 2 {
		t.Fatalf("runs = %d, want 2 (attempts exhausted)", sink.count())
	}

	// Failed kept: not runnable, but present on the server.
	if m, err := d.Reserve(ctx); err != nil || m != nil {
		t.Fatalf("Reserve of failed message: %v %v, want nil (kept but not runnable)", m, err)
	}
	if failed, err := d.client.ZCard(ctx, d.prefix+"failed").Result(); err != nil || failed != 1 {
		t.Fatalf("failed set size = %d, %v; want 1 (kept)", failed, err)
	}
	n, err := q.RetryFailed(ctx)
	if err != nil || n != 1 {
		t.Fatalf("RetryFailed = %d, %v; want 1", n, err)
	}
	if failed, _ := d.client.ZCard(ctx, d.prefix+"failed").Result(); failed != 0 {
		t.Errorf("failed set size after RetryFailed = %d, want 0", failed)
	}
	if sink.count() != 2 {
		t.Errorf("runs after RetryFailed = %d, want still 2 (retry only requeues)", sink.count())
	}
	// Fresh cycle: the re-pushed failure requeues for its retry.
	if _, err := q.Run(ctx); err != nil {
		t.Fatalf("Run 3: %v", err)
	}
	if sink.count() != 3 {
		t.Errorf("runs after re-push = %d, want 3", sink.count())
	}
	if !waitFor("re-pushed retry due", 2*time.Second, func() bool {
		return d.dueCount(t) == 1
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

// TestRedisTTRReclaimRequeuesReservation verifies a reservation whose TTR
// elapsed returns to waiting by itself — the crashed-worker recovery path.
func TestRedisTTRReclaimRequeuesReservation(t *testing.T) {
	d, _ := newTestRedis(t)
	ctx := context.Background()

	if err := d.Push(ctx, &Message{Name: "counted", Payload: []byte(`{}`), DueAt: time.Now(), TTR: 60 * time.Millisecond}); err != nil {
		t.Fatalf("Push: %v", err)
	}
	first, err := d.Reserve(ctx)
	if err != nil || first == nil {
		t.Fatalf("Reserve: %v %v", first, err)
	}
	if m, err := d.Reserve(ctx); err != nil || m != nil {
		t.Fatalf("Reserve while reservation alive: %v %v", m, err)
	}
	// Past the TTR the reservation returns to waiting and is claimable again;
	// the probe holds the reclaimed reservation, so capture it there.
	var reclaimed *Message
	if !waitFor("reservation reclaimed", 2*time.Second, func() bool {
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

// TestRedisRunThroughQueueDeliversPayload verifies the payload round-trips
// through the server and reaches the rebuilt job.
func TestRedisRunThroughQueueDeliversPayload(t *testing.T) {
	q, _ := newRedisQueue(t)
	ctx := context.Background()

	mustPush(t, q, &countedJob{Value: "redis", N: 5})
	if n, err := q.Run(ctx); err != nil || n != 1 {
		t.Fatalf("Run = %d, %v; want 1", n, err)
	}
	runs := sink.recorded()
	if len(runs) != 1 || runs[0].Value != "redis" || runs[0].N != 5 {
		t.Errorf("executed with payload %+v, want one run {redis 5}", runs)
	}
}

// TestRedisCustomPrefixIsolatesQueues verifies independent deployments can
// share one database without sharing queues.
func TestRedisCustomPrefixIsolatesQueues(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	a := NewRedis(client, "app1:")
	b := NewRedis(client, "app2:")
	ctx := context.Background()
	if err := a.Push(ctx, &Message{Name: "counted", Payload: []byte(`{}`), DueAt: time.Now(), TTR: time.Minute}); err != nil {
		t.Fatalf("Push a: %v", err)
	}
	if m, err := b.Reserve(ctx); err != nil || m != nil {
		t.Fatalf("Reserve b sees %v %v; want nil (different prefix)", m, err)
	}
	if m, err := a.Reserve(ctx); err != nil || m == nil {
		t.Fatalf("Reserve a: %v %v", m, err)
	}
}
