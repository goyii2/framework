package queue

import (
	"context"
	"errors"
	"time"
)

// Sync is the immediate-execution driver: Queue.Push runs the job inline on
// the pushing goroutine instead of storing anything, and the job's error —
// if any — is returned to the pusher. It exists for tests and for wiring
// checks: a queue configured sync proves the push side end to end without a
// worker, a database or a redis.
//
// The deferred-execution options are meaningless here by construction: there
// is no worker to defer to and no storage to retry from, so a job runs
// exactly once and a failure surfaces directly to the pusher (Attempts,
// Delay, TTR and Backoff are ignored). When the queue stores messages, use
// the db or redis driver.
type Sync struct{}

// NewSync creates the sync driver.
func NewSync() *Sync { return &Sync{} }

// errSyncStoresNothing is the answer to any attempt to use the sync driver
// as a store: the driver executes through Queue.Push, which short-circuits
// the driver entirely, so reaching these methods means the driver was wired
// into the wrong path.
var errSyncStoresNothing = errors.New("queue: the sync driver executes inline through Queue.Push and stores nothing")

// Push never stores; see errSyncStoresNothing.
func (*Sync) Push(context.Context, *Message) error { return errSyncStoresNothing }

// Reserve never yields; the sync queue has no waiting messages.
func (*Sync) Reserve(context.Context) (*Message, error) { return nil, nil }

// Complete is a no-op.
func (*Sync) Complete(context.Context, *Message) error { return nil }

// Requeue is a no-op: the failing job's error already went to the pusher.
func (*Sync) Requeue(context.Context, *Message, time.Duration) error { return nil }

// Fail is a no-op.
func (*Sync) Fail(context.Context, *Message) error { return nil }

// RetryFailed has nothing to retry and reports zero.
func (*Sync) RetryFailed(context.Context) (int, error) { return 0, nil }
