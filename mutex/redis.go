package mutex

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// defaultPrefix namespaces the lock keys so the mutex can share a redis
// database with caches, sessions and the like.
const defaultPrefix = "goyii:mutex:"

// releaseScript frees the lock only when the stored token is the caller's
// own: a release racing its own ttl expiry — or landing after a successor
// has already re-acquired the expired name — finds a foreign token under
// the key and deletes nothing.
var releaseScript = redis.NewScript(`
if redis.call("get", KEYS[1]) == ARGV[1] then
	return redis.call("del", KEYS[1])
else
	return 0
end`)

// Redis is the cross-process driver. A lock is one redis key set with
// SET NX PX — value a freshly generated token, lifetime the requested ttl —
// so the server frees the lock by key expiry no matter what happens to the
// holder. Release compares the token in Lua before deleting, which keeps it
// idempotent and prevents a stale holder from freeing a successor's lock.
// Any client shape works — single node, cluster or ring — as long as it
// satisfies redis.UniversalClient.
type Redis struct {
	client     redis.UniversalClient
	prefix     string
	retryDelay time.Duration // pause between acquisition attempts in Lock
}

// NewRedis creates the redis driver over client. prefix is prepended to
// every lock name ("goyii:mutex:" when empty) so independent mutex
// deployments can share one database without sharing locks.
func NewRedis(client redis.UniversalClient, prefix string) *Redis {
	if prefix == "" {
		prefix = defaultPrefix
	}
	return &Redis{client: client, prefix: prefix, retryDelay: 50 * time.Millisecond}
}

// Lock acquires name for at most ttl, polling while another holder keeps
// the key. See the Mutex interface for the full contract.
func (r *Redis) Lock(ctx context.Context, name string, ttl time.Duration) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := checkAcquire(name, ttl); err != nil {
		return nil, err
	}
	token, err := newToken()
	if err != nil {
		return nil, err
	}
	key := r.prefix + name
	for {
		ok, err := r.client.SetNX(ctx, key, token, ttl).Result()
		if err != nil {
			return nil, fmt.Errorf("mutex: acquire %q: %w", name, err)
		}
		if ok {
			return r.releaseOnce(key, token), nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(r.retryDelay):
		}
	}
}

// TryLock acquires name for at most ttl with a single SET NX PX attempt:
// a held name fails with ErrLocked at once. See the Mutex interface for the
// full contract.
func (r *Redis) TryLock(ctx context.Context, name string, ttl time.Duration) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := checkAcquire(name, ttl); err != nil {
		return nil, err
	}
	token, err := newToken()
	if err != nil {
		return nil, err
	}
	key := r.prefix + name
	ok, err := r.client.SetNX(ctx, key, token, ttl).Result()
	if err != nil {
		return nil, fmt.Errorf("mutex: acquire %q: %w", name, err)
	}
	if !ok {
		return nil, ErrLocked
	}
	return r.releaseOnce(key, token), nil
}

// releaseOnce returns the idempotent release of one acquisition. Release
// runs on a background context — the acquiring request may be long gone —
// and its error is dropped: with no caller to receive it, and the ttl
// bounding how long a failed release can matter, swallowing is the honest
// option.
func (r *Redis) releaseOnce(key, token string) func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			_ = releaseScript.Run(context.Background(), r.client, []string{key}, token).Err()
		})
	}
}

// newToken generates the acquisition token: random per acquisition, so two
// successive holders of the same name are distinguishable under the key.
func newToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("mutex: generate token: %w", err)
	}
	return hex.EncodeToString(b), nil
}
