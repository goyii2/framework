package session

import (
	"context"
	"time"
)

// Store persists session records between requests. Implementations receive
// the record's remaining lifetime with every call so the lifetime stays owned
// by the session manager; a store may also apply it server-side (redis TTL)
// or check it against the record's age (file modification time). All three
// methods must be safe for concurrent use.
type Store interface {
	// Load returns the stored data for id; ok is false when the id is
	// unknown or its record expired. An expired record is best removed.
	Load(ctx context.Context, id string, ttl time.Duration) (data map[string]any, ok bool, err error)
	// Save stores data under id, replacing any previous record, and
	// refreshes the record's lifetime to ttl.
	Save(ctx context.Context, id string, data map[string]any, ttl time.Duration) error
	// Destroy removes the record for id; removing an unknown id is not an
	// error.
	Destroy(ctx context.Context, id string) error
}
