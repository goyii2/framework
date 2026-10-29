package cache

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/goyii2/framework/config"
)

// TestNewFromConfigSelectsDriver verifies the factory maps each driver value
// to its backend with the configured values applied.
func TestNewFromConfigSelectsDriver(t *testing.T) {
	t.Run("memory is the default", func(t *testing.T) {
		for _, driver := range []string{"", "memory"} {
			if _, ok := NewFromConfig(config.Cache{Driver: driver}).(*Memory); !ok {
				t.Errorf("driver %q did not build a *Memory", driver)
			}
		}
	})

	t.Run("file applies path prefix and ttl", func(t *testing.T) {
		dir := t.TempDir()
		c := NewFromConfig(config.Cache{Driver: "file", Path: dir, KeyPrefix: "p:", TTL: 60})
		f, ok := c.(*File)
		if !ok {
			t.Fatalf("driver file built %T, want *File", c)
		}
		if f.dir != dir || f.prefix != "p:" || f.defaultTTL != time.Minute {
			t.Errorf("file backend wiring: dir=%q prefix=%q ttl=%v", f.dir, f.prefix, f.defaultTTL)
		}
	})

	t.Run("redis connects from the embedded values", func(t *testing.T) {
		mr := miniredis.RunT(t)
		c := NewFromConfig(config.Cache{
			Driver: "redis",
			Redis:  config.Redis{Addr: mr.Addr()},
			TTL:    60,
		})
		r, ok := c.(*Redis)
		if !ok {
			t.Fatalf("driver redis built %T, want *Redis", c)
		}
		if r.prefix != "goyii:cache:" {
			t.Errorf("empty KeyPrefix must fall back to goyii:cache:, got %q", r.prefix)
		}
		// The connection the factory built must actually carry entries.
		ctx := context.Background()
		if err := c.Set(ctx, "k", "v", time.Minute); err != nil {
			t.Fatalf("Set through the factory-built backend: %v", err)
		}
		var got string
		if found, err := c.Get(ctx, "k", &got); err != nil || !found || got != "v" {
			t.Fatalf("Get through the factory-built backend: found=%v value=%q err=%v", found, got, err)
		}
	})
}

// TestNewFromConfigWithRedisUsesGivenClient verifies the shared-connection
// factory variant: the redis driver adopts the exact client handed in (one
// connection shared with every other redis-backed component), a nil client
// keeps the open-your-own behavior of NewFromConfig, and the other drivers
// ignore the client entirely.
func TestNewFromConfigWithRedisUsesGivenClient(t *testing.T) {
	t.Run("redis driver adopts the given client instance", func(t *testing.T) {
		mr := miniredis.RunT(t)
		client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
		c := NewFromConfigWithRedis(config.Cache{Driver: "redis", TTL: 60}, client)
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

	t.Run("nil client opens its own from the section values", func(t *testing.T) {
		mr := miniredis.RunT(t)
		c := NewFromConfigWithRedis(config.Cache{
			Driver: "redis",
			Redis:  config.Redis{Addr: mr.Addr()},
			TTL:    60,
		}, nil)
		ctx := context.Background()
		if err := c.Set(ctx, "k", "v", time.Minute); err != nil {
			t.Fatalf("Set through the self-opened client: %v", err)
		}
		var got string
		if found, err := c.Get(ctx, "k", &got); err != nil || !found || got != "v" {
			t.Fatalf("Get through the self-opened client: found=%v value=%q err=%v", found, got, err)
		}
	})

	t.Run("other drivers ignore the client", func(t *testing.T) {
		mr := miniredis.RunT(t)
		client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
		if _, ok := NewFromConfigWithRedis(config.Cache{Driver: "memory"}, client).(*Memory); !ok {
			t.Error("driver memory must build a *Memory regardless of the client")
		}
	})
}

// TestNewFromConfigFillsOmittedValues verifies a sparse cache section still
// yields a usable backend: ttl falls back to one hour, prefix to the
// framework namespace.
func TestNewFromConfigFillsOmittedValues(t *testing.T) {
	dir := t.TempDir()
	f, ok := NewFromConfig(config.Cache{Driver: "file", Path: dir}).(*File)
	if !ok {
		t.Fatal("driver file did not build a *File")
	}
	if f.defaultTTL != time.Hour {
		t.Errorf("omitted ttl must default to one hour, got %v", f.defaultTTL)
	}
	if f.prefix != "goyii:cache:" {
		t.Errorf("omitted prefix must default to goyii:cache:, got %q", f.prefix)
	}
}

// TestConfigDefaultsApplyToCache verifies the configuration defaults fill
// the cache section the factory consumes.
func TestConfigDefaultsApplyToCache(t *testing.T) {
	cfg := config.Default().Cache
	if cfg.Driver != "memory" {
		t.Errorf("default cache driver = %q, want memory", cfg.Driver)
	}
	if cfg.TTL != 3600 {
		t.Errorf("default cache ttl = %d, want 3600", cfg.TTL)
	}
	if cfg.Path == "" {
		t.Error("default cache path must be filled")
	}
}
