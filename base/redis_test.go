package base

import (
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// TestApplicationRedisSlot verifies the redis slot's replaceability; the
// not-wired panic behavior lives in application_test.go.
func TestApplicationRedisSlot(t *testing.T) {
	mr := miniredis.RunT(t)
	app := New()

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	app.SetRedis(client)
	if app.Redis() != client {
		t.Error("SetRedis must install the client Redis returns")
	}
}
