package cache

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// statsValue is the round-trip payload of the matrix: a struct with the
// shapes JSON handles differently (int, string, nested slice).
type statsValue struct {
	Count int      `json:"count"`
	Label string   `json:"label"`
	Tags  []string `json:"tags"`
}

// cacheCase is one backend under test. expire makes an entry stored with
// liveTTL read as expired without waiting on wall-clock time; fastforward
// advances the backend's effective clock by d (real sleep or miniredis
// FastForward).
type cacheCase struct {
	name        string
	cache       Cache
	liveTTL     time.Duration
	expire      func(t *testing.T, key string)
	fastforward func(d time.Duration)
}

// cacheCases assembles the three backends with identical contracts. The
// memory and file entries expire through their envelope's absolute time, so
// their hooks backdate it directly; redis expires server-side, so its hook
// advances the server clock.
func cacheCases(t *testing.T) []cacheCase {
	t.Helper()

	ttl := time.Hour
	mem := NewMemory(ttl)
	t.Cleanup(mem.Stop)
	dir := t.TempDir()
	file := NewFile(dir, "goyii:cache:", ttl)

	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	rds := NewRedis(client, "goyii:cache:", ttl)

	return []cacheCase{
		{
			name:    "memory",
			cache:   mem,
			liveTTL: ttl,
			expire: func(t *testing.T, key string) {
				backdateMemoryEntry(t, mem, key)
			},
			fastforward: time.Sleep,
		},
		{
			name:    "file",
			cache:   file,
			liveTTL: ttl,
			expire: func(t *testing.T, key string) {
				backdateFileEntry(t, file, key)
			},
			fastforward: time.Sleep,
		},
		{
			name:    "redis",
			cache:   rds,
			liveTTL: ttl,
			expire: func(t *testing.T, key string) {
				mr.FastForward(ttl + time.Minute)
			},
			fastforward: mr.FastForward,
		},
	}
}

// backdateMemoryEntry moves the entry's expiry into the past, as if its ttl
// had elapsed.
func backdateMemoryEntry(t *testing.T, m *Memory, key string) {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.items[key]
	if !ok {
		t.Fatalf("backdating memory entry %q: not stored", key)
	}
	e.ExpiresAt = time.Now().Add(-time.Minute).UnixNano()
	m.items[key] = e
}

// backdateFileEntry rewrites the entry file with a past expiry time, as if
// its ttl had elapsed.
func backdateFileEntry(t *testing.T, f *File, key string) {
	t.Helper()
	path := f.path(key)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("backdating file entry %q: %v", key, err)
	}
	var e entry
	if err := json.Unmarshal(b, &e); err != nil {
		t.Fatalf("backdating file entry %q: %v", key, err)
	}
	e.ExpiresAt = time.Now().Add(-time.Minute).UnixNano()
	b, err = json.Marshal(e)
	if err != nil {
		t.Fatalf("backdating file entry %q: %v", key, err)
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatalf("backdating file entry %q: %v", key, err)
	}
}

// TestCacheBehaviorConsistency runs one scenario table over the memory, file
// and redis backends: all three must round-trip, overwrite, delete, report
// existence, expire and flush identically.
func TestCacheBehaviorConsistency(t *testing.T) {
	for _, tc := range cacheCases(t) {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			c := tc.cache

			t.Run("round trip and overwrite", func(t *testing.T) {
				want := statsValue{Count: 3, Label: "demo", Tags: []string{"a", "b"}}
				if err := c.Set(ctx, "stats", want, tc.liveTTL); err != nil {
					t.Fatalf("Set: %v", err)
				}
				var got statsValue
				found, err := c.Get(ctx, "stats", &got)
				if err != nil || !found {
					t.Fatalf("Get after Set: found=%v err=%v", found, err)
				}
				if got.Count != want.Count || got.Label != want.Label || len(got.Tags) != 2 || got.Tags[1] != "b" {
					t.Errorf("round trip decoded %+v, want %+v", got, want)
				}

				// The stored value must be a copy: mutating the decoded
				// reference cannot leak into the cache.
				got.Count = 99
				var again statsValue
				if _, err := c.Get(ctx, "stats", &again); err != nil {
					t.Fatalf("second Get: %v", err)
				}
				if again.Count != want.Count {
					t.Errorf("decoded value aliases the stored entry: Count=%d, want %d", again.Count, want.Count)
				}

				if err := c.Set(ctx, "stats", statsValue{Count: 7}, tc.liveTTL); err != nil {
					t.Fatalf("overwriting Set: %v", err)
				}
				var latest statsValue
				if _, err := c.Get(ctx, "stats", &latest); err != nil {
					t.Fatalf("Get after overwrite: %v", err)
				}
				if latest.Count != 7 || latest.Label != "" {
					t.Errorf("Get after overwrite decoded %+v, want the replaced value", latest)
				}
			})

			t.Run("missing key reads as miss", func(t *testing.T) {
				var got statsValue
				got.Count = 5
				found, err := c.Get(ctx, "missing", &got)
				if err != nil {
					t.Fatalf("Get missing key: %v", err)
				}
				if found {
					t.Error("Get of a missing key reported found")
				}
				if got.Count != 5 {
					t.Error("Get of a missing key must leave dest untouched")
				}
			})

			t.Run("exists tracks set and delete", func(t *testing.T) {
				ctx := context.Background()
				ok, err := c.Exists(ctx, "exists-key")
				if err != nil {
					t.Fatalf("Exists before Set: %v", err)
				}
				if ok {
					t.Error("Exists reported true before Set")
				}
				if err := c.Set(ctx, "exists-key", 1, tc.liveTTL); err != nil {
					t.Fatalf("Set: %v", err)
				}
				if ok, err := c.Exists(ctx, "exists-key"); err != nil || !ok {
					t.Fatalf("Exists after Set: ok=%v err=%v", ok, err)
				}
				if err := c.Delete(ctx, "exists-key"); err != nil {
					t.Fatalf("Delete: %v", err)
				}
				if ok, err := c.Exists(ctx, "exists-key"); err != nil || ok {
					t.Fatalf("Exists after Delete: ok=%v err=%v", ok, err)
				}
				// Deleting an absent key is not an error.
				if err := c.Delete(ctx, "exists-key"); err != nil {
					t.Fatalf("Delete of an absent key: %v", err)
				}
				var got statsValue
				if found, _ := c.Get(ctx, "exists-key", &got); found {
					t.Error("Get after Delete reported found")
				}
			})

			t.Run("expired entry reads as miss", func(t *testing.T) {
				if err := c.Set(ctx, "expiring", "v", tc.liveTTL); err != nil {
					t.Fatalf("Set: %v", err)
				}
				tc.expire(t, "expiring")
				var got string
				if found, err := c.Get(ctx, "expiring", &got); err != nil || found {
					t.Fatalf("Get after expiry: found=%v err=%v", found, err)
				}
				if ok, err := c.Exists(ctx, "expiring"); err != nil || ok {
					t.Fatalf("Exists after expiry: ok=%v err=%v", ok, err)
				}
			})

			t.Run("flush drops every entry", func(t *testing.T) {
				if err := c.Set(ctx, "flush-a", 1, tc.liveTTL); err != nil {
					t.Fatalf("Set a: %v", err)
				}
				if err := c.Set(ctx, "flush-b", 2, tc.liveTTL); err != nil {
					t.Fatalf("Set b: %v", err)
				}
				if err := c.Flush(ctx); err != nil {
					t.Fatalf("Flush: %v", err)
				}
				for _, key := range []string{"flush-a", "flush-b"} {
					if ok, err := c.Exists(ctx, key); err != nil || ok {
						t.Errorf("Exists(%q) after Flush: ok=%v err=%v", key, ok, err)
					}
				}
			})

			t.Run("get or set rebuilds only on miss", func(t *testing.T) {
				builds := 0
				build := func(context.Context) (any, error) {
					builds++
					return statsValue{Count: 11}, nil
				}
				var got statsValue
				if err := c.GetOrSet(ctx, "gos", &got, build, tc.liveTTL, nil); err != nil {
					t.Fatalf("GetOrSet miss: %v", err)
				}
				if got.Count != 11 || builds != 1 {
					t.Fatalf("GetOrSet miss: dest.Count=%d builds=%d, want 11/1", got.Count, builds)
				}
				got.Count = 0
				if err := c.GetOrSet(ctx, "gos", &got, build, tc.liveTTL, nil); err != nil {
					t.Fatalf("GetOrSet hit: %v", err)
				}
				if got.Count != 11 || builds != 1 {
					t.Fatalf("GetOrSet hit must decode the cached value without rebuilding: dest.Count=%d builds=%d", got.Count, builds)
				}
				if err := c.Delete(ctx, "gos"); err != nil {
					t.Fatalf("Delete: %v", err)
				}
				if err := c.GetOrSet(ctx, "gos", &got, build, tc.liveTTL, nil); err != nil {
					t.Fatalf("GetOrSet after Delete: %v", err)
				}
				if builds != 2 {
					t.Fatalf("GetOrSet after Delete must rebuild: builds=%d, want 2", builds)
				}
			})

			t.Run("rejects empty keys", func(t *testing.T) {
				if err := c.Set(ctx, "", "v", tc.liveTTL); err == nil {
					t.Error("Set with an empty key returned nil error")
				}
				var got string
				if found, err := c.Get(ctx, "", &got); err == nil || found {
					t.Errorf("Get with an empty key: found=%v err=%v, want an error", found, err)
				}
			})

			t.Run("unencodable value fails the write", func(t *testing.T) {
				if err := c.Set(ctx, "bad", make(chan int), tc.liveTTL); err == nil {
					t.Error("Set of a channel returned nil error")
				}
			})
		})
	}
}

// TestFileFlushCleansTemporariesAndSparesForeignFiles verifies the file
// backend's Flush owns exactly its own files: crashed-write temporaries go,
// foreign files in the directory stay.
func TestFileFlushCleansTemporariesAndSparesForeignFiles(t *testing.T) {
	dir := t.TempDir()
	f := NewFile(dir, "", time.Hour)
	ctx := context.Background()

	if err := f.Set(ctx, "k", "v", time.Hour); err != nil {
		t.Fatalf("Set: %v", err)
	}
	temp := filepath.Join(dir, "cache-crash-leftover")
	if err := os.WriteFile(temp, []byte("partial"), 0o600); err != nil {
		t.Fatalf("writing leftover temporary: %v", err)
	}
	foreign := filepath.Join(dir, "foreign.txt")
	if err := os.WriteFile(foreign, []byte("mine"), 0o600); err != nil {
		t.Fatalf("writing foreign file: %v", err)
	}

	if err := f.Flush(ctx); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	if _, err := os.Stat(temp); !os.IsNotExist(err) {
		t.Error("Flush must remove leftover write temporaries")
	}
	if _, err := os.Stat(foreign); err != nil {
		t.Errorf("Flush must spare foreign files: %v", err)
	}
	var got string
	if found, _ := f.Get(ctx, "k", &got); found {
		t.Error("Flush must remove the entries")
	}
}

// TestCacheNonPositiveTTLFallsBackToDefault verifies a ttl <= 0 stores under
// the backend's default lifetime instead of forever: the entry survives a
// short wait and expires once the default elapses.
func TestCacheNonPositiveTTLFallsBackToDefault(t *testing.T) {
	const defTTL = 60 * time.Millisecond
	margin := 50 * time.Millisecond

	mem := NewMemory(defTTL)
	t.Cleanup(mem.Stop)
	dir := t.TempDir()
	file := NewFile(dir, "", defTTL)

	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	rds := NewRedis(client, "", defTTL)

	cases := []struct {
		name        string
		cache       Cache
		fastforward func(time.Duration)
	}{
		{"memory", mem, time.Sleep},
		{"file", file, time.Sleep},
		{"redis", rds, mr.FastForward},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			if err := tc.cache.Set(ctx, "ttl-default", "v", 0); err != nil {
				t.Fatalf("Set with ttl 0: %v", err)
			}
			var got string
			if found, err := tc.cache.Get(ctx, "ttl-default", &got); err != nil || !found || got != "v" {
				t.Fatalf("Get right after Set: found=%v value=%q err=%v", found, got, err)
			}
			tc.fastforward(defTTL + margin)
			if found, err := tc.cache.Get(ctx, "ttl-default", &got); err != nil || found {
				t.Fatalf("Get after the default ttl elapsed: found=%v err=%v, want an expired miss", found, err)
			}
		})
	}
}
