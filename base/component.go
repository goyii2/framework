package base

import (
	"sync"
	"sync/atomic"
)

// component is a lazily initialized, replaceable holder for one application
// component value. The first get builds the value via the factory exactly
// once; set swaps the value at any time, and setting nil resets the holder so
// the next get rebuilds from the factory. Safe for concurrent use.
type component[T any] struct {
	mu    sync.Mutex // serializes the first build
	build func() T   // factory for the default value; must not call back into this holder
	val   atomic.Pointer[T]
}

func newComponent[T any](build func() T) *component[T] {
	return &component[T]{build: build}
}

// get returns the current value, building it on first access.
func (c *component[T]) get() T {
	if p := c.val.Load(); p != nil {
		return *p
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if p := c.val.Load(); p != nil {
		return *p
	}
	v := c.build()
	c.val.Store(&v)
	return v
}

// set replaces the value. A nil value resets the holder to lazy
// initialization from the factory.
func (c *component[T]) set(v T) {
	if any(v) == nil {
		c.val.Store(nil)
		return
	}
	c.val.Store(&v)
}
