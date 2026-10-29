package web

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"

	"github.com/goyii2/framework/base"
	"github.com/goyii2/framework/config"
)

// TestNewHandlerSessionRedisSharesApplicationConnection verifies the redis
// session backend draws from the application's shared connection: a session
// section without redis values inherits the top-level section, and the
// handler-installed store holds the exact shared client instance.
func TestNewHandlerSessionRedisSharesApplicationConnection(t *testing.T) {
	mr := miniredis.RunT(t)
	app := base.New(config.Config{
		Redis: config.Redis{Addr: mr.Addr()},
		Cache: config.Cache{Driver: "redis"},
		Session: config.Session{
			Backend: "redis",
			Timeout: 300,
		},
	})

	h := NewHandler(app)
	store, ok := h.SessionManager().Store.(*RedisSessionStore)
	if !ok {
		t.Fatalf("session backend = %T, want *RedisSessionStore", h.SessionManager().Store)
	}
	if store.client != app.Redis() {
		t.Error("session store must hold the application's shared connection instance")
	}

	// Session records ride the shared connection's server.
	ctx := context.Background()
	if err := store.Save(ctx, "sess-1", map[string]any{"user": "ada"}, time.Minute); err != nil {
		t.Fatalf("Save through the shared connection: %v", err)
	}
	if !mr.Exists("goyii:session:sess-1") {
		t.Error("session record must land on the shared connection's server")
	}

	// The cache on the same application shares that connection too: its
	// entries land on the same server alongside the session records.
	if err := app.Cache().Set(ctx, "k", "v", time.Minute); err != nil {
		t.Fatalf("Set through the cache component: %v", err)
	}
	if !mr.Exists("goyii:cache:k") {
		t.Error("cache entry must land on the shared connection's server")
	}
}

// TestNewHandlerSessionRedisOverrideUsesDedicatedConnection verifies a
// non-empty session redis section overrides the top-level values with a
// connection of its own, distinct from the shared one.
func TestNewHandlerSessionRedisOverrideUsesDedicatedConnection(t *testing.T) {
	mrTop := miniredis.RunT(t)
	mrOverride := miniredis.RunT(t)
	app := base.New(config.Config{
		Redis: config.Redis{Addr: mrTop.Addr()},
		Session: config.Session{
			Backend: "redis",
			Timeout: 300,
			Redis:   config.Redis{Addr: mrOverride.Addr()},
		},
	})

	h := NewHandler(app)
	store, ok := h.SessionManager().Store.(*RedisSessionStore)
	if !ok {
		t.Fatalf("session backend = %T, want *RedisSessionStore", h.SessionManager().Store)
	}
	if store.client == app.Redis() {
		t.Error("an overriding session section must not use the shared connection")
	}

	ctx := context.Background()
	if err := store.Save(ctx, "sess-2", map[string]any{"user": "ada"}, time.Minute); err != nil {
		t.Fatalf("Save through the dedicated connection: %v", err)
	}
	if !mrOverride.Exists("goyii:session:sess-2") {
		t.Error("session record must land on the section's own server")
	}
	if mrTop.Exists("goyii:session:sess-2") {
		t.Error("session record must not leak to the top-level server")
	}
}

// TestNewSessionManagerWithRedisNilClientKeepsOwnConnection verifies the
// legacy construction shape: without an injected client the redis backend
// opens its own connection from the session section, as NewSessionManager
// always did.
func TestNewSessionManagerWithRedisNilClientKeepsOwnConnection(t *testing.T) {
	mr := miniredis.RunT(t)
	m := NewSessionManagerWithRedis(config.Session{
		Backend: "redis",
		Redis:   config.Redis{Addr: mr.Addr()},
	}, nil)
	store, ok := m.Store.(*RedisSessionStore)
	if !ok {
		t.Fatalf("session backend = %T, want *RedisSessionStore", m.Store)
	}
	ctx := context.Background()
	if err := store.Save(ctx, "sess-3", map[string]any{"k": "v"}, time.Minute); err != nil {
		t.Fatalf("Save through the self-opened connection: %v", err)
	}
	if !mr.Exists("goyii:session:sess-3") {
		t.Error("session record must land on the section's configured server")
	}
}
