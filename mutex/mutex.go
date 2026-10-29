package mutex

import (
	"context"
	"errors"
	"time"
)

// ErrLocked reports that TryLock found the name held by someone else. Lock
// never returns it: Lock waits for the holder instead of giving up.
var ErrLocked = errors.New("mutex: lock held")

// Mutex excludes concurrent callers from a critical section keyed by name.
// Two drivers ship: Process (per-name locks inside one process) and Redis
// (cross-process locks in a shared redis). The name space is caller-defined;
// drivers never share names with each other or with other components.
//
// The contract is deliberately small and not reentrant:
//
//   - ttl must be positive and bounds how long the lock can be held. When
//     it elapses the lock frees itself — the redis server drops the key at
//     expiry, the process driver fires a timer — so a crashed or wedged
//     holder cannot deadlock everyone else. Locks are not renewed
//     automatically: work expected to outlive its ttl must be split into
//     segments, each running under its own acquire-release round
//     (lock -> do a bounded chunk -> release -> re-check and lock again).
//     Between segments the name is free; whoever gets it first wins, so a
//     segment boundary is also the point where leadership can change.
//
//   - release is idempotent and safe to call after the ttl has elapsed: it
//     only ever frees the caller's own acquisition (the redis driver
//     compares the acquisition token before deleting), so a late release
//     cannot steal the lock from whoever acquired it after the expiry.
//
//   - Lock waits until the name is free or ctx is done; TryLock returns
//     ErrLocked immediately when the name is held. Acquiring a name the
//     caller already holds is not reentrancy: Lock deadlocks on the held
//     name (the process driver blocks, the redis driver blocks until the
//     first acquisition's ttl frees it) and TryLock fails with ErrLocked.
//
//   - A canceled or expired ctx fails acquisition with its own error even
//     when the name is free; a name must be non-empty and a ttl must be
//     positive, or acquisition fails with a configuration-style error.
//
// All methods are safe for concurrent use.
type Mutex interface {
	// Lock acquires name for at most ttl and returns the function that
	// frees it. It blocks while another caller holds name, returning an
	// error when ctx is done first.
	Lock(ctx context.Context, name string, ttl time.Duration) (release func(), err error)

	// TryLock is Lock without the waiting: when another caller holds name
	// it returns ErrLocked at once, otherwise it acquires like Lock.
	TryLock(ctx context.Context, name string, ttl time.Duration) (release func(), err error)
}
