package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Redis keeps entries as one string value per key, with the entry's lifetime
// as the key's server-side TTL (expiry handled by redis, refreshed on every
// write). Any client shape works — single node, cluster or ring — as long as
// it satisfies redis.UniversalClient.
type Redis struct {
	client     redis.UniversalClient
	prefix     string
	defaultTTL time.Duration
}

// NewRedis creates the redis backend. prefix is prepended to every key so
// the cache can share a database with other components (for example
// "goyii:cache:"); Flush removes exactly the keys under this prefix, never
// the whole database.
func NewRedis(client redis.UniversalClient, prefix string, defaultTTL time.Duration) *Redis {
	return &Redis{client: client, prefix: prefix, defaultTTL: defaultTTL}
}

// Get decodes the value under key into dest; missing and expired keys read
// the same (redis drops the latter server-side).
func (r *Redis) Get(ctx context.Context, key string, dest any) (bool, error) {
	return get(ctx, r, key, dest)
}

// Set stores value under key, replacing any previous entry.
func (r *Redis) Set(ctx context.Context, key string, value any, ttl time.Duration) error {
	return set(ctx, r, key, value, ttl)
}

// Delete removes key; an absent key is not an error.
func (r *Redis) Delete(ctx context.Context, key string) error {
	if err := checkKey(key); err != nil {
		return err
	}
	if err := r.client.Del(ctx, r.key(key)).Err(); err != nil {
		return fmt.Errorf("cache: delete key %q: %w", key, err)
	}
	return nil
}

// Exists asks redis whether the key is present; the server-side TTL keeps
// the answer honest about expiry.
func (r *Redis) Exists(ctx context.Context, key string) (bool, error) {
	if err := checkKey(key); err != nil {
		return false, err
	}
	n, err := r.client.Exists(ctx, r.key(key)).Result()
	if err != nil {
		return false, fmt.Errorf("cache: check key %q: %w", key, err)
	}
	return n > 0, nil
}

// Flush removes every key under the cache's prefix — redis databases are
// routinely shared, so FLUSHDB is off the table. The scan-and-delete walk is
// not atomic: keys written concurrently may survive a flush racing them.
func (r *Redis) Flush(ctx context.Context) error {
	var keys []string
	iter := r.client.Scan(ctx, 0, r.prefix+"*", 100).Iterator()
	for iter.Next(ctx) {
		keys = append(keys, iter.Val())
	}
	if err := iter.Err(); err != nil {
		return fmt.Errorf("cache: flush prefix %q: %w", r.prefix, err)
	}
	if len(keys) == 0 {
		return nil
	}
	if err := r.client.Del(ctx, keys...).Err(); err != nil {
		return fmt.Errorf("cache: flush prefix %q: %w", r.prefix, err)
	}
	return nil
}

// GetOrSet decodes the entry under key into dest, rebuilding it with build
// when missing, expired or invalidated by dep.
func (r *Redis) GetOrSet(ctx context.Context, key string, dest any, build func(context.Context) (any, error), ttl time.Duration, dep Dependency) error {
	return getOrSet(ctx, r, key, dest, build, ttl, dep)
}

// lookup reads and decodes the entry; the envelope's expiry time is checked
// as a backstop, though the server-side TTL normally removes the key first.
func (r *Redis) lookup(ctx context.Context, key string) (entry, bool, error) {
	b, err := r.client.Get(ctx, r.key(key)).Bytes()
	if err == redis.Nil {
		return entry{}, false, nil
	}
	if err != nil {
		return entry{}, false, fmt.Errorf("cache: read key %q: %w", key, err)
	}
	var e entry
	if err := json.Unmarshal(b, &e); err != nil {
		return entry{}, false, fmt.Errorf("cache: decode entry for key %q: %w", key, err)
	}
	if e.expired(time.Now()) {
		return entry{}, false, nil
	}
	return e, true, nil
}

// save writes the entry with the resolved lifetime as the key's TTL.
func (r *Redis) save(ctx context.Context, key string, e entry, ttl time.Duration) error {
	e.ExpiresAt = time.Now().Add(ttlFor(ttl, r.defaultTTL)).UnixNano()
	b, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("cache: encode entry for key %q: %w", key, err)
	}
	if err := r.client.Set(ctx, r.key(key), b, ttlFor(ttl, r.defaultTTL)).Err(); err != nil {
		return fmt.Errorf("cache: write key %q: %w", key, err)
	}
	return nil
}

// key maps a cache key to its redis key.
func (r *Redis) key(key string) string {
	return r.prefix + key
}
