package base

import (
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
)

// countingLog is a concurrency-safe Log fake that records calls.
type countingLog struct {
	mu    sync.Mutex
	calls []string
}

func (l *countingLog) Enabled(level slog.Level) bool { return true }

func (l *countingLog) record(level, msg string) {
	l.mu.Lock()
	l.calls = append(l.calls, level+":"+msg)
	l.mu.Unlock()
}

func (l *countingLog) Debug(msg string, args ...any) { l.record("debug", msg) }
func (l *countingLog) Info(msg string, args ...any)  { l.record("info", msg) }
func (l *countingLog) Warn(msg string, args ...any)  { l.record("warn", msg) }
func (l *countingLog) Error(msg string, args ...any) { l.record("error", msg) }

func (l *countingLog) snapshot() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.calls...)
}

func TestComponentLazyInitBuildsOnce(t *testing.T) {
	var builds atomic.Int32
	c := newComponent(func() Log {
		builds.Add(1)
		return &countingLog{}
	})
	first := c.get()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if got := c.get(); got != first {
				t.Error("concurrent get returned a different component instance")
			}
		}()
	}
	wg.Wait()

	if n := builds.Load(); n != 1 {
		t.Errorf("component built %d times under concurrent access, want exactly 1", n)
	}
}

func TestComponentSetReplacesWithoutBuilding(t *testing.T) {
	var builds atomic.Int32
	c := newComponent(func() Log {
		builds.Add(1)
		return &countingLog{}
	})

	fake := &countingLog{}
	c.set(fake)
	if got := c.get(); got != Log(fake) {
		t.Fatal("get after set did not return the injected component")
	}
	if n := builds.Load(); n != 0 {
		t.Errorf("factory ran %d times after set, want 0", n)
	}
}

func TestComponentSetNilRebuildsDefault(t *testing.T) {
	var builds atomic.Int32
	c := newComponent(func() Log {
		builds.Add(1)
		return &countingLog{}
	})

	fake := &countingLog{}
	c.set(fake)
	c.set(nil)
	if n := builds.Load(); n != 0 {
		t.Fatalf("nil reset built the default %d times, want 0", n)
	}

	got := c.get()
	if n := builds.Load(); n != 1 {
		t.Errorf("default rebuilt %d times after nil reset, want 1", n)
	}
	if got == Log(fake) {
		t.Error("get after nil reset returned the stale injected component")
	}
}

// TestComponentConcurrentSetAndGet mixes replacement and lazy initialization
// from many goroutines; run with -race.
func TestComponentConcurrentSetAndGet(t *testing.T) {
	var builds atomic.Int32
	c := newComponent(func() Log {
		builds.Add(1)
		return &countingLog{}
	})

	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if i%2 == 0 {
				c.set(&countingLog{})
			} else {
				_ = c.get().Enabled(slog.LevelInfo)
			}
		}(i)
	}
	wg.Wait()

	_ = c.get().Enabled(slog.LevelInfo)
	if n := builds.Load(); n > 1 {
		t.Errorf("default built %d times, want at most 1", n)
	}
}
