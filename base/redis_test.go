package base

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/goyii2/framework/config"
)

// TestApplicationRedisSharedComponent verifies the shared connection's
// component semantics: lazily built once, handed out unchanged on every
// access, replaceable, and reset by a nil replacement.
func TestApplicationRedisSharedComponent(t *testing.T) {
	mr := miniredis.RunT(t)
	app := New(config.Config{Redis: config.Redis{Addr: mr.Addr()}})

	if app.redisComp.val.Load() != nil {
		t.Fatal("the redis component must not initialize before first use")
	}

	shared := app.Redis()
	if shared == nil {
		t.Fatal("Redis() returned nil")
	}
	if app.Redis() != shared {
		t.Error("repeated Redis() returned a different instance")
	}
	// An empty section inherits: the resolution must hand out exactly the
	// shared connection, not a second client to the same server.
	if app.RedisFor(config.Redis{}) != shared {
		t.Error("RedisFor with an empty section must return the shared connection instance")
	}
	if app.redisComp.val.Load() == nil {
		t.Error("first access must initialize the shared component")
	}

	// The shared connection is built from the top-level section.
	ctx := context.Background()
	if err := shared.Ping(ctx).Err(); err != nil {
		t.Fatalf("shared connection must reach the configured server: %v", err)
	}

	// Replacement travels to every accessor, including the resolution.
	fake := redis.NewClient(&redis.Options{Addr: miniredis.RunT(t).Addr()})
	app.SetRedis(fake)
	if app.Redis() != fake || app.RedisFor(config.Redis{}) != fake {
		t.Error("SetRedis must replace what Redis and the empty-section resolution return")
	}

	app.SetRedis(nil)
	if app.Redis() == fake {
		t.Error("SetRedis(nil) must restore lazy initialization from the configuration")
	}
	if err := app.Redis().Ping(ctx).Err(); err != nil {
		t.Fatalf("re-built connection must still reach the configured server: %v", err)
	}
}

// TestApplicationCacheRedisInheritsSharedConnection verifies a cache section
// without redis values draws from the top-level section: the cache component
// and the shared connection work against the same server.
func TestApplicationCacheRedisInheritsSharedConnection(t *testing.T) {
	mr := miniredis.RunT(t)
	app := New(config.Config{
		Redis: config.Redis{Addr: mr.Addr()},
		Cache: config.Cache{Driver: "redis", TTL: 60},
	})

	ctx := context.Background()
	if err := app.Cache().Set(ctx, "k", "v", time.Minute); err != nil {
		t.Fatalf("Set through the cache component: %v", err)
	}
	// The shared connection sees the cache's entry: same server, and — with
	// the cache package storing the client it is handed verbatim — same
	// client instance.
	b, err := app.Redis().Get(ctx, "goyii:cache:k").Bytes()
	if err != nil {
		t.Fatalf("cache entry not visible through the shared connection: %v", err)
	}
	if len(b) == 0 {
		t.Fatal("cache entry through the shared connection is empty")
	}
	var got string
	if found, err := app.Cache().Get(ctx, "k", &got); err != nil || !found || got != "v" {
		t.Fatalf("Get through the cache component: found=%v value=%q err=%v", found, got, err)
	}
}

// TestApplicationCacheRedisOverrideOpensOwnConnection verifies a non-empty
// cache redis section overrides the top-level values with a connection of
// its own, leaving the shared connection untouched.
func TestApplicationCacheRedisOverrideOpensOwnConnection(t *testing.T) {
	mrTop := miniredis.RunT(t)
	mrOverride := miniredis.RunT(t)
	app := New(config.Config{
		Redis: config.Redis{Addr: mrTop.Addr()},
		Cache: config.Cache{
			Driver: "redis",
			TTL:    60,
			Redis:  config.Redis{Addr: mrOverride.Addr()},
		},
	})

	ctx := context.Background()
	if err := app.Cache().Set(ctx, "k", "v", time.Minute); err != nil {
		t.Fatalf("Set through the overriding cache component: %v", err)
	}
	if !mrOverride.Exists("goyii:cache:k") {
		t.Error("cache entry must land on the section's own server")
	}
	if mrTop.Exists("goyii:cache:k") {
		t.Error("cache entry must not leak to the top-level server")
	}

	// The shared connection still serves the top-level values and stays a
	// different instance than the cache's dedicated one.
	if err := app.Redis().Set(ctx, "probe:shared", "1", 0).Err(); err != nil {
		t.Fatalf("shared connection must reach the top-level server: %v", err)
	}
	if !mrTop.Exists("probe:shared") || mrOverride.Exists("probe:shared") {
		t.Error("shared connection must keep answering to the top-level server only")
	}
	if client := app.RedisFor(config.Redis{Addr: mrOverride.Addr()}); client == app.Redis() {
		t.Error("a non-empty section must resolve to a dedicated connection, not the shared one")
	}
}

// TestApplicationRedisZeroOverheadWithoutUser verifies the component stays
// unbuilt while nothing redis-backed asks for it: a memory-cache application
// never initializes the connection.
func TestApplicationRedisZeroOverheadWithoutUser(t *testing.T) {
	app := New(config.Config{Cache: config.Cache{Driver: "memory"}})

	_ = app.Cache() // memory driver: must not draw a redis connection
	if app.redisComp.val.Load() != nil {
		t.Error("a memory-cache application must not initialize the redis component")
	}

	// The first redis ask — an inheriting section or direct access — builds it.
	_ = app.RedisFor(config.Redis{})
	if app.redisComp.val.Load() == nil {
		t.Error("the first redis ask must initialize the shared component")
	}
}
