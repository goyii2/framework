package session

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisStore persists session records as JSON values in redis, with the
// record's lifetime as the key's TTL (expiry handled server-side, and
// refreshed on every save). Any redis client shape works — single node,
// cluster or ring — as long as it satisfies redis.UniversalClient.
type RedisStore struct {
	client redis.UniversalClient
	prefix string
}

// NewRedisStore creates the redis backend. prefix is prepended to every
// record key so sessions can share a database with other components (for
// example "goyii:session:").
func NewRedisStore(client redis.UniversalClient, prefix string) *RedisStore {
	return &RedisStore{client: client, prefix: prefix}
}

// Load returns the record for id; expired keys are gone server-side, which
// surfaces as a missing id here.
func (s *RedisStore) Load(ctx context.Context, id string, ttl time.Duration) (map[string]any, bool, error) {
	b, err := s.client.Get(ctx, s.key(id)).Bytes()
	if err == redis.Nil {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	data, err := unmarshalSession(b)
	if err != nil {
		return nil, false, err
	}
	return data, data != nil, nil
}

// Save replaces the record for id with a ttl matching the record's remaining
// lifetime.
func (s *RedisStore) Save(ctx context.Context, id string, data map[string]any, ttl time.Duration) error {
	b, err := marshalSession(data)
	if err != nil {
		return err
	}
	return s.client.Set(ctx, s.key(id), b, ttl).Err()
}

// Destroy removes the record for id; absent records are not an error.
func (s *RedisStore) Destroy(ctx context.Context, id string) error {
	return s.client.Del(ctx, s.key(id)).Err()
}

// key maps a session id to its redis key.
func (s *RedisStore) key(id string) string {
	return s.prefix + id
}
