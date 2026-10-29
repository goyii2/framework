package mutex

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestProcessLockExcludesConcurrentCallers verifies the core mutual
// exclusion property across goroutines: with N goroutines locking the same
// name, the critical section runs one at a time and every goroutine gets
// its turn.
func TestProcessLockExcludesConcurrentCallers(t *testing.T) {
	m := NewProcess()
	const (
		callers = 16
		rounds  = 50
	)
	var (
		inside    atomic.Int32
		maxInside atomic.Int32
		done      sync.WaitGroup
	)
	done.Add(callers)
	for i := 0; i < callers; i++ {
		go func() {
			defer done.Done()
			for r := 0; r < rounds; r++ {
				release, err := m.Lock(context.Background(), "job", time.Minute)
				if err != nil {
					t.Errorf("Lock: %v", err)
					return
				}
				n := inside.Add(1)
				for {
					cur := maxInside.Load()
					if n <= cur || maxInside.CompareAndSwap(cur, n) {
						break
					}
				}
				time.Sleep(50 * time.Microsecond) // widen the race window
				if got := inside.Add(-1); got != 0 {
					t.Errorf("critical section overlapped: inside=%d", got)
					return
				}
				release()
			}
		}()
	}
	done.Wait()
	if maxInside.Load() != 1 {
		t.Fatalf("max concurrent holders = %d, want 1", maxInside.Load())
	}
}

// TestProcessTryLockCompetes verifies TryLock's immediate verdict: exactly
// one of many simultaneous attempts on a held name succeeds, the losers get
// ErrLocked without waiting.
func TestProcessTryLockCompetes(t *testing.T) {
	m := NewProcess()
	release, err := m.TryLock(context.Background(), "leader", time.Minute)
	if err != nil {
		t.Fatalf("first TryLock: %v", err)
	}
	defer release()

	// Held name: every attempt fails at once with ErrLocked (not by timing
	// out — the call must return well before any plausible wait would).
	for i := 0; i < 8; i++ {
		start := time.Now()
		rel, err := m.TryLock(context.Background(), "leader", time.Minute)
		if !errors.Is(err, ErrLocked) {
			t.Fatalf("TryLock on held name: err=%v release=%t, want ErrLocked", err, rel != nil)
		}
		if d := time.Since(start); d > 100*time.Millisecond {
			t.Fatalf("TryLock waited %v, want immediate ErrLocked", d)
		}
	}

	// A different name stays free: names are independent locks.
	rel2, err := m.TryLock(context.Background(), "other", time.Minute)
	if err != nil {
		t.Fatalf("TryLock on a different name: %v", err)
	}
	rel2()
}

// TestProcessLockWaitsUntilRelease verifies Lock blocks while the name is
// held and acquires as soon as the holder releases.
func TestProcessLockWaitsUntilRelease(t *testing.T) {
	m := NewProcess()
	release, err := m.Lock(context.Background(), "job", time.Minute)
	if err != nil {
		t.Fatalf("first Lock: %v", err)
	}

	acquired := make(chan struct{})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
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

// TestProcessLockContextDoneWhileWaiting verifies a caller gives up with
// its context's error instead of waiting forever.
func TestProcessLockContextDoneWhileWaiting(t *testing.T) {
	m := NewProcess()
	release, err := m.Lock(context.Background(), "job", time.Minute)
	if err != nil {
		t.Fatalf("first Lock: %v", err)
	}
	defer release()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	rel, err := m.Lock(ctx, "job", time.Minute)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("waiting Lock: err=%v release=%t, want context.DeadlineExceeded", err, rel != nil)
	}

	// Same for a context that is canceled (not by deadline).
	ctx2, cancel2 := context.WithCancel(context.Background())
	cancel2()
	rel2, err := m.Lock(ctx2, "job", time.Minute)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Lock with canceled ctx: err=%v release=%t, want context.Canceled", err, rel2 != nil)
	}
}

// TestProcessTTLFreesWithoutRelease verifies the expiry contract: once the
// ttl elapses the lock frees itself, and the original holder's release
// stays a harmless no-op that cannot disturb the successor.
func TestProcessTTLFreesWithoutRelease(t *testing.T) {
	m := NewProcess()
	release, err := m.Lock(context.Background(), "job", 50*time.Millisecond)
	if err != nil {
		t.Fatalf("Lock: %v", err)
	}

	// Wait out the ttl: the name must become acquirable with no release.
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := m.TryLock(context.Background(), "job", time.Minute); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("lock not freed after its ttl elapsed")
		}
		time.Sleep(5 * time.Millisecond)
	}

	// The successor holds the name now; the expired holder's late release
	// must not free it (release only ever frees its own acquisition).
	release()
	if _, err := m.TryLock(context.Background(), "job", time.Minute); !errors.Is(err, ErrLocked) {
		t.Fatalf("late release freed the successor's lock: err=%v, want ErrLocked", err)
	}
}

// TestProcessReleaseIdempotent verifies calling the release function more
// than once is safe and actually frees the name.
func TestProcessReleaseIdempotent(t *testing.T) {
	m := NewProcess()
	release, err := m.Lock(context.Background(), "job", time.Minute)
	if err != nil {
		t.Fatalf("Lock: %v", err)
	}
	release()
	release() // must not panic, block, or free anyone else's lock

	rel, err := m.TryLock(context.Background(), "job", time.Minute)
	if err != nil {
		t.Fatalf("TryLock after double release: %v", err)
	}
	rel()
}

// TestProcessRejectsInvalidArguments verifies the input contract: an empty
// name or a non-positive ttl fails acquisition for both entry points, and
// an already-done context fails even though the name is free.
func TestProcessRejectsInvalidArguments(t *testing.T) {
	m := NewProcess()
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

	ctx2, cancel := context.WithCancel(context.Background())
	cancel()
	if rel, err := m.Lock(ctx2, "job", time.Minute); err == nil {
		rel()
		t.Fatal("Lock with a done context succeeded, want the context error")
	}
	if rel, err := m.TryLock(ctx2, "job", time.Minute); err == nil {
		rel()
		t.Fatal("TryLock with a done context succeeded, want the context error")
	}
}
