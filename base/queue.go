package base

import (
	"context"

	"github.com/goyii2/framework/queue"
)

// Queue returns the queue component. The value is the queue package's Queue
// — one job registry over one driver (sync, db or redis) — and the
// application's five lifecycle events (queue.EventBeforePush and friends)
// ride along through this application's event registry when the queue was
// built with QueueDispatcher (the fallback is). Jobs are application code:
// register them on the component before workers run.
func (a *Application) Queue() *queue.Queue { return a.queue }

// SetQueue replaces the queue component (e.g. the queue built from the
// project's configuration, with its driver's dependencies injected and
// QueueDispatcher beside them); nil restores the sync fallback.
func (a *Application) SetQueue(q *queue.Queue) {
	if q == nil {
		q = a.newDefaultQueue()
	}
	a.queue = q
}

// QueueDispatcher returns the queue option routing the five lifecycle events
// through this application's event registry; pass it to queue.New beside the
// driver's dependency options so queue events and request events observe one
// subscriber table.
func (a *Application) QueueDispatcher() queue.Option {
	return queue.WithDispatcher(queue.Dispatcher(func(ctx context.Context, event string, data any) {
		a.Trigger(ctx, EventName(event), data)
	}))
}

// newDefaultQueue builds the fallback queue component: the sync driver,
// executing jobs inline on push and touching no external dependency. The
// zero-value queue.Config selects it, so this constructor cannot fail; a
// failure here is a framework bug and panics.
func (a *Application) newDefaultQueue() *queue.Queue {
	q, err := queue.New(queue.Config{}, a.QueueDispatcher())
	if err != nil {
		panic("goyii: building the default queue component: " + err.Error())
	}
	return q
}
