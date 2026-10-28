package base

import (
	"context"
	"sync"
)

// EventName identifies a dispatchable event. The application lifecycle events
// are predefined constants; applications define their own events as
// additional EventName values.
type EventName string

// Application lifecycle events. The web layer triggers them around each
// request; until that wiring lands, they can be triggered directly.
const (
	// EventBeforeRequest fires before a request enters routing and
	// controllers.
	EventBeforeRequest EventName = "beforeRequest"
	// EventAfterRequest fires after the response has been produced.
	EventAfterRequest EventName = "afterRequest"
)

// Event is the payload handed to handlers: a concrete struct with the event
// name and an optional event-specific data value, instead of a dynamic
// key-value payload. The request context travels beside it as the ctx
// argument of Handler.
type Event struct {
	// Name is the event being dispatched.
	Name EventName
	// Data optionally carries event-specific values set by the trigger site;
	// the lifecycle events leave it nil.
	Data any
}

// Handler processes one dispatched event. ctx is the context the event was
// triggered with, so handlers can flow request-scoped state.
type Handler func(ctx context.Context, e Event)

// subscription pairs a handler with an identity that can be matched for
// removal, since function values are not comparable in Go.
type subscription struct {
	handler Handler
}

// eventRegistry holds the handler lists per event; safe for concurrent use.
type eventRegistry struct {
	mu       sync.RWMutex
	handlers map[EventName][]*subscription
}

// on appends h to the handlers of name and returns a function that
// unsubscribes it again; the returned function is idempotent.
func (r *eventRegistry) on(name EventName, h Handler) func() {
	sub := &subscription{handler: h}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.handlers == nil {
		r.handlers = make(map[EventName][]*subscription)
	}
	r.handlers[name] = append(r.handlers[name], sub)
	return func() { r.off(name, sub) }
}

// off removes sub from the handlers of name if still present.
func (r *eventRegistry) off(name EventName, sub *subscription) {
	r.mu.Lock()
	defer r.mu.Unlock()
	list := r.handlers[name]
	for i, cur := range list {
		if cur == sub {
			r.handlers[name] = append(list[:i], list[i+1:]...)
			if len(r.handlers[name]) == 0 {
				delete(r.handlers, name)
			}
			return
		}
	}
}

// trigger invokes the current handlers of e.Name in registration order,
// carrying ctx. The list is snapshotted before invoking, so handlers may
// subscribe or unsubscribe (including themselves) while dispatch is in
// flight; such changes take effect from the next trigger on.
func (r *eventRegistry) trigger(ctx context.Context, e Event) {
	r.mu.RLock()
	subs := make([]*subscription, len(r.handlers[e.Name]))
	copy(subs, r.handlers[e.Name])
	r.mu.RUnlock()
	for _, sub := range subs {
		sub.handler(ctx, e)
	}
}
