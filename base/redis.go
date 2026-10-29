package base

import (
	"github.com/redis/go-redis/v9"

	"github.com/goyii2/framework/config"
)

// Redis returns the application's shared redis connection, initializing it
// on first access from the top-level redis section of the configuration.
// One connection serves every redis-backed component; components that
// configure their own server (a cache or session section override) draw
// dedicated connections instead, see RedisFor. go-redis dials lazily on
// first use, so applications that never touch redis never open one.
func (a *Application) Redis() redis.UniversalClient { return a.redisComp.get() }

// SetRedis replaces the shared redis connection (e.g. a test fake or a
// cluster client); nil restores lazy initialization from the configuration.
func (a *Application) SetRedis(c redis.UniversalClient) { a.redisComp.set(c) }

// RedisFor resolves the connection a redis-backed configuration section
// works with: the section's own values when it carries any — a dedicated
// connection, since a different server cannot share one — and the shared
// connection when it carries none and inherits the top-level values. The
// cache and session wiring call this with their section; components that
// share unconditionally (mutex, queue) use Redis.
func (a *Application) RedisFor(section config.Redis) redis.UniversalClient {
	return redisClientFor(section, a.redisComp)
}

// redisClientFor is the resolution behind RedisFor: an empty section takes
// the shared component's connection, a non-empty section opens its own.
func redisClientFor(section config.Redis, shared *component[redis.UniversalClient]) redis.UniversalClient {
	if section == (config.Redis{}) {
		return shared.get()
	}
	return redis.NewClient(&redis.Options{
		Addr:     section.Addr,
		Password: section.Password,
		DB:       section.DB,
	})
}

// newDefaultRedis builds the shared connection from the top-level redis
// section; an unconfigured section keeps go-redis's own defaults
// (localhost), matching what the per-section connections did before the
// shared component existed.
func newDefaultRedis(cfg config.Redis) redis.UniversalClient {
	return redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	})
}
