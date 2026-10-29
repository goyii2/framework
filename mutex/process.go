package mutex

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Process is the in-process driver: one lock per name inside a single
// process, enforced across goroutines. A held name is an entry in the lock
// table whose channel is open; releasing — explicitly or by the ttl timer —
// removes the entry and closes the channel, waking the waiting Locks, which
// race for the freed table slot. Locks do not outlive the process, and
// separate Process instances keep separate lock tables.
type Process struct {
	mu    sync.Mutex // guards locks
	locks map[string]*heldLock
}

// heldLock is one acquisition of a name: free is closed exactly once, when
// the acquisition ends by release or by ttl expiry.
type heldLock struct {
	free chan struct{}
	once sync.Once
}

// open marks the lock free. Closing under a once makes release idempotent:
// the ttl timer and the release function may both fire.
func (h *heldLock) open() { h.once.Do(func() { close(h.free) }) }

// NewProcess creates the process driver.
func NewProcess() *Process {
	return &Process{locks: make(map[string]*heldLock)}
}

// Lock acquires name for at most ttl, blocking while another caller holds
// it. See the Mutex interface for the full contract.
func (p *Process) Lock(ctx context.Context, name string, ttl time.Duration) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := checkAcquire(name, ttl); err != nil {
		return nil, err
	}
	for {
		p.mu.Lock()
		held, ok := p.locks[name]
		if !ok {
			held = &heldLock{free: make(chan struct{})}
			p.locks[name] = held
			p.mu.Unlock()
			return p.armRelease(name, held, ttl), nil
		}
		p.mu.Unlock()

		// The name is taken: wait for its channel (freed by release or by
		// the ttl timer) or for the context, then race for the slot again.
		select {
		case <-held.free:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

// TryLock acquires name for at most ttl without waiting: a held name fails
// with ErrLocked at once. See the Mutex interface for the full contract.
func (p *Process) TryLock(ctx context.Context, name string, ttl time.Duration) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := checkAcquire(name, ttl); err != nil {
		return nil, err
	}
	p.mu.Lock()
	if _, ok := p.locks[name]; ok {
		p.mu.Unlock()
		return nil, ErrLocked
	}
	held := &heldLock{free: make(chan struct{})}
	p.locks[name] = held
	p.mu.Unlock()
	return p.armRelease(name, held, ttl), nil
}

// armRelease schedules the ttl expiry and returns the explicit release
// function; whichever of the two runs first frees the acquisition, the
// other becomes a no-op.
func (p *Process) armRelease(name string, held *heldLock, ttl time.Duration) func() {
	timer := time.AfterFunc(ttl, func() { p.release(name, held) })
	return func() {
		timer.Stop()
		p.release(name, held)
	}
}

// release ends one acquisition: it drops the table entry when this
// acquisition is still the current one — an acquisition ending after its
// successor has already taken the name must not evict the successor — and
// wakes the waiters. The entry goes before the close, so a woken waiter
// finds the slot free instead of spinning on a stale entry.
func (p *Process) release(name string, held *heldLock) {
	p.mu.Lock()
	if cur, ok := p.locks[name]; ok && cur == held {
		delete(p.locks, name)
	}
	p.mu.Unlock()
	held.open()
}

// checkAcquire validates the acquire inputs both drivers share.
func checkAcquire(name string, ttl time.Duration) error {
	if name == "" {
		return fmt.Errorf("mutex: empty lock name")
	}
	if ttl <= 0 {
		return fmt.Errorf("mutex: ttl must be positive, got %v", ttl)
	}
	return nil
}
