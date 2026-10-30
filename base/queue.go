package base

import (
	"context"

	"github.com/redis/go-redis/v9"

	"github.com/goyii2/framework/config"
	"github.com/goyii2/framework/db"
	"github.com/goyii2/framework/queue"
)

// Queue returns the queue component, building it from the configuration's
// queue section on first access. The value is the queue package's Queue —
// one job registry over one driver (sync, db or redis) — and the
// application's five lifecycle events (queue.EventBeforePush and friends)
// ride along through this application's event registry. Jobs are
// application code: register them on the component before workers run.
func (a *Application) Queue() *queue.Queue { return a.queueComp.get() }

// SetQueue replaces the queue component (e.g. a test fake or a differently
// configured driver); nil restores lazy initialization from the
// configuration.
func (a *Application) SetQueue(q *queue.Queue) { a.queueComp.set(q) }

// newDefaultQueue builds the default queue component from the queue
// configuration section. The section's values convert one-to-one into a
// queue.Config and the driver selection — including which dependency that
// driver resolves — belongs to the queue package: the database connection
// and redis client are handed over lazily (WithDBFunc / WithRedisFunc), so
// the sync driver touches neither. A construction the queue package rejects
// (an unknown driver, a database that will not open) yields a queue whose
// operations fail with that error at first use instead of breaking
// application assembly.
func newDefaultQueue(cfg config.Queue, dbComp *db.Component, shared *component[redis.UniversalClient], a *Application) *queue.Queue {
	dispatch := queue.Dispatcher(func(ctx context.Context, event string, data any) {
		a.Trigger(ctx, EventName(event), data)
	})
	q, err := queue.New(queue.Config{
		Driver:         cfg.Driver,
		TableName:      cfg.TableName,
		PollIntervalMs: cfg.PollIntervalMs,
	},
		queue.WithDBFunc(dbComp.Get),
		queue.WithRedisFunc(shared.get),
		queue.WithDispatcher(dispatch),
	)
	if err != nil {
		return queue.NewWithDriver(queue.Failing(err), nil, queue.WithDispatcher(dispatch))
	}
	return q
}
