package mutex

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// newTestRedis starts a miniredis and a redis mutex bound to it with the
// shortest poll delay, so the blocking tests run fast.
func newTestRedis(t *testing.T) (*Redis, *miniredis.Miniredis, redis.UniversalClient) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	m := NewRedis(client, "")
	m.retryDelay = time.Millisecond
	return m, mr, client
}

// TestRedisLockExcludesSecondAcquirer verifies the basic exclusion: while
// the key exists no second acquisition succeeds, after release it does.
func TestRedisLockExcludesSecondAcquirer(t *testing.T) {
	m, mr, _ := newTestRedis(t)
	ctx := context.Background()

	release, err := m.Lock(ctx, "job", time.Minute)
	if err != nil {
		t.Fatalf("first Lock: %v", err)
	}
	if !mr.Exists("goyii:mutex:job") {
		t.Fatal("lock key missing under the default prefix")
	}
	if _, err := m.TryLock(ctx, "job", time.Minute); !errors.Is(err, ErrLocked) {
		t.Fatalf("TryLock on held name: err=%v, want ErrLocked", err)
	}
	// A second mutex instance on the same server sees the same held name:
	// the lock is cross-process, not per-instance.
	other := NewRedis(redis.NewClient(&redis.Options{Addr: mr.Addr()}), "")
	if _, err := other.TryLock(ctx, "job", time.Minute); !errors.Is(err, ErrLocked) {
		t.Fatalf("TryLock from a second instance: err=%v, want ErrLocked", err)
	}

	release()
	if mr.Exists("goyii:mutex:job") {
		t.Fatal("release must delete the lock key")
	}
	rel, err := m.TryLock(ctx, "job", time.Minute)
	if err != nil {
		t.Fatalf("TryLock after release: %v", err)
	}
	rel()
}

// TestRedisTryLockCompetes verifies exactly one winner among many
// simultaneous TryLock attempts, and that losers see ErrLocked.
func TestRedisTryLockCompetes(t *testing.T) {
	m, _, _ := newTestRedis(t)
	const attempts = 16
	var (
		wg       sync.WaitGroup
		winners  atomic.Int32
		locked   atomic.Int32
		releases sync.Map // release funcs of the winners, freed at the end
	)
	wg.Add(attempts)
	for i := 0; i < attempts; i++ {
		go func() {
			defer wg.Done()
			rel, err := m.TryLock(context.Background(), "leader", time.Minute)
			if errors.Is(err, ErrLocked) {
				locked.Add(1)
				return
			}
			if err != nil {
				t.Errorf("TryLock: %v", err)
				return
			}
			winners.Add(1)
			releases.Store(1, rel)
		}()
	}
	wg.Wait()
	if winners.Load() != 1 {
		t.Fatalf("winners = %d, want 1", winners.Load())
	}
	if locked.Load() != attempts-1 {
		t.Fatalf("ErrLocked responses = %d, want %d", locked.Load(), attempts-1)
	}
	if rel, ok := releases.Load(1); ok {
		rel.(func())()
	}
}

// TestRedisLockWaitsUntilRelease verifies Lock polls until the holder
// releases, then acquires.
func TestRedisLockWaitsUntilRelease(t *testing.T) {
	m, _, _ := newTestRedis(t)
	ctx := context.Background()
	release, err := m.Lock(ctx, "job", time.Minute)
	if err != nil {
		t.Fatalf("first Lock: %v", err)
	}

	acquired := make(chan struct{})
	go func() {
		defer close(acquired)
		rel, err := m.Lock(ctx, "job", time.Minute)
		if err != nil {
			t.Errorf("waiting Lock: %v", err)
			return
		}
		rel()
	}()

	select {
	case <-acquired:
		t.Fatal("Lock acquired while the name was still held")
	case <-time.After(100 * time.Millisecond):
	}

	release()
	select {
	case <-acquired:
	case <-time.After(5 * time.Second):
		t.Fatal("Lock did not acquire after the holder released")
	}
}

// TestRedisLockContextDoneWhileWaiting verifies Lock gives up with the
// context's error while the name stays held.
func TestRedisLockContextDoneWhileWaiting(t *testing.T) {
	m, _, _ := newTestRedis(t)
	ctx := context.Background()
	release, err := m.Lock(ctx, "job", time.Minute)
	if err != nil {
		t.Fatalf("first Lock: %v", err)
	}
	defer release()

	tctx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	if rel, err := m.Lock(tctx, "job", time.Minute); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("waiting Lock: err=%v release=%t, want context.DeadlineExceeded", err, rel != nil)
	}

	cctx, ccancel := context.WithCancel(ctx)
	ccancel()
	if rel, err := m.Lock(cctx, "job", time.Minute); !errors.Is(err, context.Canceled) {
		t.Fatalf("Lock with canceled ctx: err=%v release=%t, want context.Canceled", err, rel != nil)
	}
}

// TestRedisTTLFreesLock verifies the expiry contract: the ttl drops the
// lock server-side, and the expired holder's late release — which finds a
// foreign token under the key — deletes nothing.
func TestRedisTTLFreesLock(t *testing.T) {
	m, mr, _ := newTestRedis(t)
	ctx := context.Background()
	release, err := m.Lock(ctx, "job", time.Second)
	if err != nil {
		t.Fatalf("Lock: %v", err)
	}

	mr.FastForward(2 * time.Second)
	if mr.Exists("goyii:mutex:job") {
		t.Fatal("lock key survived its ttl")
	}

	// The successor acquires the expired name; the stale release must not
	// disturb it.
	rel2, err := m.TryLock(ctx, "job", time.Minute)
	if err != nil {
		t.Fatalf("TryLock after ttl expiry: %v", err)
	}
	release()
	if !mr.Exists("goyii:mutex:job") {
		t.Fatal("stale release deleted the successor's lock")
	}
	if _, err := m.TryLock(ctx, "job", time.Minute); !errors.Is(err, ErrLocked) {
		t.Fatalf("TryLock after stale release: err=%v, want ErrLocked", err)
	}
	rel2()
}

// TestRedisReleaseChecksToken verifies the release guard: a release whose
// token no longer matches the stored value (the key was rewritten by a
// successor or a foreign writer) deletes nothing.
func TestRedisReleaseChecksToken(t *testing.T) {
	m, mr, client := newTestRedis(t)
	ctx := context.Background()
	release, err := m.Lock(ctx, "job", time.Minute)
	if err != nil {
		t.Fatalf("Lock: %v", err)
	}

	// Simulate a foreign holder rewriting the key under the same name.
	if err := client.Set(ctx, "goyii:mutex:job", "foreign-token", time.Minute).Err(); err != nil {
		t.Fatalf("rewriting the key: %v", err)
	}
	release()
	if !mr.Exists("goyii:mutex:job") {
		t.Fatal("release with a mismatched token deleted the key")
	}
	if got, _ := mr.Get("goyii:mutex:job"); got != "foreign-token" {
		t.Fatalf("stored token = %q, want the foreign one untouched", got)
	}

	// With the foreign key cleared, a fresh acquisition's release works.
	if err := client.Del(ctx, "goyii:mutex:job").Err(); err != nil {
		t.Fatalf("clearing the key: %v", err)
	}
	rel, err := m.Lock(ctx, "job", time.Minute)
	if err != nil {
		t.Fatalf("Lock after clearing: %v", err)
	}
	rel()
	if mr.Exists("goyii:mutex:job") {
		t.Fatal("release with a matching token did not delete the key")
	}
}

// TestRedisCustomPrefixIsolatesKeys verifies a custom prefix namespaces the
// lock keys so several mutex deployments can share one database.
func TestRedisCustomPrefixIsolatesKeys(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	m := NewRedis(client, "app1:lock:")
	m.retryDelay = time.Millisecond

	ctx := context.Background()
	release, err := m.TryLock(ctx, "job", time.Minute)
	if err != nil {
		t.Fatalf("TryLock: %v", err)
	}
	if !mr.Exists("app1:lock:job") {
		t.Fatal("lock key missing under the custom prefix")
	}
	// The default prefix sees nothing: different namespace, different lock —
	// the two prefixes exclude independently.
	if mr.Exists("goyii:mutex:job") {
		t.Fatal("lock key leaked into the default prefix")
	}
	other := NewRedis(client, "")
	rel, err := other.TryLock(ctx, "job", time.Minute)
	if err != nil {
		t.Fatalf("TryLock under the default prefix: %v", err)
	}
	rel()
	release()
}

// TestRedisRejectsInvalidArguments verifies the shared input contract
// without touching the server.
func TestRedisRejectsInvalidArguments(t *testing.T) {
	m, _, _ := newTestRedis(t)
	ctx := context.Background()
	for _, c := range []struct {
		name string
		ttl  time.Duration
	}{{"", time.Minute}, {"job", 0}, {"job", -time.Second}} {
		if rel, err := m.Lock(ctx, c.name, c.ttl); err == nil {
			rel()
			t.Fatalf("Lock(name=%q, ttl=%v) succeeded, want error", c.name, c.ttl)
		}
		if rel, err := m.TryLock(ctx, c.name, c.ttl); err == nil {
			rel()
			t.Fatalf("TryLock(name=%q, ttl=%v) succeeded, want error", c.name, c.ttl)
		}
	}

	cctx, cancel := context.WithCancel(ctx)
	cancel()
	if rel, err := m.Lock(cctx, "job", time.Minute); err == nil {
		rel()
		t.Fatal("Lock with a done context succeeded, want the context error")
	}
	if rel, err := m.TryLock(cctx, "job", time.Minute); err == nil {
		rel()
		t.Fatal("TryLock with a done context succeeded, want the context error")
	}
}
