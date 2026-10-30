package session_test

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/goyii2/framework/session"
)

// sid pads a label into the shape of a minted session id (at least 22
// URL-safe base64 characters): store-level ids must look like the ones the
// manager mints, since they double as file names.
func sid(label string) string {
	for len(label) < 24 {
		label += "-0"
	}
	return label
}

// storeCase is the behavioral matrix both backends must satisfy identically.
// expire simulates the record outliving its timeout without waiting on
// wall-clock time.
type storeCase struct {
	name   string
	store  session.Store
	expire func(id string)
}

func storeCases(t *testing.T, ttl time.Duration) []storeCase {
	t.Helper()

	dir := t.TempDir()
	file := session.NewFileStore(dir)

	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	redisStore := session.NewRedisStore(client, "goyii:session:")

	return []storeCase{
		{
			name:  "file",
			store: file,
			expire: func(id string) {
				past := time.Now().Add(-(ttl + time.Second))
				if err := os.Chtimes(filepath.Join(dir, id), past, past); err != nil {
					t.Fatalf("backdating session file: %v", err)
				}
			},
		},
		{
			name:  "redis",
			store: redisStore,
			expire: func(id string) {
				mr.FastForward(ttl + time.Second)
			},
		},
	}
}

// TestStoreBehaviorConsistency runs one scenario table over the file and
// redis backends: both must load, round-trip, overwrite, destroy and expire
// identically.
func TestStoreBehaviorConsistency(t *testing.T) {
	ttl := 30 * time.Second
	ctx := t.Context()

	for _, tc := range storeCases(t, ttl) {
		t.Run(tc.name, func(t *testing.T) {
			// Round trip: values survive a save/load cycle. JSON encoding
			// turns numbers into float64 on the way back, so the expectation
			// matches that shape.
			data := map[string]any{"user": "alice", "count": 7, "nested": map[string]any{"a": "b"}}
			if err := tc.store.Save(ctx, sid("s1"), data, ttl); err != nil {
				t.Fatalf("save: %v", err)
			}
			loaded, ok, err := tc.store.Load(ctx, sid("s1"), ttl)
			if err != nil || !ok {
				t.Fatalf("load after save: ok=%v err=%v", ok, err)
			}
			if loaded["user"] != "alice" || loaded["nested"].(map[string]any)["a"] != "b" {
				t.Fatalf("round trip mismatch: %#v", loaded)
			}
			if n := loaded["count"].(float64); n != 7 {
				t.Fatalf("count = %v, want 7", n)
			}

			// Unknown ids report missing, never an error nor a session.
			if _, ok, err := tc.store.Load(ctx, sid("none"), ttl); ok || err != nil {
				t.Fatalf("load unknown id: ok=%v err=%v, want ok=false err=nil", ok, err)
			}

			// A save replaces the record wholesale: keys dropped from the
			// map disappear instead of merging into the old record.
			if err := tc.store.Save(ctx, sid("s1"), map[string]any{"only": "one"}, ttl); err != nil {
				t.Fatalf("overwrite save: %v", err)
			}
			loaded, ok, err = tc.store.Load(ctx, sid("s1"), ttl)
			if err != nil || !ok {
				t.Fatalf("load after overwrite: ok=%v err=%v", ok, err)
			}
			if len(loaded) != 1 || loaded["only"] != "one" {
				t.Fatalf("overwrite left residue: %#v", loaded)
			}

			// Destroy removes the record; destroying again stays clean.
			if err := tc.store.Destroy(ctx, sid("s1")); err != nil {
				t.Fatalf("destroy: %v", err)
			}
			if err := tc.store.Destroy(ctx, sid("s1")); err != nil {
				t.Fatalf("destroy twice: %v", err)
			}
			if _, ok, _ := tc.store.Load(ctx, sid("s1"), ttl); ok {
				t.Fatal("destroyed id still loads")
			}

			// Records idle past the timeout expire: a load reports missing
			// instead of resurrecting stale data.
			if err := tc.store.Save(ctx, sid("s2"), map[string]any{"k": "v"}, ttl); err != nil {
				t.Fatalf("save s2: %v", err)
			}
			tc.expire(sid("s2"))
			if _, ok, _ := tc.store.Load(ctx, sid("s2"), ttl); ok {
				t.Fatal("expired id still loads")
			}
		})
	}
}

// TestStoreConcurrentAccess hammers both backends from parallel goroutines;
// beyond correctness it guards the -race build (store internals must be safe
// for concurrent use).
func TestStoreConcurrentAccess(t *testing.T) {
	ttl := 30 * time.Second
	ctx := t.Context()

	for _, tc := range storeCases(t, ttl) {
		t.Run(tc.name, func(t *testing.T) {
			const workers, ops = 16, 25
			var wg sync.WaitGroup
			for w := range workers {
				wg.Add(1)
				go func(w int) {
					defer wg.Done()
					id := sid(fmt.Sprintf("cw-%d", w))
					for i := range ops {
						if err := tc.store.Save(ctx, id, map[string]any{"w": w, "i": i}, ttl); err != nil {
							t.Errorf("worker %d save: %v", w, err)
							return
						}
						if _, ok, err := tc.store.Load(ctx, id, ttl); !ok || err != nil {
							t.Errorf("worker %d load: ok=%v err=%v", w, ok, err)
							return
						}
						// A shared id exercises cross-goroutine contention.
						if _, _, err := tc.store.Load(ctx, sid("shared"), ttl); err != nil {
							t.Errorf("worker %d shared load: %v", w, err)
							return
						}
					}
					if err := tc.store.Destroy(ctx, id); err != nil {
						t.Errorf("worker %d destroy: %v", w, err)
					}
				}(w)
			}
			wg.Wait()
		})
	}
}

func TestNewZeroConfigIsFileBackend(t *testing.T) {
	m, err := session.New(session.Config{})
	if err != nil {
		t.Fatalf("zero-value construction: %v", err)
	}
	if _, ok := m.Store.(*session.FileStore); !ok {
		t.Fatalf("zero-value backend = %T, want *session.FileStore", m.Store)
	}
	if m.CookieName != "goyii-session" {
		t.Errorf("default cookie name = %q, want goyii-session", m.CookieName)
	}
	if m.Timeout != 1440*time.Second {
		t.Errorf("default timeout = %s, want 24m", m.Timeout)
	}
	if !m.HttpOnly || m.SameSite != http.SameSiteLaxMode {
		t.Errorf("defensive cookie defaults lost: HttpOnly=%v SameSite=%v", m.HttpOnly, m.SameSite)
	}
}

func TestNewRedisBackendRequiresClient(t *testing.T) {
	if _, err := session.New(session.Config{Backend: "redis"}); err == nil {
		t.Fatal("redis backend without a client must fail construction")
	}
}

func TestNewRejectsUnknownBackend(t *testing.T) {
	if _, err := session.New(session.Config{Backend: "memcached"}); err == nil {
		t.Fatal("unknown backend must fail construction")
	}
}

func TestNewRedisBackendStoresOnInjectedClient(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	m, err := session.New(session.Config{Backend: "redis"}, session.WithRedis(client))
	if err != nil {
		t.Fatalf("redis construction: %v", err)
	}
	if err := m.Store.Save(context.Background(), "s", map[string]any{"k": "v"}, time.Minute); err != nil {
		t.Fatalf("save: %v", err)
	}
	if !mr.Exists("goyii:session:s") {
		t.Error("record must land under the goyii:session: prefix on the injected client")
	}
}
