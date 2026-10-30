package web

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/goyii2/framework/base"
	"github.com/goyii2/framework/session"
)

// TestNewHandlerSessionComesFromAppComponent verifies the handler's session
// manager is the application's session component: a redis backend wired over
// the application's shared connection stores its records on that server.
func TestNewHandlerSessionComesFromAppComponent(t *testing.T) {
	mr := miniredis.RunT(t)
	app := base.New()
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	app.SetRedis(client)

	m, err := session.New(session.Config{Backend: "redis", Timeout: 300}, session.WithRedis(app.Redis()))
	if err != nil {
		t.Fatalf("session.New(redis): %v", err)
	}
	app.SetSession(m)

	h := NewHandler(app)
	if h.SessionManager() != app.Session() {
		t.Fatal("the handler must install the application's session component")
	}

	ctx := context.Background()
	if err := h.SessionManager().Store.Save(ctx, "sess-1", map[string]any{"user": "ada"}, time.Minute); err != nil {
		t.Fatalf("Save through the shared connection: %v", err)
	}
	if !mr.Exists("goyii:session:sess-1") {
		t.Error("session record must land on the shared connection's server")
	}
}

// TestSessionDedicatedConnectionStaysSeparate verifies a session manager the
// assembly wires over its own client (a session-only server) keeps its
// records off the application's shared server.
func TestSessionDedicatedConnectionStaysSeparate(t *testing.T) {
	mrTop := miniredis.RunT(t)
	mrOverride := miniredis.RunT(t)
	app := base.New()
	shared := redis.NewClient(&redis.Options{Addr: mrTop.Addr()})
	t.Cleanup(func() { _ = shared.Close() })
	app.SetRedis(shared)

	own := redis.NewClient(&redis.Options{Addr: mrOverride.Addr()})
	t.Cleanup(func() { _ = own.Close() })
	m, err := session.New(session.Config{Backend: "redis", Timeout: 300}, session.WithRedis(own))
	if err != nil {
		t.Fatalf("session.New(redis, own): %v", err)
	}
	app.SetSession(m)

	ctx := context.Background()
	if err := app.Session().Store.Save(ctx, "sess-2", map[string]any{"user": "ada"}, time.Minute); err != nil {
		t.Fatalf("Save through the dedicated connection: %v", err)
	}
	if !mrOverride.Exists("goyii:session:sess-2") {
		t.Error("session record must land on the dedicated server")
	}
	if mrTop.Exists("goyii:session:sess-2") {
		t.Error("session record must not leak to the shared server")
	}
}
