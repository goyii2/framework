package mutex

import (
	"fmt"
	"strings"

	"github.com/redis/go-redis/v9"
)

// Config carries the mutex component values. The zero value selects the
// in-process driver; the redis driver additionally requires a connection
// injected with WithRedis.
type Config struct {
	Driver string `yaml:"driver"` // process | redis; empty = process
}

// Option customizes mutex construction; options carry cross-component
// dependencies, values live in Config.
type Option func(*options)

// options accumulates what the options set.
type options struct {
	redis redis.UniversalClient
}

// WithRedis hands the mutex the redis connection the redis driver locks
// through — the application's shared connection, so every process locking
// the same name on the same server coordinates through one key space. The
// process driver (the default) ignores the client.
func WithRedis(client redis.UniversalClient) Option {
	return func(o *options) { o.redis = client }
}

// New builds the mutex Config selects: "process" (also the empty default)
// locks inside one process, "redis" locks across processes over the injected
// connection. The redis driver has no connection values of its own — a mutex
// coordinates processes, so a private server nobody else locks on would be a
// mistake — and a redis driver without a connection fails construction, as
// does a driver New cannot name.
func New(cfg Config, opts ...Option) (Mutex, error) {
	var o options
	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}
	switch strings.ToLower(strings.TrimSpace(cfg.Driver)) {
	case "", "process":
		return NewProcess(), nil
	case "redis":
		if o.redis == nil {
			return nil, fmt.Errorf("mutex: redis driver requires a redis connection (pass one with WithRedis)")
		}
		return NewRedis(o.redis, ""), nil
	default:
		return nil, fmt.Errorf("mutex: unknown driver %q (want process or redis)", cfg.Driver)
	}
}
