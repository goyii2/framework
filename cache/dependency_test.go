package cache

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// TestDependencyInvalidation runs the dependency flow over all backends: an
// entry stored under a ValueDependency survives reads while the dependency
// holds, and rebuilds once the dependency's value changes.
func TestDependencyInvalidation(t *testing.T) {
	for _, tc := range cacheCases(t) {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			c := tc.cache
			builds := 0
			build := func(context.Context) (any, error) {
				builds++
				return builds, nil // each rebuild yields a new value
			}

			// Store under version 1 of the dependency.
			var got int
			if err := c.GetOrSet(ctx, "dep-key", &got, build, tc.liveTTL, NewValueDependency(1)); err != nil {
				t.Fatalf("GetOrSet initial: %v", err)
			}
			if got != 1 || builds != 1 {
				t.Fatalf("initial GetOrSet: dest=%d builds=%d, want 1/1", got, builds)
			}

			// Same dependency value: the entry is served, no rebuild.
			if err := c.GetOrSet(ctx, "dep-key", &got, build, tc.liveTTL, NewValueDependency(1)); err != nil {
				t.Fatalf("GetOrSet unchanged dependency: %v", err)
			}
			if got != 1 || builds != 1 {
				t.Fatalf("unchanged dependency must hit the cache: dest=%d builds=%d, want 1/1", got, builds)
			}

			// Dependency value moved to 2: the entry is stale, rebuilt.
			if err := c.GetOrSet(ctx, "dep-key", &got, build, tc.liveTTL, NewValueDependency(2)); err != nil {
				t.Fatalf("GetOrSet changed dependency: %v", err)
			}
			if got != 2 || builds != 2 {
				t.Fatalf("changed dependency must rebuild: dest=%d builds=%d, want 2/2", got, builds)
			}

			// Plain Set (no dependency) then GetOrSet with a dependency: the
			// stored entry carries no fingerprint, so it reads as stale.
			if err := c.Set(ctx, "plain-key", "raw", tc.liveTTL); err != nil {
				t.Fatalf("Set: %v", err)
			}
			var s string
			if err := c.GetOrSet(ctx, "plain-key", &s, func(context.Context) (any, error) { return "rebuilt", nil }, tc.liveTTL, NewValueDependency(1)); err != nil {
				t.Fatalf("GetOrSet over a plain entry: %v", err)
			}
			if s != "rebuilt" {
				t.Fatalf("entry stored without a dependency must rebuild under one: got %q", s)
			}
		})
	}
}

// TestTimeDependencyInvalidation verifies an entry expires at the dependency's
// fixed time even though its ttl has not elapsed. The window is quantized to
// the hour, as a schedule would derive it, so independently constructed
// dependencies agree within the window.
func TestTimeDependencyInvalidation(t *testing.T) {
	for _, tc := range cacheCases(t) {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			c := tc.cache
			builds := 0
			build := func(context.Context) (any, error) {
				builds++
				return "v", nil
			}
			var got string
			window := func() time.Time {
				return time.Now().Truncate(time.Hour).Add(2 * time.Hour)
			}

			// Dependency pointing into the future: entry is cached.
			if err := c.GetOrSet(ctx, "time-key", &got, build, tc.liveTTL, NewTimeDependency(window())); err != nil {
				t.Fatalf("GetOrSet future dependency: %v", err)
			}
			if err := c.GetOrSet(ctx, "time-key", &got, build, tc.liveTTL, NewTimeDependency(window())); err != nil {
				t.Fatalf("GetOrSet future dependency again: %v", err)
			}
			if builds != 1 {
				t.Fatalf("future dependency must keep the entry cached: builds=%d, want 1", builds)
			}

			// A dependency whose time already passed invalidates the entry.
			if err := c.GetOrSet(ctx, "time-key", &got, build, tc.liveTTL, NewTimeDependency(time.Now().Add(-time.Minute))); err != nil {
				t.Fatalf("GetOrSet past dependency: %v", err)
			}
			if builds != 2 {
				t.Fatalf("past dependency must rebuild: builds=%d, want 2", builds)
			}

			// A dependency whose time stays in the past keeps invalidating —
			// the rebuild site must supply the next window's time.
			if err := c.GetOrSet(ctx, "time-key", &got, build, tc.liveTTL, NewTimeDependency(time.Now().Add(-time.Minute))); err != nil {
				t.Fatalf("GetOrSet past dependency again: %v", err)
			}
			if builds != 3 {
				t.Fatalf("a still-past dependency must rebuild again: builds=%d, want 3", builds)
			}
		})
	}
}

// TestValueDependencyFingerprint covers the fingerprint semantics directly:
// equal values capture equal fingerprints, changed values do not, and an
// empty stored fingerprint always counts as changed.
func TestValueDependencyFingerprint(t *testing.T) {
	ctx := context.Background()

	a := NewValueDependency(map[string]int{"rows": 10})
	b := NewValueDependency(map[string]int{"rows": 10})
	fpA, err := a.Capture(ctx)
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}
	fpB, err := b.Capture(ctx)
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}
	if fpA != fpB {
		t.Error("equal dependency values must capture equal fingerprints")
	}

	// JSON encoding is deterministic (map keys sorted), so the fingerprint
	// depends only on the value, not on construction order.
	if _, err := NewValueDependency(map[string]int{"rows": 10, "other": 1}).Capture(ctx); err != nil {
		t.Fatalf("Capture: %v", err)
	}
	changed := NewValueDependency(map[string]int{"rows": 11})
	if !changed.Changed(ctx, fpA) {
		t.Error("a changed dependency value must count as changed")
	}
	if !b.Changed(ctx, "") {
		t.Error("an empty stored fingerprint must count as changed")
	}
	if b.Changed(ctx, fpA) {
		t.Error("an unchanged dependency value must not count as changed")
	}

	// A value json cannot encode makes the dependency judge itself changed:
	// validity cannot be proven, so the entry must not be served.
	uncapturable := NewValueDependency(make(chan int))
	if _, err := uncapturable.Capture(ctx); err == nil {
		t.Error("Capture of a channel returned nil error")
	}
	if !uncapturable.Changed(ctx, "any") {
		t.Error("an uncapturable dependency must count as changed")
	}
}

// TestTimeDependencyChangedOnUnparsableFingerprint verifies a corrupted or
// missing stored fingerprint reads as changed rather than as valid, and that
// a dependency carrying a different future window than the stored one
// invalidates the entry.
func TestTimeDependencyChangedOnUnparsableFingerprint(t *testing.T) {
	ctx := context.Background()
	dep := NewTimeDependency(time.Now().Add(time.Hour))
	if !dep.Changed(ctx, "") {
		t.Error("empty fingerprint must count as changed")
	}
	if !dep.Changed(ctx, "not a timestamp") {
		t.Error("unparsable fingerprint must count as changed")
	}
	fp, err := dep.Capture(ctx)
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}
	if dep.Changed(ctx, fp) {
		t.Error("a matching future window must not count as changed")
	}
	otherWindow := NewTimeDependency(time.Now().Add(2 * time.Hour))
	fpOther, err := otherWindow.Capture(ctx)
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}
	if !dep.Changed(ctx, fpOther) {
		t.Error("a stored fingerprint naming a different window must count as changed")
	}
	past := NewTimeDependency(time.Now().Add(-time.Second))
	fpPast, err := past.Capture(ctx)
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}
	if !past.Changed(ctx, fpPast) {
		t.Error("an elapsed expiry time must count as changed")
	}
}

// TestGetOrSetDependencyPersistsAcrossCacheLookups verifies an entry stored
// under a dependency is an ordinary entry for the rest of the API: plain Get
// serves it without re-reading the dependency (Yii2's get() can take one;
// here the dependency arbitrates only inside GetOrSet, keeping the
// decode-into-dest signature free of optional parameters).
func TestGetOrSetDependencyPersistsAcrossCacheLookups(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	c := NewRedis(client, "goyii:cache:", time.Hour)

	ctx := context.Background()
	builds := 0
	var got int
	err := c.GetOrSet(ctx, "k", &got, func(context.Context) (any, error) {
		builds++
		return 42, nil
	}, time.Hour, NewValueDependency("v1"))
	if err != nil {
		t.Fatalf("GetOrSet: %v", err)
	}
	var plain int
	found, err := c.Get(ctx, "k", &plain)
	if err != nil || !found || plain != 42 {
		t.Fatalf("plain Get of a dependency-stored entry: found=%v value=%d err=%v", found, plain, err)
	}
	if builds != 1 {
		t.Fatalf("builds=%d, want 1", builds)
	}
}
