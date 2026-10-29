package mutex

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/goyii2/framework/config"
)

// TestNewFromConfigDispatch verifies the driver names the mutex section
// accepts: the empty default and "process" build the process driver,
// "redis" the redis driver over the given connection, anything else fails
// at first use with a configuration error.
func TestNewFromConfigDispatch(t *testing.T) {
	client := redis.NewClient(&redis.Options{Addr: miniredis.RunT(t).Addr()})

	if _, ok := NewFromConfig(config.Mutex{}, client).(*Process); !ok {
		t.Error("empty driver must select the process mutex")
	}
	if _, ok := NewFromConfig(config.Mutex{Driver: "process"}, client).(*Process); !ok {
		t.Error("driver \"process\" must select the process mutex")
	}
	m, ok := NewFromConfig(config.Mutex{Driver: " redis "}, client).(*Redis)
	if !ok {
		t.Fatal("driver \"redis\" must select the redis mutex")
	}
	if m.prefix != defaultPrefix {
		t.Errorf("redis driver prefix = %q, want the default %q", m.prefix, defaultPrefix)
	}

	failing := NewFromConfig(config.Mutex{Driver: "bogus"}, client)
	if rel, err := failing.Lock(context.Background(), "job", time.Minute); err == nil || !strings.Contains(err.Error(), "unknown driver") {
		rel()
		t.Fatalf("Lock through a bad driver: err=%v, want unknown-driver error", err)
	}
	if rel, err := failing.TryLock(context.Background(), "job", time.Minute); err == nil || !strings.Contains(err.Error(), "unknown driver") {
		rel()
		t.Fatalf("TryLock through a bad driver: err=%v, want unknown-driver error", err)
	}
}
