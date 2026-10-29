package mutex

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/goyii2/framework/config"
)

// NewFromConfig builds the mutex the configuration's mutex section selects:
// driver "redis" keeps the locks in redis over client (the application core
// passes the shared connection this way), anything else — including
// "process" and the empty default — is the in-process driver. A driver the
// section cannot name yields a mutex whose acquisitions fail with a
// configuration error rather than a nil component, so the mistake surfaces
// at first use instead of breaking application assembly.
func NewFromConfig(cfg config.Mutex, client redis.UniversalClient) Mutex {
	switch strings.ToLower(strings.TrimSpace(cfg.Driver)) {
	case "", "process":
		return NewProcess()
	case "redis":
		return NewRedis(client, "")
	default:
		return failingMutex{fmt.Errorf("mutex: unknown driver %q (want process or redis)", cfg.Driver)}
	}
}

// failingMutex reports one fixed configuration error from every acquisition.
type failingMutex struct{ err error }

func (f failingMutex) Lock(context.Context, string, time.Duration) (func(), error) { return nil, f.err }
func (f failingMutex) TryLock(context.Context, string, time.Duration) (func(), error) {
	return nil, f.err
}
