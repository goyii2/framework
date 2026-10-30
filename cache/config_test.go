package cache

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// TestNewSelectsDriver verifies the constructor maps each driver value to its
// backend with the configured values applied.
func TestNewSelectsDriver(t *testing.T) {
	t.Run("memory is the zero-value default", func(t *testing.T) {
		for _, driver := range []string{"", "memory", " Memory "} {
			c, err := New(Config{Driver: driver})
			if err != nil {
				t.Fatalf("driver %q: %v", driver, err)
			}
			if _, ok := c.(*Memory); !ok {
				t.Errorf("driver %q did not build a *Memory", driver)
			}
		}
	})

	t.Run("file applies path prefix and ttl", func(t *testing.T) {
		dir := t.TempDir()
		c, err := New(Config{Driver: "file", Path: dir, KeyPrefix: "p:", TTL: 60})
		if err != nil {
			t.Fatalf("driver file: %v", err)
		}
		f, ok := c.(*File)
		if !ok {
			t.Fatalf("driver file built %T, want *File", c)
		}
		if f.dir != dir || f.prefix != "p:" || f.defaultTTL != time.Minute {
			t.Errorf("file backend wiring: dir=%q prefix=%q ttl=%v", f.dir, f.prefix, f.defaultTTL)
		}
	})

	t.Run("redis connects from the config values", func(t *testing.T) {
		mr := miniredis.RunT(t)
		c, err := New(Config{
			Driver: "redis",
			Redis:  RedisConfig{Addr: mr.Addr()},
			TTL:    60,
		})
		if err != nil {
			t.Fatalf("driver redis: %v", err)
		}
		r, ok := c.(*Redis)
		if !ok {
			t.Fatalf("driver redis built %T, want *Redis", c)
		}
		if r.prefix != "goyii:cache:" {
			t.Errorf("empty KeyPrefix must fall back to goyii:cache:, got %q", r.prefix)
		}
		// The connection the constructor built must actually carry entries.
		ctx := context.Background()
		if err := c.Set(ctx, "k", "v", time.Minute); err != nil {
			t.Fatalf("Set through the constructed backend: %v", err)
		}
		var got string
		if found, err := c.Get(ctx, "k", &got); err != nil || !found || got != "v" {
			t.Fatalf("Get through the constructed backend: found=%v value=%q err=%v", found, got, err)
		}
	})
}

// TestNewWithRedisInjectsClient verifies the dependency-injection option: the
// redis driver adopts the exact client handed in (one connection shared with
// every other redis-backed component), while the other drivers ignore the
// client entirely.
func TestNewWithRedisInjectsClient(t *testing.T) {
	t.Run("redis driver adopts the given client instance", func(t *testing.T) {
		mr := miniredis.RunT(t)
		client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
		c, err := New(Config{Driver: "redis", TTL: 60}, WithRedis(client))
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		r, ok := c.(*Redis)
		if !ok {
			t.Fatalf("driver redis built %T, want *Redis", c)
		}
		if r.client != client {
			t.Error("backend must hold the exact client instance passed in")
		}
		// And that client must actually serve the backend.
		ctx := context.Background()
		if err := c.Set(ctx, "k", "v", time.Minute); err != nil {
			t.Fatalf("Set through the injected client: %v", err)
		}
		var got string
		if found, err := c.Get(ctx, "k", &got); err != nil || !found || got != "v" {
			t.Fatalf("Get through the injected client: found=%v value=%q err=%v", found, got, err)
		}
	})

	t.Run("the injected client wins over the config values", func(t *testing.T) {
		// The config points at a dead server; the injected client at a live
		// one. The backend must work, proving the client was used.
		mr := miniredis.RunT(t)
		client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
		c, err := New(Config{
			Driver: "redis",
			Redis:  RedisConfig{Addr: "127.0.0.1:1"},
		}, WithRedis(client))
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		if err := c.Set(context.Background(), "k", "v", time.Minute); err != nil {
			t.Fatalf("Set must go through the injected client, got %v", err)
		}
	})

	t.Run("other drivers ignore the client", func(t *testing.T) {
		mr := miniredis.RunT(t)
		client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
		c, err := New(Config{Driver: "memory"}, WithRedis(client))
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		if _, ok := c.(*Memory); !ok {
			t.Errorf("driver memory must build a *Memory regardless of the client, got %T", c)
		}
	})
}

// TestNewFillsOmittedValues verifies a sparse Config still yields a usable
// backend: ttl falls back to one hour, prefix to the framework namespace and
// the file path to a temporary directory.
func TestNewFillsOmittedValues(t *testing.T) {
	c, err := New(Config{Driver: "file"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	f, ok := c.(*File)
	if !ok {
		t.Fatal("driver file did not build a *File")
	}
	if f.defaultTTL != time.Hour {
		t.Errorf("omitted ttl must default to one hour, got %v", f.defaultTTL)
	}
	if f.prefix != "goyii:cache:" {
		t.Errorf("omitted prefix must default to goyii:cache:, got %q", f.prefix)
	}
	if f.dir == "" {
		t.Error("omitted path must default to a non-empty temporary directory")
	}
}

// TestNewRejectsUnknownDriver verifies a driver the constructor cannot name
// fails construction instead of silently falling back to a backend. Driver
// names are matched case-insensitively with surrounding space ignored.
func TestNewRejectsUnknownDriver(t *testing.T) {
	for _, driver := range []string{"bogus", "memcached"} {
		_, err := New(Config{Driver: driver})
		if err == nil {
			t.Fatalf("driver %q must fail construction", driver)
		}
	}
	_, err := New(Config{Driver: "bogus"})
	if err == nil || !strings.Contains(err.Error(), "unknown driver") {
		t.Fatalf("error = %v, want an unknown-driver error", err)
	}
}
