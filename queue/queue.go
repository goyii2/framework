package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// Job is one unit of deferred work. Name is the registry key the worker
// resolves the job's prototype by; Execute receives the worker context, so a
// cooperative job honors its cancellation. The payload travels in the job
// struct's exported fields, marshaled to JSON on push and decoded back into a
// fresh instance of the registered prototype before Execute runs. Implement
// Job on a pointer receiver and register the pointer — the registry needs a
// concrete type to instantiate.
type Job interface {
	// Name returns the registered job name.
	Name() string
	// Execute performs the work.
	Execute(ctx context.Context) error
}

// DefaultTTR is the reservation lifetime used when a push sets none: five
// minutes, after which a wedged worker's reservation returns to waiting.
const DefaultTTR = 5 * time.Minute

// DefaultBackoff returns the retry delay after the given attempt fails:
// 2^attempt seconds capped at fifteen minutes. attempt is 1-based (the
// number of the attempt that just failed); values below 1 are treated as 1.
func DefaultBackoff(attempt int) time.Duration {
	const capDelay = 15 * time.Minute
	if attempt < 1 {
		attempt = 1
	}
	if attempt > 40 { // 2^41 seconds overflows int64; far past the cap anyway
		return capDelay
	}
	secs := int64(1) << uint(attempt)
	if secs > int64(capDelay/time.Second) {
		return capDelay
	}
	return time.Duration(secs) * time.Second
}

// PushOptions tunes one push. The zero value executes the job once (Attempts
// 1) under the default TTR with no delay and no priority.
type PushOptions struct {
	// Delay defers the first execution attempt until it elapses.
	Delay time.Duration
	// Priority orders due messages: higher runs first.
	Priority int
	// Attempts is the total execution attempts allowed, the first included.
	// Values below 1 clamp to 1 — a push always buys at least one attempt.
	Attempts int
	// TTR (time to run) bounds one reservation: when it elapses before the
	// worker settles the message, the reservation returns to waiting and the
	// job becomes due again. Non-positive values fall back to DefaultTTR.
	TTR time.Duration
	// Backoff computes the delay before retry attempt n (1-based, the
	// attempt that just failed). Nil selects DefaultBackoff. The delays are
	// resolved at push time and travel with the message, so retries work
	// across process boundaries where the function itself cannot go.
	Backoff func(attempt int) time.Duration
}

// PushOption amends PushOptions for one push.
type PushOption func(*PushOptions)

// WithDelay defers the first execution attempt until d elapses.
func WithDelay(d time.Duration) PushOption { return func(o *PushOptions) { o.Delay = d } }

// WithPriority raises the message's execution order among due messages.
func WithPriority(p int) PushOption { return func(o *PushOptions) { o.Priority = p } }

// WithAttempts allows n total execution attempts (first included).
func WithAttempts(n int) PushOption { return func(o *PushOptions) { o.Attempts = n } }

// WithTTR sets the reservation lifetime; beyond it the reservation is
// reclaimed and the job becomes due again.
func WithTTR(d time.Duration) PushOption { return func(o *PushOptions) { o.TTR = d } }

// WithBackoff replaces the default retry schedule with fn.
func WithBackoff(fn func(attempt int) time.Duration) PushOption {
	return func(o *PushOptions) { o.Backoff = fn }
}

// pushOptions applies opts over the zero value, normalizing invalid entries.
func pushOptions(opts ...PushOption) PushOptions {
	var o PushOptions
	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}
	if o.Delay < 0 {
		o.Delay = 0
	}
	if o.Attempts < 1 {
		o.Attempts = 1
	}
	if o.TTR <= 0 {
		o.TTR = DefaultTTR
	}
	return o
}

// Message is one queued job with its execution state. Push creates it; the
// driver persists it and restores it on Reserve. RetryDelays is the
// backoff schedule resolved at push time (index 0 = delay before retry 1).
type Message struct {
	ID          int64
	Name        string
	Payload     json.RawMessage
	Priority    int
	Attempts    int
	Attempt     int
	TTR         time.Duration
	RetryDelays []time.Duration
	DueAt       time.Time
	PushedAt    time.Time
}

// retryDelay returns the wait before the retry after the given attempt (1-based,
// the attempt that just failed): the push-time schedule when it covers the
// attempt, DefaultBackoff otherwise (robust for messages whose stored schedule
// is absent or shorter than the attempt count).
func (m *Message) retryDelay(attempt int) time.Duration {
	if attempt-1 >= 0 && attempt-1 < len(m.RetryDelays) {
		return m.RetryDelays[attempt-1]
	}
	return DefaultBackoff(attempt)
}

// newMessage marshals job and resolves its options into a runnable message.
func newMessage(job Job, o PushOptions) (*Message, error) {
	payload, err := json.Marshal(job)
	if err != nil {
		return nil, fmt.Errorf("queue: marshal payload of %q: %w", job.Name(), err)
	}
	now := time.Now()
	m := &Message{
		Name:     job.Name(),
		Payload:  payload,
		Priority: o.Priority,
		Attempts: o.Attempts,
		TTR:      o.TTR,
		PushedAt: now,
		DueAt:    now.Add(o.Delay),
	}
	if o.Attempts > 1 {
		backoff := o.Backoff
		if backoff == nil {
			backoff = DefaultBackoff
		}
		m.RetryDelays = make([]time.Duration, o.Attempts-1)
		for i := 1; i < o.Attempts; i++ {
			m.RetryDelays[i-1] = backoff(i)
		}
	}
	return m, nil
}

// Driver persists messages and hands them to workers. Implementations must be
// safe for concurrent use by several workers, and Reserve must be atomic:
// a message reserved here is invisible to every other Reserve until it is
// settled (Complete, Requeue or Fail) or its TTR elapses.
type Driver interface {
	// Push stores m, assigning m.ID. The message becomes due at m.DueAt.
	Push(ctx context.Context, m *Message) error
	// Reserve atomically claims the next due message — highest priority
	// first, then earliest due — first reclaiming reservations whose TTR has
	// elapsed. It returns nil (not an error) when nothing is due.
	Reserve(ctx context.Context) (*Message, error)
	// Complete removes m after a successful execution.
	Complete(ctx context.Context, m *Message) error
	// Requeue returns m to waiting, runnable after delay; m.Attempt carries
	// the attempts made so far.
	Requeue(ctx context.Context, m *Message, delay time.Duration) error
	// Fail parks m as failed: kept for RetryFailed, not runnable.
	Fail(ctx context.Context, m *Message) error
	// RetryFailed moves every failed message back to waiting with its
	// attempt count reset and reports how many moved.
	RetryFailed(ctx context.Context) (int, error)
}

// Event names of the queue lifecycle, dispatched through the application's
// event registry (base On/Trigger). They are untyped constants on purpose:
// they convert to base.EventName at the subscribe site without this package
// importing base.
const (
	EventBeforePush = "queue.beforePush" // Data: PushEventData
	EventAfterPush  = "queue.afterPush"  // Data: PushEventData
	EventBeforeExec = "queue.beforeExec" // Data: ExecEventData
	EventAfterExec  = "queue.afterExec"  // Data: ExecEventData
	EventExecError  = "queue.execError"  // Data: ExecEventData (Err set)
)

// PushEventData is carried by the BeforePush and AfterPush events.
type PushEventData struct {
	Job      string
	Priority int
	Delay    time.Duration
	Attempts int
}

// ExecEventData is carried by the three execution events. Attempt is 1-based;
// Err carries the failure on ExecError and is nil otherwise.
type ExecEventData struct {
	Job     string
	ID      int64
	Attempt int
	Err     error
}

// Dispatcher triggers one lifecycle event; the application core injects an
// adapter onto its event registry. A nil dispatcher (the default) drops
// events.
type Dispatcher func(ctx context.Context, event string, data any)

// Queue is the application queue: a job registry over a driver. Push defers
// work; Run and Listen execute it. The zero poll interval is not usable —
// construct through New.
type Queue struct {
	driver Driver
	jobs   *Registry
	events Dispatcher
	poll   time.Duration
}

// QueueOption configures a Queue at construction.
type QueueOption func(*Queue)

// WithDispatcher routes the five lifecycle events through d.
func WithDispatcher(d Dispatcher) QueueOption { return func(q *Queue) { q.events = d } }

// WithPollInterval sets how long Listen waits between drain passes when the
// queue is idle; d must be positive.
func WithPollInterval(d time.Duration) QueueOption {
	return func(q *Queue) {
		if d > 0 {
			q.poll = d
		}
	}
}

// New creates the queue over driver. jobs may be nil (an empty registry the
// application fills through Jobs before workers run).
func New(d Driver, jobs *Registry, opts ...QueueOption) *Queue {
	if jobs == nil {
		jobs = new(Registry)
	}
	q := &Queue{driver: d, jobs: jobs, poll: time.Second}
	for _, opt := range opts {
		if opt != nil {
			opt(q)
		}
	}
	return q
}

// Jobs returns the job registry the worker resolves names through.
func (q *Queue) Jobs() *Registry { return q.jobs }

// Driver returns the underlying driver (sync, db, redis or a custom one).
func (q *Queue) Driver() Driver { return q.driver }

// trigger dispatches one event when a dispatcher is installed.
func (q *Queue) trigger(ctx context.Context, event string, data any) {
	if q.events != nil {
		q.events(ctx, event, data)
	}
}

// Push defers job: it serializes the payload, stores the message through the
// driver and returns the driver-assigned id. With the sync driver the job
// executes inline instead and Push returns its error, if any.
func (q *Queue) Push(ctx context.Context, job Job, opts ...PushOption) (int64, error) {
	o := pushOptions(opts...)
	m, err := newMessage(job, o)
	if err != nil {
		return 0, err
	}
	pushData := PushEventData{Job: m.Name, Priority: o.Priority, Delay: o.Delay, Attempts: o.Attempts}
	q.trigger(ctx, EventBeforePush, pushData)

	if _, ok := q.driver.(*Sync); ok {
		jobErr, infraErr := q.execute(ctx, m, job)
		q.trigger(ctx, EventAfterPush, pushData)
		switch {
		case infraErr != nil:
			return 0, infraErr
		case jobErr != nil:
			return 0, jobErr
		default:
			return 0, nil
		}
	}
	if err := q.driver.Push(ctx, m); err != nil {
		return 0, fmt.Errorf("queue: push %q: %w", m.Name, err)
	}
	q.trigger(ctx, EventAfterPush, pushData)
	return m.ID, nil
}

// Run drains one pass: it reserves and executes messages until none are due,
// returning how many ran. Job failures are settled inside (requeue or fail)
// and never abort the pass; only driver errors do.
func (q *Queue) Run(ctx context.Context) (int, error) {
	processed := 0
	for {
		if err := ctx.Err(); err != nil {
			return processed, err
		}
		m, err := q.driver.Reserve(ctx)
		if err != nil {
			return processed, fmt.Errorf("queue: reserve: %w", err)
		}
		if m == nil {
			return processed, nil
		}
		processed++
		if infraErr := q.runOne(ctx, m); infraErr != nil {
			return processed, infraErr
		}
	}
}

// Listen runs drain passes forever, waiting q.poll between idle passes, until
// ctx is canceled; the cancellation is the normal stop and returns nil. A
// driver error aborts listening with that error.
func (q *Queue) Listen(ctx context.Context) error {
	for {
		if _, err := q.Run(ctx); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(q.poll):
		}
	}
}

// RetryFailed requeues every failed message with its attempt count reset and
// returns how many were re-pushed.
func (q *Queue) RetryFailed(ctx context.Context) (int, error) {
	n, err := q.driver.RetryFailed(ctx)
	if err != nil {
		return 0, fmt.Errorf("queue: retry failed messages: %w", err)
	}
	return n, nil
}

// runOne executes one reserved message. It returns an error only for driver
// (infrastructure) failures; the job's own failure is settled through
// Requeue or Fail and reported through the ExecError event.
func (q *Queue) runOne(ctx context.Context, m *Message) error {
	job, err := q.jobs.Build(m.Name, m.Payload)
	if err != nil {
		// Unknown name or broken payload: nothing to run. ExecError carries
		// the cause, and the message parks as failed — requeueing it would
		// spin the queue on a job that can never rebuild.
		q.trigger(ctx, EventExecError, ExecEventData{Job: m.Name, ID: m.ID, Attempt: m.Attempt + 1, Err: err})
		if ferr := q.driver.Fail(ctx, m); ferr != nil {
			return fmt.Errorf("queue: park undecodable job %q (id %d): %w", m.Name, m.ID, ferr)
		}
		return nil
	}
	_, infraErr := q.execute(ctx, m, job)
	return infraErr
}

// execute runs job for m once and settles the message: Complete on success;
// after a failure Requeue when attempts remain, Fail when they are exhausted.
// It returns the job's own error (nil on success — the job error is settled
// queue-side, not fatal) separately from infrastructure errors.
func (q *Queue) execute(ctx context.Context, m *Message, job Job) (jobErr, infraErr error) {
	attempt := m.Attempt + 1
	q.trigger(ctx, EventBeforeExec, ExecEventData{Job: m.Name, ID: m.ID, Attempt: attempt})
	err := job.Execute(ctx)
	m.Attempt = attempt

	if err == nil {
		q.trigger(ctx, EventAfterExec, ExecEventData{Job: m.Name, ID: m.ID, Attempt: attempt})
		if cerr := q.driver.Complete(ctx, m); cerr != nil {
			return nil, fmt.Errorf("queue: complete %q (id %d): %w", m.Name, m.ID, cerr)
		}
		return nil, nil
	}

	q.trigger(ctx, EventExecError, ExecEventData{Job: m.Name, ID: m.ID, Attempt: attempt, Err: err})
	if m.Attempt < m.Attempts {
		if rerr := q.driver.Requeue(ctx, m, m.retryDelay(m.Attempt)); rerr != nil {
			return err, fmt.Errorf("queue: requeue %q (id %d): %w", m.Name, m.ID, rerr)
		}
		return err, nil
	}
	if ferr := q.driver.Fail(ctx, m); ferr != nil {
		return err, fmt.Errorf("queue: park %q (id %d) as failed: %w", m.Name, m.ID, ferr)
	}
	return err, nil
}
