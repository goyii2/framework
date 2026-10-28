package cache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

// Dependency invalidates cache entries when the thing they were built from
// has changed. It corresponds to Yii2's yii\caching\Dependency: the
// dependency's state is captured as a fingerprint when an entry is stored;
// GetOrSet re-evaluates it on every read and rebuilds the entry once the
// fingerprint no longer matches. Dependencies are immutable value objects —
// construct a fresh one carrying the current state at each call site.
type Dependency interface {
	// Capture returns a fingerprint of the dependency's current state; it is
	// stored alongside the cached value at write time.
	Capture(ctx context.Context) (string, error)

	// Changed reports whether the dependency has changed since fingerprint
	// was captured, making entries stored with that fingerprint stale. An
	// empty or unparsable fingerprint — an entry written without this
	// dependency — counts as changed. A dependency whose current state cannot
	// be captured also counts as changed: never serve a value whose
	// validity cannot be judged.
	Changed(ctx context.Context, fingerprint string) bool
}

// ValueDependency invalidates an entry when the value it depends on has
// changed since the entry was stored — a table version counter, a config
// revision, a max(updated_at) timestamp. It corresponds to Yii2's
// yii\caching\ValueDependency.
//
// Construct it with the current value on every GetOrSet call:
//
//	dep := cache.NewValueDependency(currentVersion(ctx))
//	err := c.GetOrSet(ctx, key, &stats, build, ttl, dep)
//
// A dependency reused while its underlying value changes never notices:
// its captured value and the stored fingerprint change together.
type ValueDependency struct {
	value any
}

// NewValueDependency captures v as the state the cached entry depends on.
func NewValueDependency(v any) *ValueDependency {
	return &ValueDependency{value: v}
}

// Capture fingerprints the current value: its JSON encoding (deterministic —
// map keys are sorted) hashed to a compact string.
func (d *ValueDependency) Capture(ctx context.Context) (string, error) {
	b, err := json.Marshal(d.value)
	if err != nil {
		return "", fmt.Errorf("cache: capture value dependency: %w", err)
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

// Changed compares the current fingerprint with the stored one.
func (d *ValueDependency) Changed(ctx context.Context, fingerprint string) bool {
	current, err := d.Capture(ctx)
	if err != nil {
		return true
	}
	return current != fingerprint
}

// TimeDependency invalidates an entry at a fixed wall-clock time — data
// rebuilt on a schedule (nightly imports, daily digests) instead of after a
// lifetime. A ttl still applies and may expire the entry first; whichever
// boundary passes sooner wins.
//
// Construct it from a quantized schedule (for example "the next midnight") so
// independently constructed dependencies agree within a window: two
// dependencies carrying different times invalidate each other's entries,
// and a time derived from a bare `time.Now()` never agrees with an earlier
// construction. Once a window's time has passed the dependency stays changed
// until a later window is supplied — the rebuild site derives the next
// window's time when constructing the dependency for the next call.
type TimeDependency struct {
	expireAt time.Time
}

// NewTimeDependency invalidates entries the moment expireAt is reached.
func NewTimeDependency(expireAt time.Time) *TimeDependency {
	return &TimeDependency{expireAt: expireAt}
}

// Capture records the expiry time itself as the fingerprint, so the stored
// fingerprint survives reconstruction of an equal dependency.
func (d *TimeDependency) Capture(ctx context.Context) (string, error) {
	return d.expireAt.Format(time.RFC3339Nano), nil
}

// Changed reports the entry stale when the wall clock has passed this
// dependency's expiry time, or when the stored fingerprint names a different
// time than this dependency carries (the schedule moved): the caller's
// current statement of validity governs, a mismatch always rebuilds.
func (d *TimeDependency) Changed(ctx context.Context, fingerprint string) bool {
	if !time.Now().Before(d.expireAt) {
		return true
	}
	current, err := d.Capture(ctx)
	return err != nil || current != fingerprint
}
