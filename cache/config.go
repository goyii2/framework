package cache

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// Config carries the cache component values. The zero value selects the
// in-process memory backend; every other driver is named explicitly.
type Config struct {
	Driver    string `yaml:"driver"`    // memory | file | redis; empty = memory
	KeyPrefix string `yaml:"keyPrefix"` // key namespace for the file and redis backends (memory is instance-local)
	TTL       int    `yaml:"ttl"`       // seconds; default lifetime when a caller passes ttl <= 0
	Path      string `yaml:"path"`      // file backend storage directory

	// RedisConfig carries the connection values the redis driver opens for
	// itself when no client is injected with WithRedis. Empty values fall
	// back to go-redis's own defaults (localhost).
	Redis RedisConfig `yaml:"redis"`
}

// RedisConfig holds connection values for a dedicated redis connection of
// the redis backend. A connection shared with the rest of the application is
// injected with WithRedis instead and ignores these values.
type RedisConfig struct {
	Addr     string `yaml:"addr"`
	Password string `yaml:"password"`
	DB       int    `yaml:"db"`
}

// Option customizes cache construction; options carry cross-component
// dependencies, values live in Config.
type Option func(*options)

// options accumulates what the options set.
type options struct {
	redis redis.UniversalClient
}

// WithRedis hands the cache a redis connection to use for the redis driver —
// typically the application's shared connection, so every redis-backed
// component talks to the same server over one client. Without it the redis
// driver opens its own connection from the Config's redis values. Other
// drivers ignore the client.
func WithRedis(client redis.UniversalClient) Option {
	return func(o *options) { o.redis = client }
}

// New builds the cache Config selects: driver "file" persists under the
// configured path, "redis" keeps entries in redis, "memory" (also the empty
// default) stays in-process. Omitted values fall back to the defaults: a
// one-hour default lifetime, the "goyii:cache:" key prefix, and the
// operating system's temporary directory for the file backend. A driver New
// cannot name returns an error instead of a component.
func New(cfg Config, opts ...Option) (Cache, error) {
	var o options
	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}
	ttl := time.Duration(cfg.TTL) * time.Second
	if ttl <= 0 {
		ttl = time.Hour
	}
	prefix := cfg.KeyPrefix
	if prefix == "" {
		prefix = "goyii:cache:"
	}
	switch strings.ToLower(strings.TrimSpace(cfg.Driver)) {
	case "", "memory":
		return NewMemory(ttl), nil
	case "file":
		path := cfg.Path
		if path == "" {
			path = filepath.Join(os.TempDir(), "goyii-cache")
		}
		return NewFile(path, prefix, ttl), nil
	case "redis":
		client := o.redis
		if client == nil {
			client = redis.NewClient(&redis.Options{
				Addr:     cfg.Redis.Addr,
				Password: cfg.Redis.Password,
				DB:       cfg.Redis.DB,
			})
		}
		return NewRedis(client, prefix, ttl), nil
	default:
		return nil, fmt.Errorf("cache: unknown driver %q (want memory, file or redis)", cfg.Driver)
	}
}
