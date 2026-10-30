package base

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
)

func TestEventTriggerDeliversEventAndContext(t *testing.T) {
	app := New()
	type payload struct{ Path string }

	ctx := app.WithContext(context.Background())
	var gotCtx context.Context
	var gotEvent Event
	app.On(EventName("user.login"), func(ctx context.Context, e Event) {
		gotCtx = ctx
		gotEvent = e
	})

	app.Trigger(ctx, EventName("user.login"), payload{Path: "/login"})

	if gotEvent.Name != EventName("user.login") {
		t.Errorf("handler received event name %q, want %q", gotEvent.Name, "user.login")
	}
	if p, ok := gotEvent.Data.(payload); !ok || p.Path != "/login" {
		t.Errorf("handler received data %#v, want payload{Path: %q}", gotEvent.Data, "/login")
	}
	if gotCtx != ctx {
		t.Error("handler must receive the context the event was triggered with")
	}
}

func TestEventHandlersRunInRegistrationOrder(t *testing.T) {
	app := New()

	var calls []string
	for _, tag := range []string{"a", "b", "c"} {
		app.On(EventName("ordered"), func(ctx context.Context, e Event) {
			calls = append(calls, tag)
		})
	}

	app.Trigger(context.Background(), EventName("ordered"), nil)

	if len(calls) != 3 || calls[0] != "a" || calls[1] != "b" || calls[2] != "c" {
		t.Errorf("handler calls = %v, want [a b c]", calls)
	}
}

func TestEventOffUnsubscribesOnlyTargetHandler(t *testing.T) {
	app := New()

	var calls []string
	app.On(EventName("tick"), func(ctx context.Context, e Event) {
		calls = append(calls, "keep")
	})
	off := app.On(EventName("tick"), func(ctx context.Context, e Event) {
		calls = append(calls, "drop")
	})

	off()
	off() // must be idempotent

	app.Trigger(context.Background(), EventName("tick"), nil)

	if len(calls) != 1 || calls[0] != "keep" {
		t.Errorf("handler calls after unsubscribe = %v, want [keep]", calls)
	}
}

// TestEventOffDuringDispatchTakesEffectNextTrigger pins the dispatch
// semantics: a handler may unsubscribe itself and subscribe new handlers
// while dispatch is in flight; the current dispatch still completes the
// snapshot it started with, and changes apply from the next trigger on.
func TestEventOffDuringDispatchTakesEffectNextTrigger(t *testing.T) {
	app := New()

	var calls []string
	app.On(EventName("tick"), func(ctx context.Context, e Event) {
		calls = append(calls, "first")
	})
	var offSecond func()
	offSecond = app.On(EventName("tick"), func(ctx context.Context, e Event) {
		calls = append(calls, "second")
		offSecond()
		app.On(EventName("tick"), func(ctx context.Context, e Event) {
			calls = append(calls, "third")
		})
	})

	app.Trigger(context.Background(), EventName("tick"), nil)
	app.Trigger(context.Background(), EventName("tick"), nil)

	want := []string{"first", "second", "first", "third"}
	if len(calls) != len(want) {
		t.Fatalf("handler calls = %v, want %v", calls, want)
	}
	for i := range want {
		if calls[i] != want[i] {
			t.Fatalf("handler calls = %v, want %v", calls, want)
		}
	}
}

func TestEventTriggerWithoutSubscribersIsNoop(t *testing.T) {
	app := New()

	off := app.On(EventName("ephemeral"), func(ctx context.Context, e Event) {})
	off()

	app.Trigger(context.Background(), EventName("ephemeral"), nil) // must not panic
	app.Trigger(context.Background(), EventName("never-subscribed"), nil)
}

// TestLifecycleEventsSimulatedRequestTrigger mirrors how the web layer wires
// the request lifecycle: both application events fire around a (simulated)
// request and reach their subscribers with the request context.
func TestLifecycleEventsSimulatedRequestTrigger(t *testing.T) {
	if EventBeforeRequest != "beforeRequest" || EventAfterRequest != "afterRequest" {
		t.Fatalf("lifecycle event names = %q / %q, want %q / %q",
			EventBeforeRequest, EventAfterRequest, "beforeRequest", "afterRequest")
	}

	app := New()
	ctx := app.WithContext(context.Background())

	var seen []EventName
	var seenCtx context.Context
	app.On(EventBeforeRequest, func(ctx context.Context, e Event) {
		seenCtx = ctx
		seen = append(seen, e.Name)
	})
	app.On(EventAfterRequest, func(ctx context.Context, e Event) {
		seen = append(seen, e.Name)
	})

	app.Trigger(ctx, EventBeforeRequest, nil)
	app.Trigger(ctx, EventAfterRequest, nil)

	if len(seen) != 2 || seen[0] != EventBeforeRequest || seen[1] != EventAfterRequest {
		t.Errorf("observed events = %v, want [%v %v]", seen, EventBeforeRequest, EventAfterRequest)
	}
	if seenCtx != ctx {
		t.Error("BeforeRequest handler must receive the request context")
	}
}

// TestEventConcurrentSubscribeTriggerOff mixes subscription, unsubscription
// and dispatch from many goroutines; run with -race. Transient subscriptions
// are cancelled immediately, so once everything quiesces exactly the stable
// subscribers receive a final trigger.
func TestEventConcurrentSubscribeTriggerOff(t *testing.T) {
	app := New()
	name := EventName("custom")

	var delivered atomic.Int64
	var stable atomic.Int64
	for i := 0; i < 4; i++ {
		app.On(name, func(ctx context.Context, e Event) {
			delivered.Add(1)
		})
		stable.Add(1)
	}

	var wg sync.WaitGroup
	for i := 0; i < 48; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			switch i % 3 {
			case 0:
				off := app.On(name, func(ctx context.Context, e Event) {
					delivered.Add(1)
				})
				off()
				off()
			case 1:
				app.Trigger(context.Background(), name, nil)
			default:
				app.Trigger(context.Background(), EventAfterRequest, nil)
			}
		}(i)
	}
	wg.Wait()

	before := delivered.Load()
	app.Trigger(context.Background(), name, nil)
	if got := delivered.Load() - before; got != stable.Load() {
		t.Errorf("final trigger reached %d handlers, want the %d stable subscribers", got, stable.Load())
	}
}
