package cache

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// TestMemorySweeperDropsExpiredEntries verifies the background sweep removes
// entries whose ttl elapsed even when nothing reads them.
func TestMemorySweeperDropsExpiredEntries(t *testing.T) {
	// ttl 20ms puts the sweep interval at the 10ms floor.
	m := NewMemory(20 * time.Millisecond)
	t.Cleanup(m.Stop)

	ctx := context.Background()
	if err := m.Set(ctx, "a", 1, 20*time.Millisecond); err != nil {
		t.Fatalf("Set a: %v", err)
	}
	if err := m.Set(ctx, "b", 2, time.Minute); err != nil {
		t.Fatalf("Set b: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	var aAlive, bAlive bool
	for {
		m.mu.Lock()
		_, aAlive = m.items["a"]
		_, bAlive = m.items["b"]
		m.mu.Unlock()
		if !aAlive {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("expired entry still present after the sweep window")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !bAlive {
		t.Error("the sweeper removed a live entry")
	}
}

// TestMemoryStopHaltsSweep verifies Stop ends the background goroutine: the
// items map then only sheds entries lazily on read.
func TestMemoryStopHaltsSweep(t *testing.T) {
	m := NewMemory(20 * time.Millisecond)
	m.Stop()

	ctx := context.Background()
	if err := m.Set(ctx, "a", 1, 20*time.Millisecond); err != nil {
		t.Fatalf("Set: %v", err)
	}
	backdateMemoryEntry(t, m, "a")

	// Give a would-be sweep tick ample time to (wrongly) fire.
	time.Sleep(80 * time.Millisecond)
	m.mu.RLock()
	_, present := m.items["a"]
	m.mu.RUnlock()
	if !present {
		t.Fatal("entry vanished although the sweeper was stopped")
	}

	// Reads still expire the entry lazily.
	var got int
	if found, err := m.Get(ctx, "a", &got); err != nil || found {
		t.Fatalf("Get of an expired entry with the sweeper stopped: found=%v err=%v", found, err)
	}
	m.mu.RLock()
	_, present = m.items["a"]
	m.mu.RUnlock()
	if present {
		t.Error("expired entry must still be droppable lazily on read")
	}
}

// TestRedisFlushScopedToPrefix verifies Flush removes only this cache's keys:
// a redis database is shared infrastructure and neighbors must survive.
func TestRedisFlushScopedToPrefix(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	mine := NewRedis(client, "goyii:cache:app1:", time.Hour)
	neighbor := NewRedis(client, "goyii:cache:app2:", time.Hour)
	bare := NewRedis(client, "", time.Hour)

	ctx := context.Background()
	if err := mine.Set(ctx, "k", 1, time.Hour); err != nil {
		t.Fatalf("Set mine: %v", err)
	}
	if err := neighbor.Set(ctx, "k", 2, time.Hour); err != nil {
		t.Fatalf("Set neighbor: %v", err)
	}
	if err := bare.Set(ctx, "k", 3, time.Hour); err != nil {
		t.Fatalf("Set bare: %v", err)
	}

	if err := mine.Flush(ctx); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	if ok, err := mine.Exists(ctx, "k"); err != nil || ok {
		t.Errorf("mine.Exists after Flush: ok=%v err=%v, want gone", ok, err)
	}
	if ok, err := neighbor.Exists(ctx, "k"); err != nil || !ok {
		t.Errorf("neighbor.Exists after foreign Flush: ok=%v err=%v, want kept", ok, err)
	}
	if ok, err := bare.Exists(ctx, "k"); err != nil || !ok {
		t.Errorf("unprefixed.Exists after foreign Flush: ok=%v err=%v, want kept", ok, err)
	}

	// A cache with no prefix flushes every key of the database.
	if err := bare.Flush(ctx); err != nil {
		t.Fatalf("Flush bare: %v", err)
	}
	if ok, _ := neighbor.Exists(ctx, "k"); ok {
		t.Error("unprefixed Flush must remove every key")
	}
}

// TestCacheConcurrentMixedOperations hammers every backend with goroutines
// mixing writes, reads, GetOrSet rebuilds, existence checks, deletes and
// flushes; run with -race to prove the backends are safe under contention.
func TestCacheConcurrentMixedOperations(t *testing.T) {
	for _, tc := range cacheCases(t) {
		t.Run(tc.name, func(t *testing.T) {
			c := tc.cache
			const workers, iterations = 16, 50

			var wg sync.WaitGroup
			for w := 0; w < workers; w++ {
				wg.Add(1)
				go func(w int) {
					defer wg.Done()
					ctx := context.Background()
					for i := 0; i < iterations; i++ {
						key := "hot"
						switch (w + i) % 6 {
						case 0:
							_ = c.Set(ctx, key, statsValue{Count: i}, tc.liveTTL)
						case 1:
							var got statsValue
							_, _ = c.Get(ctx, key, &got)
						case 2:
							var got statsValue
							_ = c.GetOrSet(ctx, key, &got, func(context.Context) (any, error) {
								return statsValue{Count: i}, nil
							}, tc.liveTTL, nil)
						case 3:
							_, _ = c.Exists(ctx, key)
						case 4:
							_ = c.Delete(ctx, key)
						case 5:
							// A flush racing readers and writers must never
							// corrupt state, only empty it sometimes.
							_ = c.Flush(ctx)
						}
						// Every worker also touches its own key, so deletes
						// and flushes cannot starve shared-key checks.
						_ = c.Set(ctx, "own", w*i, tc.liveTTL)
						var n int
						_, _ = c.Get(ctx, "own", &n)
					}
				}(w)
			}
			wg.Wait()

			// The cache must still work after the storm.
			ctx := context.Background()
			if err := c.Set(ctx, "after", "ok", tc.liveTTL); err != nil {
				t.Fatalf("Set after concurrent phase: %v", err)
			}
			var got string
			if found, err := c.Get(ctx, "after", &got); err != nil || !found || got != "ok" {
				t.Fatalf("Get after concurrent phase: found=%v value=%q err=%v", found, got, err)
			}
		})
	}
}

// TestDependencyConcurrentCapture exercises dependency evaluation under
// concurrent GetOrSet calls; run with -race.
func TestDependencyConcurrentCapture(t *testing.T) {
	mem := NewMemory(time.Hour)
	t.Cleanup(mem.Stop)

	ctx := context.Background()
	var wg sync.WaitGroup
	for w := 0; w < 16; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				var got int
				_ = mem.GetOrSet(ctx, "dep", &got, func(context.Context) (any, error) {
					return i, nil
				}, time.Hour, NewValueDependency(i%2))
			}
		}(w)
	}
	wg.Wait()
}
