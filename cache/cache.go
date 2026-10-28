package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// Cache stores values under string keys with a per-entry lifetime and
// optional dependency-based invalidation. It corresponds to Yii2's
// yii\cache\Cache contract.
//
// Values are JSON-encoded, so anything encoding/json accepts round-trips:
// structs, maps, slices, scalars. Get decodes into the destination the caller
// supplies, which keeps type fidelity without a gob-style type registry
// (gob would require registering every cached concrete type before it can
// decode back into an empty interface destination).
//
// A cache miss is (false, nil); an error means the backend itself failed
// (unwritable directory, unreachable redis). All methods are safe for
// concurrent use.
type Cache interface {
	// Get decodes the value stored under key into dest (a pointer, as with
	// json.Unmarshal). found is false for missing or expired entries.
	Get(ctx context.Context, key string, dest any) (found bool, err error)

	// Set stores value under key for ttl. A ttl <= 0 keeps the entry for the
	// backend's default lifetime (see each constructor; one hour from the
	// configuration defaults). Unlike Yii2, where duration 0 means "forever",
	// entries always carry a lifetime — immortal entries are a leak waiting
	// to happen in a long-running process.
	Set(ctx context.Context, key string, value any, ttl time.Duration) error

	// Delete removes key; an absent key is not an error.
	Delete(ctx context.Context, key string) error

	// Exists reports whether key holds a live (not expired) entry without
	// decoding its value.
	Exists(ctx context.Context, key string) (bool, error)

	// Flush removes every entry this cache owns. For the redis backend that
	// is the entries under the key prefix, not the whole database: redis is
	// routinely shared by unrelated components.
	Flush(ctx context.Context) error

	// GetOrSet decodes the value stored under key into dest, or rebuilds it:
	// when the entry is missing, expired or invalidated by dep (nil skips the
	// dependency check), build produces a fresh value that is stored under
	// key and decoded into dest. Rebuilds are not deduplicated — concurrent
	// callers may run build more than once; every one of them still observes
	// a consistent value.
	GetOrSet(ctx context.Context, key string, dest any, build func(context.Context) (any, error), ttl time.Duration, dep Dependency) error
}

// entry is the envelope every backend persists: the JSON-encoded value, the
// dependency fingerprint captured at write time, and the absolute expiry
// time. Memory checks the expiry on read and sweeps it in the background;
// file checks on read; redis relies on the server-side key TTL, with the
// envelope time as a uniform backstop.
type entry struct {
	Value     json.RawMessage `json:"v"`
	Dep       string          `json:"d,omitempty"`
	ExpiresAt int64           `json:"e"` // unix nanoseconds
}

// expired reports whether the entry has outlived its expiry time.
func (e entry) expired(now time.Time) bool {
	return !now.Before(time.Unix(0, e.ExpiresAt))
}

// backend is the union of the public contract with the envelope-level
// operations the shared GetOrSet flow needs. Each backend implements lookup
// and save once; the public methods below assemble the contract on top.
type backend interface {
	Cache
	lookup(ctx context.Context, key string) (entry, bool, error)
	save(ctx context.Context, key string, e entry, ttl time.Duration) error
}

// ttlFor resolves the effective lifetime: an explicit ttl wins, anything
// else falls back to the backend's default.
func ttlFor(ttl, defaultTTL time.Duration) time.Duration {
	if ttl > 0 {
		return ttl
	}
	return defaultTTL
}

// checkKey rejects keys no backend can address.
func checkKey(key string) error {
	if key == "" {
		return fmt.Errorf("cache: empty key")
	}
	return nil
}

// encodeValue marshals value into the envelope's value slot.
func encodeValue(key string, value any) (json.RawMessage, error) {
	b, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("cache: encode value for key %q: %w", key, err)
	}
	return b, nil
}

// decodeInto fills dest with the envelope's value.
func decodeInto(key string, e entry, dest any) error {
	if err := json.Unmarshal(e.Value, dest); err != nil {
		return fmt.Errorf("cache: decode value for key %q: %w", key, err)
	}
	return nil
}

// get is the shared Get: a lookup followed by a decode.
func get(ctx context.Context, b backend, key string, dest any) (bool, error) {
	if err := checkKey(key); err != nil {
		return false, err
	}
	e, ok, err := b.lookup(ctx, key)
	if err != nil || !ok {
		return false, err
	}
	return true, decodeInto(key, e, dest)
}

// set is the shared Set: encode and store under the resolved lifetime.
func set(ctx context.Context, b backend, key string, value any, ttl time.Duration) error {
	if err := checkKey(key); err != nil {
		return err
	}
	v, err := encodeValue(key, value)
	if err != nil {
		return err
	}
	return b.save(ctx, key, entry{Value: v}, ttl)
}

// exists is the shared Exists for backends where existence equals a
// successful lookup (memory, file). The redis backend answers with the
// server-side EXISTS instead.
func exists(ctx context.Context, b backend, key string) (bool, error) {
	if err := checkKey(key); err != nil {
		return false, err
	}
	_, ok, err := b.lookup(ctx, key)
	return ok, err
}

// getOrSet is the shared GetOrSet flow: capture the dependency's current
// state, take the entry, and either decode it (hit, dependency unchanged) or
// rebuild, store and decode the fresh value.
func getOrSet(ctx context.Context, b backend, key string, dest any, build func(context.Context) (any, error), ttl time.Duration, dep Dependency) error {
	if err := checkKey(key); err != nil {
		return err
	}
	var captured string
	if dep != nil {
		fp, err := dep.Capture(ctx)
		if err != nil {
			return fmt.Errorf("cache: capture dependency for key %q: %w", key, err)
		}
		captured = fp
	}
	if e, ok, err := b.lookup(ctx, key); err != nil {
		return err
	} else if ok && (dep == nil || !dep.Changed(ctx, e.Dep)) {
		return decodeInto(key, e, dest)
	}

	value, err := build(ctx)
	if err != nil {
		return fmt.Errorf("cache: build value for key %q: %w", key, err)
	}
	v, err := encodeValue(key, value)
	if err != nil {
		return err
	}
	if err := b.save(ctx, key, entry{Value: v, Dep: captured}, ttl); err != nil {
		return err
	}
	return decodeInto(key, entry{Value: v}, dest)
}
