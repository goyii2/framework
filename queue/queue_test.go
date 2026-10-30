package queue

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeDriver is an in-memory Driver: Push appends to pushed, Reserve pops
// reserveQ, so tests script exactly what the worker sees.
type fakeDriver struct {
	mu             sync.Mutex
	pushed         []*Message
	reserveQ       []*Message
	completed      []*Message
	requeues       []requeueCall // pending: cleared as their delays elapse
	requeueHistory []requeueCall
	failed         []*Message
	retried        int
}

type requeueCall struct {
	m     *Message
	delay time.Duration
}

// makeRequeuesDue moves every requeued message into the reserve queue,
// simulating the retry delay having elapsed in a real driver. Pending requeue
// records are cleared; the history keeps every call for assertions.
func (f *fakeDriver) makeRequeuesDue() {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, rq := range f.requeues {
		f.reserveQ = append(f.reserveQ, rq.m)
	}
	f.requeues = nil
}

func (f *fakeDriver) Push(_ context.Context, m *Message) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	m.ID = int64(len(f.pushed) + 1)
	f.pushed = append(f.pushed, m)
	return nil
}

func (f *fakeDriver) Reserve(context.Context) (*Message, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.reserveQ) == 0 {
		return nil, nil
	}
	m := f.reserveQ[0]
	f.reserveQ = f.reserveQ[1:]
	return m, nil
}

func (f *fakeDriver) Complete(_ context.Context, m *Message) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.completed = append(f.completed, m)
	return nil
}

func (f *fakeDriver) Requeue(_ context.Context, m *Message, delay time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requeues = append(f.requeues, requeueCall{m: m, delay: delay})
	f.requeueHistory = append(f.requeueHistory, requeueCall{m: m, delay: delay})
	return nil
}

func (f *fakeDriver) Fail(_ context.Context, m *Message) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failed = append(f.failed, m)
	return nil
}

func (f *fakeDriver) RetryFailed(context.Context) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.retried++
	return len(f.failed), nil
}

// queueForDrain moves every pushed message into the reserve queue, the way a
// real driver makes pushed messages due.
func (f *fakeDriver) queueForDrain() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reserveQ = append(f.reserveQ, f.pushed...)
	f.pushed = nil
}

// eventLog records triggered events in order.
type eventLog struct {
	mu     sync.Mutex
	events []string
	data   []any
}

func (l *eventLog) dispatch(_ context.Context, event string, data any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.events = append(l.events, event)
	l.data = append(l.data, data)
}

func (l *eventLog) snapshot() ([]string, []any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.events...), append([]any(nil), l.data...)
}

// execSink records every countedJob execution. The registry rebuilds fresh
// job instances from the payload, so run observations must live outside the
// job struct — only payload-carried values (Value/N/Failures) travel to the
// rebuilt copy.
type execSink struct {
	mu   sync.Mutex
	runs []sinkRun
}

type sinkRun struct {
	Value string
	N     int
}

var sink = &execSink{}

func resetSink() {
	sink.mu.Lock()
	sink.runs = nil
	sink.mu.Unlock()
}

// recordRun logs one execution and returns its 1-based number.
func (s *execSink) recordRun(v string, n int) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.runs = append(s.runs, sinkRun{Value: v, N: n})
	return len(s.runs)
}

func (s *execSink) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.runs)
}

func (s *execSink) recorded() []sinkRun {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]sinkRun(nil), s.runs...)
}

// countedJob is the test workhorse. Failures (payload-carried) is how many of
// its runs fail before it starts succeeding; every run lands in the shared
// sink with the payload it was rebuilt from.
type countedJob struct {
	Value    string        `json:"value,omitempty"`
	N        int           `json:"n,omitempty"`
	Failures int           `json:"failures,omitempty"`
	Slow     time.Duration `json:"slow,omitempty"`
}

func (*countedJob) Name() string { return "counted" }

func (j *countedJob) Execute(ctx context.Context) error {
	n := sink.recordRun(j.Value, j.N)
	if j.Slow > 0 {
		select {
		case <-time.After(j.Slow):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if n <= j.Failures {
		return fmt.Errorf("boom #%d", n)
	}
	return nil
}

// newTestQueue wires a Queue over a fake driver with the counted job
// registered and the event log attached as dispatcher.
func newTestQueue(t *testing.T) (*Queue, *fakeDriver, *eventLog) {
	t.Helper()
	resetSink()
	reg, err := NewRegistry(&countedJob{})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	log := &eventLog{}
	drv := &fakeDriver{}
	q := NewWithDriver(drv, reg, WithDispatcher(log.dispatch), WithPollInterval(time.Millisecond))
	return q, drv, log
}

// TestPushOptionsDefaults verifies the zero options: one attempt, the default
// TTR, no retry delays, due immediately.
func TestPushOptionsDefaults(t *testing.T) {
	q, drv, _ := newTestQueue(t)
	ctx := context.Background()

	if _, err := q.Push(ctx, &countedJob{}); err != nil {
		t.Fatalf("Push: %v", err)
	}
	if len(drv.pushed) != 1 {
		t.Fatalf("pushed %d messages, want 1", len(drv.pushed))
	}
	m := drv.pushed[0]
	if m.Attempts != 1 {
		t.Errorf("Attempts = %d, want 1 (no retries by default)", m.Attempts)
	}
	if m.TTR != DefaultTTR {
		t.Errorf("TTR = %v, want %v", m.TTR, DefaultTTR)
	}
	if len(m.RetryDelays) != 0 {
		t.Errorf("RetryDelays = %v, want none for Attempts=1", m.RetryDelays)
	}
	if m.Priority != 0 {
		t.Errorf("Priority = %d, want 0", m.Priority)
	}
	if m.Name != "counted" {
		t.Errorf("Name = %q, want counted", m.Name)
	}
	if m.DueAt.Before(m.PushedAt) {
		t.Errorf("DueAt %v before PushedAt %v, want immediate due", m.DueAt, m.PushedAt)
	}
}

// TestPushSerializesPayload verifies the payload is the job's JSON encoding.
func TestPushSerializesPayload(t *testing.T) {
	q, drv, _ := newTestQueue(t)
	ctx := context.Background()

	if _, err := q.Push(ctx, &countedJob{Value: "x", N: 3}); err != nil {
		t.Fatalf("Push: %v", err)
	}
	if got, want := string(drv.pushed[0].Payload), `{"value":"x","n":3}`; got != want {
		t.Errorf("payload = %s, want %s", got, want)
	}
}

// TestPushOptionsFunctional verifies each option reaches the message.
func TestPushOptionsFunctional(t *testing.T) {
	q, drv, _ := newTestQueue(t)
	ctx := context.Background()

	_, err := q.Push(ctx, &countedJob{},
		WithDelay(2*time.Minute),
		WithPriority(7),
		WithAttempts(3),
		WithTTR(time.Minute),
		WithBackoff(func(attempt int) time.Duration { return time.Duration(attempt) * 10 * time.Millisecond }),
	)
	if err != nil {
		t.Fatalf("Push: %v", err)
	}
	m := drv.pushed[0]
	if got := m.DueAt.Sub(m.PushedAt); got < 2*time.Minute-time.Second || got > 2*time.Minute+time.Second {
		t.Errorf("due delta = %v, want ~2m", got)
	}
	if m.Priority != 7 {
		t.Errorf("Priority = %d, want 7", m.Priority)
	}
	if m.Attempts != 3 {
		t.Errorf("Attempts = %d, want 3", m.Attempts)
	}
	if m.TTR != time.Minute {
		t.Errorf("TTR = %v, want 1m", m.TTR)
	}
	want := []time.Duration{10 * time.Millisecond, 20 * time.Millisecond}
	if len(m.RetryDelays) != len(want) {
		t.Fatalf("RetryDelays = %v, want %v", m.RetryDelays, want)
	}
	for i := range want {
		if m.RetryDelays[i] != want[i] {
			t.Errorf("RetryDelays[%d] = %v, want %v", i, m.RetryDelays[i], want[i])
		}
	}

	// Attempts below 1 clamps to 1; the TTR default holds without WithTTR.
	_, err = q.Push(ctx, &countedJob{}, WithAttempts(0))
	if err != nil {
		t.Fatalf("Push with Attempts=0: %v", err)
	}
	if m := drv.pushed[1]; m.Attempts != 1 || m.TTR != DefaultTTR {
		t.Errorf("after WithAttempts(0): Attempts=%d TTR=%v, want 1 / default", m.Attempts, m.TTR)
	}
}

// TestDefaultBackoff verifies the default retry schedule: 2^attempt seconds
// capped at fifteen minutes.
func TestDefaultBackoff(t *testing.T) {
	cases := []struct {
		attempt int
		want    time.Duration
	}{
		{1, 2 * time.Second},
		{2, 4 * time.Second},
		{3, 8 * time.Second},
		{9, 512 * time.Second},
		{10, 15 * time.Minute}, // 2^10s = 1024s crosses the cap
		{12, 15 * time.Minute},
		{40, 15 * time.Minute},
		{0, 2 * time.Second}, // clamped to attempt 1
		{-3, 2 * time.Second},
	}
	for _, tc := range cases {
		if got := DefaultBackoff(tc.attempt); got != tc.want {
			t.Errorf("DefaultBackoff(%d) = %v, want %v", tc.attempt, got, tc.want)
		}
	}
}

// TestRegistryBasics covers Register validation, duplicate rejection, Get and
// sorted Names.
func TestRegistryBasics(t *testing.T) {
	r := new(Registry)
	if err := r.Register(&countedJob{}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := r.Register(&namedJob{name: "a"}); err != nil {
		t.Fatalf("Register a: %v", err)
	}
	if err := r.Register(&namedJob{name: "a"}); err == nil {
		t.Error("duplicate name must be rejected")
	}
	if err := r.Register(nil); err == nil {
		t.Error("nil job must be rejected")
	}
	if err := r.Register((*countedJob)(nil)); err == nil {
		t.Error("typed nil job must be rejected")
	}
	if err := r.Register(&namedJob{name: ""}); err == nil {
		t.Error("empty name must be rejected")
	}
	if err := r.Register(valueJob{}); err == nil {
		t.Error("non-pointer job must be rejected")
	}
	if _, ok := r.Get("zz"); ok {
		t.Error("Get of unknown name reported found")
	}
	if _, ok := r.Get("a"); !ok {
		t.Error("Get of registered name reported missing")
	}
	if got := r.Names(); fmt.Sprint(got) != "[a counted]" {
		t.Errorf("Names() = %v, want [a counted]", got)
	}

	if _, err := NewRegistry(&namedJob{name: "x"}, &namedJob{name: "x"}); err == nil {
		t.Error("NewRegistry with duplicates must fail")
	}
}

// namedJob is a minimal registered job for registry bookkeeping tests.
type namedJob struct{ name string }

func (j *namedJob) Name() string                { return j.name }
func (*namedJob) Execute(context.Context) error { return nil }

// valueJob implements Job on a value receiver; the registry must refuse it.
type valueJob struct{}

func (valueJob) Name() string                  { return "value" }
func (valueJob) Execute(context.Context) error { return nil }

// TestRegistryBuildRebuildsFromPayload verifies the worker-side rebuild: a
// fresh instance of the prototype's type, decoded from the JSON payload, and
// the prototype itself untouched.
func TestRegistryBuildRebuildsFromPayload(t *testing.T) {
	r := new(Registry)
	proto := &countedJob{Value: "proto"}
	if err := r.Register(proto); err != nil {
		t.Fatalf("Register: %v", err)
	}
	payload := []byte(`{"value":"hello","n":42}`)
	job, err := r.Build("counted", payload)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	got, ok := job.(*countedJob)
	if !ok {
		t.Fatalf("Build returned %T, want *countedJob", job)
	}
	if got == proto {
		t.Fatal("Build returned the prototype itself; want a fresh instance")
	}
	if got.Value != "hello" || got.N != 42 {
		t.Errorf("decoded payload = {value:%s n:%d}, want {hello 42}", got.Value, got.N)
	}

	if _, err := r.Build("missing", payload); err == nil {
		t.Error("Build of unknown name must fail")
	}
	if _, err := r.Build("counted", []byte("{not json")); err == nil {
		t.Error("Build with broken payload must fail")
	}
}

// TestRunDrainsAndCompletes verifies Run processes every due message exactly
// once and reports the count.
func TestRunDrainsAndCompletes(t *testing.T) {
	q, drv, _ := newTestQueue(t)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		if _, err := q.Push(ctx, &countedJob{}); err != nil {
			t.Fatalf("Push: %v", err)
		}
	}
	drv.queueForDrain()

	n, err := q.Run(ctx)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if n != 3 {
		t.Errorf("Run processed %d, want 3", n)
	}
	if sink.count() != 3 {
		t.Errorf("job ran %d times, want 3", sink.count())
	}
	if len(drv.completed) != 3 {
		t.Errorf("completed %d, want 3", len(drv.completed))
	}
	if len(drv.requeues) != 0 || len(drv.failed) != 0 {
		t.Errorf("requeues=%v failed=%v, want none", drv.requeues, drv.failed)
	}
}

// TestRunDeliversPayload verifies the worker rebuilds the job with the pushed
// payload before executing.
func TestRunDeliversPayload(t *testing.T) {
	q, drv, _ := newTestQueue(t)
	ctx := context.Background()

	if _, err := q.Push(ctx, &countedJob{Value: "deferred", N: 9}); err != nil {
		t.Fatalf("Push: %v", err)
	}
	drv.queueForDrain()

	if _, err := q.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
	runs := sink.recorded()
	if len(runs) != 1 || runs[0].Value != "deferred" || runs[0].N != 9 {
		t.Errorf("executed with payload %+v, want one run {deferred 9}", runs)
	}
}

// TestRunRetriesWithBackoffUntilSuccess verifies the retry loop: failing
// attempts requeue with the materialized backoff delay, later attempts run
// after the requeue, and success completes the message.
func TestRunRetriesWithBackoffUntilSuccess(t *testing.T) {
	q, drv, _ := newTestQueue(t)
	ctx := context.Background()

	if _, err := q.Push(ctx, &countedJob{Failures: 2},
		WithAttempts(3),
		WithBackoff(func(int) time.Duration { return 5 * time.Millisecond }),
	); err != nil {
		t.Fatalf("Push: %v", err)
	}
	drv.queueForDrain()

	// First pass: one failure, requeued with the 5ms backoff.
	if _, err := q.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if sink.count() != 1 {
		t.Fatalf("runs after first pass = %d, want 1", sink.count())
	}
	if len(drv.requeues) != 1 || drv.requeues[0].delay != 5*time.Millisecond {
		t.Fatalf("requeues = %+v, want one 5ms requeue", drv.requeues)
	}
	// The requeued message waits out its delay in a real driver; make it due
	// and drain again, twice, covering the second failure and the success.
	drv.makeRequeuesDue()
	if _, err := q.Run(ctx); err != nil {
		t.Fatalf("Run 2: %v", err)
	}
	drv.makeRequeuesDue()
	if _, err := q.Run(ctx); err != nil {
		t.Fatalf("Run 3: %v", err)
	}
	if sink.count() != 3 {
		t.Errorf("runs = %d, want 3 (two failures then success)", sink.count())
	}
	if len(drv.failed) != 0 {
		t.Errorf("failed = %d, want 0", len(drv.failed))
	}
	if len(drv.completed) != 1 {
		t.Errorf("completed = %d, want 1", len(drv.completed))
	}
	if got := requeueDelays(drv); fmt.Sprint(got) != fmt.Sprint([]time.Duration{5 * time.Millisecond, 5 * time.Millisecond}) {
		t.Errorf("requeue delays = %v, want two 5ms requeues", got)
	}
}

// TestRunAttemptsExhaustedParksFailed verifies the terminal state: once the
// attempts are spent the message parks in failed instead of requeueing.
func TestRunAttemptsExhaustedParksFailed(t *testing.T) {
	q, drv, _ := newTestQueue(t)
	ctx := context.Background()

	if _, err := q.Push(ctx, &countedJob{Failures: 10}, WithAttempts(2)); err != nil {
		t.Fatalf("Push: %v", err)
	}
	drv.queueForDrain()

	if _, err := q.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
	drv.makeRequeuesDue() // the retry delay elapses; the last attempt runs
	if _, err := q.Run(ctx); err != nil {
		t.Fatalf("Run 2: %v", err)
	}
	if sink.count() != 2 {
		t.Errorf("runs = %d, want 2", sink.count())
	}
	if len(drv.requeueHistory) != 1 {
		t.Errorf("requeues = %d, want 1", len(drv.requeueHistory))
	}
	if len(drv.failed) != 1 {
		t.Fatalf("failed = %d, want 1", len(drv.failed))
	}
	if got := drv.failed[0].Attempt; got != 2 {
		t.Errorf("failed message Attempt = %d, want 2", got)
	}
}

// TestRunRetryDelayFallsBackToDefault verifies a message whose stored retry
// delays are missing (hand-crafted or truncated) falls back to DefaultBackoff.
func TestRunRetryDelayFallsBackToDefault(t *testing.T) {
	q, drv, _ := newTestQueue(t)
	ctx := context.Background()

	m := &Message{
		Name: "counted", Payload: []byte(`{"failures":1}`), Attempts: 2,
		TTR: DefaultTTR, DueAt: time.Now(), PushedAt: time.Now(),
	}
	drv.mu.Lock()
	drv.reserveQ = append(drv.reserveQ, m)
	drv.mu.Unlock()

	if _, err := q.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(drv.requeues) != 1 || drv.requeues[0].delay != DefaultBackoff(1) {
		t.Fatalf("requeues = %+v, want one requeue with DefaultBackoff(1)", drv.requeues)
	}
}

// requeueDelays returns the delays of every requeue ever recorded.
func requeueDelays(f *fakeDriver) []time.Duration {
	f.mu.Lock()
	defer f.mu.Unlock()
	delays := make([]time.Duration, len(f.requeueHistory))
	for i, rq := range f.requeueHistory {
		delays[i] = rq.delay
	}
	return delays
}

// TestRunUnknownJobParksFailed verifies an unregistered name fires ExecError
// and parks as failed instead of spinning.
func TestRunUnknownJobParksFailed(t *testing.T) {
	q, drv, log := newTestQueue(t)
	ctx := context.Background()

	m := &Message{Name: "ghost", Payload: []byte(`{}`), Attempts: 3, TTR: DefaultTTR, DueAt: time.Now()}
	drv.mu.Lock()
	drv.reserveQ = append(drv.reserveQ, m)
	drv.mu.Unlock()

	if _, err := q.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(drv.failed) != 1 {
		t.Fatalf("failed = %d, want 1", len(drv.failed))
	}
	events, _ := log.snapshot()
	found := false
	for _, e := range events {
		if e == EventExecError {
			found = true
		}
	}
	if !found {
		t.Errorf("events = %v, want an %s entry", events, EventExecError)
	}
}

// TestEventSequence verifies the five lifecycle events and their payloads for
// a successful push and execution.
func TestEventSequence(t *testing.T) {
	q, drv, log := newTestQueue(t)
	ctx := context.Background()

	if _, err := q.Push(ctx, &countedJob{}, WithPriority(5), WithDelay(time.Second), WithAttempts(2)); err != nil {
		t.Fatalf("Push: %v", err)
	}
	drv.queueForDrain()
	if _, err := q.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}

	events, data := log.snapshot()
	want := []string{EventBeforePush, EventAfterPush, EventBeforeExec, EventAfterExec}
	if fmt.Sprint(events) != fmt.Sprint(want) {
		t.Fatalf("events = %v, want %v", events, want)
	}
	push := data[0].(PushEventData)
	if push.Job != "counted" || push.Priority != 5 || push.Delay != time.Second || push.Attempts != 2 {
		t.Errorf("BeforePush data = %+v", push)
	}
	if after := data[1].(PushEventData); after.Job != "counted" {
		t.Errorf("AfterPush data = %+v", after)
	}
	exec := data[2].(ExecEventData)
	if exec.Job != "counted" || exec.ID != 1 || exec.Attempt != 1 || exec.Err != nil {
		t.Errorf("BeforeExec data = %+v", exec)
	}
	if after := data[3].(ExecEventData); after.Attempt != 1 {
		t.Errorf("AfterExec data = %+v", after)
	}
}

// TestExecErrorEventCarriesError verifies the failure path triggers ExecError
// with the job's error and the attempt number.
func TestExecErrorEventCarriesError(t *testing.T) {
	q, drv, log := newTestQueue(t)
	ctx := context.Background()

	if _, err := q.Push(ctx, &countedJob{Failures: 1}); err != nil {
		t.Fatalf("Push: %v", err)
	}
	drv.queueForDrain()
	if _, err := q.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}

	_, data := log.snapshot()
	var got *ExecEventData
	for _, d := range data {
		if e, ok := d.(ExecEventData); ok && e.Err != nil {
			got = &e
		}
	}
	if got == nil {
		t.Fatal("no ExecError data recorded")
	}
	if got.Err == nil || got.Err.Error() != "boom #1" {
		t.Errorf("ExecError Err = %v, want boom #1", got.Err)
	}
	if got.Attempt != 1 || got.Job != "counted" || got.ID != 1 {
		t.Errorf("ExecError data = %+v", got)
	}
}

// TestListenStopsOnCancel verifies the listen loop keeps polling until the
// context is canceled, then exits nil.
func TestListenStopsOnCancel(t *testing.T) {
	q, drv, _ := newTestQueue(t)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- q.Listen(ctx) }()

	// Nothing due: the loop just polls. Cancel and expect a prompt nil exit.
	time.Sleep(20 * time.Millisecond)
	if len(drv.completed) != 0 {
		t.Fatalf("unexpected completions: %d", len(drv.completed))
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Listen after cancel: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Listen did not exit after cancel")
	}
}

// TestListenProcessesMessagesAsTheyBecomeDue verifies a message pushed while
// listening is picked up without restarting the worker.
func TestListenProcessesMessagesAsTheyBecomeDue(t *testing.T) {
	q, drv, _ := newTestQueue(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- q.Listen(ctx) }()

	if _, err := q.Push(ctx, &countedJob{}); err != nil {
		t.Fatalf("Push: %v", err)
	}
	drv.queueForDrain()

	deadline := time.Now().Add(5 * time.Second)
	for sink.count() == 0 && time.Now().Before(deadline) {
		time.Sleep(2 * time.Millisecond)
	}
	if sink.count() == 0 {
		cancel()
		t.Fatal("listen loop never executed the pushed job")
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("Listen: %v", err)
	}
}

// TestRetryFailedDelegates verifies RetryFailed forwards to the driver.
func TestRetryFailedDelegates(t *testing.T) {
	q, drv, _ := newTestQueue(t)
	drv.mu.Lock()
	drv.failed = append(drv.failed, &Message{ID: 1})
	drv.mu.Unlock()

	n, err := q.RetryFailed(context.Background())
	if err != nil {
		t.Fatalf("RetryFailed: %v", err)
	}
	if n != 1 {
		t.Errorf("RetryFailed = %d, want 1", n)
	}
}

// TestQueueWithoutRegistry verifies New with a nil registry still works (an
// empty registry the application fills later).
func TestQueueWithoutRegistry(t *testing.T) {
	q := NewWithDriver(&fakeDriver{}, nil)
	if q.Jobs() == nil {
		t.Fatal("Jobs() returned nil for a nil-construction registry")
	}
	if _, err := q.Jobs().Build("counted", []byte(`{}`)); err == nil {
		t.Error("Build against an empty registry must fail")
	}
	var concurrent atomic.Int32
	_ = concurrent.Load()
}
