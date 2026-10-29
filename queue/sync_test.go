package queue

import (
	"context"
	"errors"
	"testing"
	"time"
)

// newSyncQueue builds a queue over the sync driver with the counted job
// registered and the event log attached.
func newSyncQueue(t *testing.T) (*Queue, *eventLog) {
	t.Helper()
	resetSink()
	reg, err := NewRegistry(&countedJob{})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	log := &eventLog{}
	q := New(NewSync(), reg, WithDispatcher(log.dispatch))
	return q, log
}

// TestSyncExecutesInline verifies a sync push executes the job on the pushing
// call, succeeds with the payload observed, and fires the push and exec
// events around the inline execution.
func TestSyncExecutesInline(t *testing.T) {
	q, log := newSyncQueue(t)
	ctx := context.Background()

	if _, err := q.Push(ctx, &countedJob{Value: "now", N: 1}); err != nil {
		t.Fatalf("Push: %v", err)
	}
	if sink.count() != 1 {
		t.Fatalf("executions = %d, want 1 (inline on push)", sink.count())
	}
	runs := sink.recorded()
	if runs[0].Value != "now" || runs[0].N != 1 {
		t.Errorf("payload observed = %+v, want {now 1}", runs[0])
	}

	events, _ := log.snapshot()
	want := []string{EventBeforePush, EventBeforeExec, EventAfterExec, EventAfterPush}
	for _, w := range want {
		found := false
		for _, e := range events {
			if e == w {
				found = true
			}
		}
		if !found {
			t.Errorf("events = %v, missing %s", events, w)
		}
	}
}

// TestSyncFailureReturnsToPusher verifies a failing job surfaces its error
// directly to the pusher and fires ExecError — no retries, no storage.
func TestSyncFailureReturnsToPusher(t *testing.T) {
	q, log := newSyncQueue(t)
	ctx := context.Background()

	_, err := q.Push(ctx, &countedJob{Failures: 3}, WithAttempts(5))
	if err == nil || err.Error() != "boom #1" {
		t.Fatalf("Push error = %v, want boom #1", err)
	}
	if sink.count() != 1 {
		t.Errorf("executions = %d, want exactly 1 (sync never retries)", sink.count())
	}

	events, data := log.snapshot()
	if len(events) == 0 || events[len(events)-1] != EventAfterPush {
		t.Errorf("events = %v, want AfterPush last (push itself completed)", events)
	}
	var execErr *ExecEventData
	for _, d := range data {
		if e, ok := d.(ExecEventData); ok && e.Err != nil {
			execErr = &e
		}
	}
	if execErr == nil || execErr.Err.Error() != "boom #1" {
		t.Errorf("ExecError data = %+v, want boom #1", execErr)
	}
}

// TestSyncIgnoresWorkerSurface verifies the sync driver has nothing for the
// worker side: Run reports zero, Listen exits on cancel, RetryFailed reports
// zero.
func TestSyncIgnoresWorkerSurface(t *testing.T) {
	q, _ := newSyncQueue(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if _, err := q.Push(ctx, &countedJob{}); err != nil {
		t.Fatalf("Push: %v", err)
	}
	n, err := q.Run(ctx)
	if err != nil || n != 0 {
		t.Errorf("Run = %d, %v; want 0, nil (nothing stored)", n, err)
	}
	if n, err := q.RetryFailed(ctx); err != nil || n != 0 {
		t.Errorf("RetryFailed = %d, %v; want 0, nil", n, err)
	}

	done := make(chan error, 1)
	go func() { done <- q.Listen(ctx) }()
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

// TestSyncDriverAsStoreErrors verifies direct store use of the sync driver
// reports the miswiring instead of pretending to store.
func TestSyncDriverAsStoreErrors(t *testing.T) {
	d := NewSync()
	ctx := context.Background()
	if err := d.Push(ctx, &Message{}); !errors.Is(err, errSyncStoresNothing) {
		t.Errorf("Push = %v, want errSyncStoresNothing", err)
	}
}
