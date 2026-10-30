package base

import (
	"fmt"
	"os"

	"github.com/redis/go-redis/v9"

	"github.com/goyii2/framework/cache"
	"github.com/goyii2/framework/config"
)

// Cache returns the cache component, building it from the configuration's
// cache section on first access. The value is the cache package's Cache
// interface — memory, file or redis behind one contract — so request code
// depends on the contract, never a backend; tests replace it with SetCache.
func (a *Application) Cache() cache.Cache { return a.cacheComp.get() }

// SetCache replaces the cache component (e.g. a test fake or a pre-wired
// backend); nil restores lazy initialization from the configuration.
func (a *Application) SetCache(c cache.Cache) { a.cacheComp.set(c) }

// newDefaultCache builds the default cache component from the cache
// configuration section. The redis driver draws its connection through the
// application's shared redis resolution: an empty cache redis section
// inherits the shared connection (top-level section), a non-empty one opens
// a dedicated connection of its own. A section the constructor rejects (an
// unknown driver) is reported on stderr and degrades to the memory backend
// so the application still starts.
func newDefaultCache(cfg config.Cache, shared *component[redis.UniversalClient]) cache.Cache {
	var client redis.UniversalClient
	if cfg.Driver == "redis" {
		client = redisClientFor(cfg.Redis, shared)
	}
	c, err := cache.New(cache.Config{
		Driver:    cfg.Driver,
		KeyPrefix: cfg.KeyPrefix,
		TTL:       cfg.TTL,
		Path:      cfg.Path,
		Redis:     cache.RedisConfig(cfg.Redis),
	}, cache.WithRedis(client))
	if err != nil {
		fmt.Fprintf(os.Stderr, "goyii: %v; falling back to the memory cache\n", err)
		// The memory backend is the zero-value driver, so this fallback
		// cannot fail; the configured lifetime is still honored.
		fallback, _ := cache.New(cache.Config{TTL: cfg.TTL})
		return fallback
	}
	return c
}
