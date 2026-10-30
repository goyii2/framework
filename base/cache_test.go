package base

import (
	"context"
	"testing"
	"time"

	"github.com/goyii2/framework/cache"
)

// recordingCache is a fake Cache that counts the calls reaching it, proving
// business code can be tested against the contract without a real backend.
type recordingCache struct {
	gets, sets, deletes, exists, flushes, getOrSets int
}

func (r *recordingCache) Get(ctx context.Context, key string, dest any) (bool, error) {
	r.gets++
	return false, nil
}

func (r *recordingCache) Set(ctx context.Context, key string, value any, ttl time.Duration) error {
	r.sets++
	return nil
}

func (r *recordingCache) Delete(ctx context.Context, key string) error {
	r.deletes++
	return nil
}

func (r *recordingCache) Exists(ctx context.Context, key string) (bool, error) {
	r.exists++
	return false, nil
}

func (r *recordingCache) Flush(ctx context.Context) error {
	r.flushes++
	return nil
}

func (r *recordingCache) GetOrSet(ctx context.Context, key string, dest any, build func(context.Context) (any, error), ttl time.Duration, dep cache.Dependency) error {
	r.getOrSets++
	return nil
}

func TestApplicationCacheDefaultIsMemory(t *testing.T) {
	app := New()

	if _, ok := app.Cache().(*cache.Memory); !ok {
		t.Fatalf("default driver must be the memory backend, got %T", app.Cache())
	}
	if app.Cache() != app.Cache() {
		t.Error("repeated Cache() returned a different instance")
	}
}

func TestApplicationCacheReplacedByConfiguredBackend(t *testing.T) {
	app := New()

	// The backend the project's configuration selects installs through the
	// component package's constructor.
	fileCache, err := cache.New(cache.Config{Driver: "file", Path: t.TempDir(), TTL: 60})
	if err != nil {
		t.Fatalf("cache.New(file): %v", err)
	}
	app.SetCache(fileCache)
	if _, ok := app.Cache().(*cache.File); !ok {
		t.Errorf("installed cache = %T, want *cache.File", app.Cache())
	}

	app.SetCache(nil)
	if _, ok := app.Cache().(*cache.Memory); !ok {
		t.Error("SetCache(nil) must restore the memory fallback")
	}
}

func TestApplicationSetCacheFake(t *testing.T) {
	app := New()

	fake := &recordingCache{}
	app.SetCache(fake)

	ctx := context.Background()
	var dest int
	if _, err := app.Cache().Get(ctx, "k", &dest); err != nil {
		t.Fatalf("Get through the fake: %v", err)
	}
	if err := app.Cache().Set(ctx, "k", 1, time.Minute); err != nil {
		t.Fatalf("Set through the fake: %v", err)
	}
	if err := app.Cache().GetOrSet(ctx, "k", &dest, func(context.Context) (any, error) { return 1, nil }, time.Minute, nil); err != nil {
		t.Fatalf("GetOrSet through the fake: %v", err)
	}
	if fake.gets != 1 || fake.sets != 1 || fake.getOrSets != 1 {
		t.Errorf("fake calls: gets=%d sets=%d getOrSets=%d, want 1/1/1", fake.gets, fake.sets, fake.getOrSets)
	}
}
