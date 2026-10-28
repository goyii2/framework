package cache

import (
	"context"
	"sync"
	"time"
)

// Memory keeps entries in a process-local map, their lifetime checked on
// read and swept in the background. It needs no infrastructure, but the
// entries are invisible to other processes: use it for single-instance
// applications or as a per-process front for a slower shared backend. It
// corresponds to Yii3's ArrayCache (Yii2 shipped no in-memory core cache;
// APCu filled that role with a shared-memory extension).
//
// Entries live in serialized form — like the file and redis backends — so a
// value retrieved from memory is a decoded copy, never the stored object:
// callers cannot mutate a cached entry through a retrieved reference, and
// all three backends behave identically.
type Memory struct {
	mu         sync.RWMutex
	items      map[string]entry
	defaultTTL time.Duration

	stop     chan struct{}
	stopOnce sync.Once
}

// NewMemory creates the in-process backend. defaultTTL applies to entries
// stored with a non-positive ttl. A background sweeper drops expired entries
// at half the default TTL, clamped between 10ms and 1m, so unread expired
// entries cannot accumulate; stop it with Stop when the cache outlives its
// use (production caches live for the process lifetime and need no Stop).
func NewMemory(defaultTTL time.Duration) *Memory {
	interval := defaultTTL / 2
	if interval > time.Minute {
		interval = time.Minute
	}
	if interval < 10*time.Millisecond {
		interval = 10 * time.Millisecond
	}
	m := &Memory{
		items:      make(map[string]entry),
		defaultTTL: defaultTTL,
		stop:       make(chan struct{}),
	}
	go m.sweep(interval)
	return m
}

// Get decodes the value under key into dest; expired entries read as missing.
func (m *Memory) Get(ctx context.Context, key string, dest any) (bool, error) {
	return get(ctx, m, key, dest)
}

// Set stores value under key, replacing any previous entry.
func (m *Memory) Set(ctx context.Context, key string, value any, ttl time.Duration) error {
	return set(ctx, m, key, value, ttl)
}

// Delete removes key; an absent key is not an error.
func (m *Memory) Delete(ctx context.Context, key string) error {
	if err := checkKey(key); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.items, key)
	return nil
}

// Exists reports whether key holds a live entry.
func (m *Memory) Exists(ctx context.Context, key string) (bool, error) {
	return exists(ctx, m, key)
}

// Flush drops every entry of this cache.
func (m *Memory) Flush(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.items = make(map[string]entry)
	return nil
}

// GetOrSet decodes the entry under key into dest, rebuilding it with build
// when missing, expired or invalidated by dep.
func (m *Memory) GetOrSet(ctx context.Context, key string, dest any, build func(context.Context) (any, error), ttl time.Duration, dep Dependency) error {
	return getOrSet(ctx, m, key, dest, build, ttl, dep)
}

// Stop halts the background sweeper. Lookups keep expiring entries lazily on
// read; only the proactive cleanup of unread entries stops. Safe to call
// more than once.
func (m *Memory) Stop() {
	m.stopOnce.Do(func() { close(m.stop) })
}

// lookup returns a live copy of the entry under key, dropping an expired
// entry on sight (the sweeper only covers entries nothing reads).
func (m *Memory) lookup(ctx context.Context, key string) (entry, bool, error) {
	m.mu.RLock()
	e, ok := m.items[key]
	expired := ok && e.expired(time.Now())
	m.mu.RUnlock()
	if !ok {
		return entry{}, false, nil
	}
	if expired {
		m.mu.Lock()
		if cur, ok := m.items[key]; ok && cur.expired(time.Now()) {
			delete(m.items, key)
		}
		m.mu.Unlock()
		return entry{}, false, nil
	}
	return e, true, nil
}

// save stores the entry under the resolved lifetime.
func (m *Memory) save(ctx context.Context, key string, e entry, ttl time.Duration) error {
	e.ExpiresAt = time.Now().Add(ttlFor(ttl, m.defaultTTL)).UnixNano()
	m.mu.Lock()
	defer m.mu.Unlock()
	m.items[key] = e
	return nil
}

// sweep periodically drops expired entries until Stop is called.
func (m *Memory) sweep(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			now := time.Now()
			m.mu.Lock()
			for key, e := range m.items {
				if e.expired(now) {
					delete(m.items, key)
				}
			}
			m.mu.Unlock()
		case <-m.stop:
			return
		}
	}
}
