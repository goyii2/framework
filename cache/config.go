package cache

import (
	"time"

	"github.com/goyii2/framework/config"
	"github.com/redis/go-redis/v9"
)

// NewFromConfig builds the cache component the configuration's cache section
// selects: driver "file" persists under the configured path, "redis" keeps
// entries in redis (address, password and database from the embedded redis
// values), anything else — including "memory" — is the in-process backend.
// The key prefix isolates the cache's keys in shared stores; empty falls
// back to "goyii:cache:".
func NewFromConfig(cfg config.Cache) Cache {
	return NewFromConfigWithRedis(cfg, nil)
}

// NewFromConfigWithRedis builds the cache like NewFromConfig but takes the
// redis connection from client instead of opening one — the application core
// wires the driver this way so every redis-backed component shares one
// connection. A nil client (and any non-redis driver) opens the connection
// from the section's own values, exactly like NewFromConfig.
func NewFromConfigWithRedis(cfg config.Cache, client redis.UniversalClient) Cache {
	ttl := time.Duration(cfg.TTL) * time.Second
	if ttl <= 0 {
		ttl = time.Hour
	}
	prefix := cfg.KeyPrefix
	if prefix == "" {
		prefix = "goyii:cache:"
	}
	switch cfg.Driver {
	case "file":
		return NewFile(cfg.Path, prefix, ttl)
	case "redis":
		if client == nil {
			client = redis.NewClient(&redis.Options{
				Addr:     cfg.Redis.Addr,
				Password: cfg.Redis.Password,
				DB:       cfg.Redis.DB,
			})
		}
		return NewRedis(client, prefix, ttl)
	default:
		return NewMemory(ttl)
	}
}
