package mutex

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// TestNewDispatch verifies the driver names the constructor accepts: the
// empty default and "process" build the process driver, "redis" the redis
// driver over the injected connection (case and surrounding space in the
// driver name are tolerated).
func TestNewDispatch(t *testing.T) {
	client := redis.NewClient(&redis.Options{Addr: miniredis.RunT(t).Addr()})

	for _, driver := range []string{"", "process", " Process "} {
		m, err := New(Config{Driver: driver})
		if err != nil {
			t.Fatalf("driver %q: %v", driver, err)
		}
		if _, ok := m.(*Process); !ok {
			t.Errorf("driver %q built %T, want *Process", driver, m)
		}
	}

	m, err := New(Config{Driver: " redis "}, WithRedis(client))
	if err != nil {
		t.Fatalf("driver redis: %v", err)
	}
	r, ok := m.(*Redis)
	if !ok {
		t.Fatalf("driver redis built %T, want *Redis", m)
	}
	if r.prefix != defaultPrefix {
		t.Errorf("redis driver prefix = %q, want the default %q", r.prefix, defaultPrefix)
	}

	// The injected connection must actually serve the driver.
	release, err := r.Lock(context.Background(), "job", time.Minute)
	if err != nil {
		t.Fatalf("Lock through the constructed driver: %v", err)
	}
	release()
}

// TestNewRedisDriverRequiresConnection verifies the redis driver fails
// construction without a connection: there are no connection values in the
// mutex Config to open one from, and a redis mutex that cannot reach redis
// must not exist.
func TestNewRedisDriverRequiresConnection(t *testing.T) {
	for name, opts := range map[string][]Option{
		"no option":      nil,
		"nil connection": {WithRedis(nil)},
	} {
		_, err := New(Config{Driver: "redis"}, opts...)
		if err == nil || !strings.Contains(err.Error(), "WithRedis") {
			t.Fatalf("%s: error = %v, want a missing-connection error naming WithRedis", name, err)
		}
	}
}

// TestNewRejectsUnknownDriver verifies a driver the constructor cannot name
// fails construction instead of yielding a component that errors later.
func TestNewRejectsUnknownDriver(t *testing.T) {
	_, err := New(Config{Driver: "bogus"})
	if err == nil || !strings.Contains(err.Error(), "unknown driver") {
		t.Fatalf("error = %v, want an unknown-driver error", err)
	}
}
