package base

import (
	"context"
	"fmt"
	"strings"
	"time"

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
// configuration section. The sync driver (the default) touches nothing; the
// db driver draws the application's database connection (its table comes
// from queue.NewMigration, applied by the application's migrations); the
// redis driver shares the application's redis connection unconditionally —
// a queue coordinates processes, so a per-section override would hand out
// messages nobody else sees. A driver the section cannot name — or a
// database that will not open — yields a queue whose operations fail with
// the configuration error at first use instead of breaking application
// assembly.
func newDefaultQueue(cfg config.Queue, dbComp *db.Component, shared *component[redis.UniversalClient], a *Application) *queue.Queue {
	dispatch := queue.Dispatcher(func(ctx context.Context, event string, data any) {
		a.Trigger(ctx, EventName(event), data)
	})
	poll := time.Duration(cfg.PollIntervalMs) * time.Millisecond
	build := func(d queue.Driver) *queue.Queue {
		return queue.New(d, nil, queue.WithDispatcher(dispatch), queue.WithPollInterval(poll))
	}
	switch strings.ToLower(strings.TrimSpace(cfg.Driver)) {
	case "", "sync":
		return build(queue.NewSync())
	case "db":
		conn, err := dbComp.Get()
		if err != nil {
			return build(queue.Failing(fmt.Errorf("queue: db driver needs the database connection: %w", err)))
		}
		return build(queue.NewDB(conn.DB, cfg.TableName))
	case "redis":
		return build(queue.NewRedis(shared.get(), ""))
	default:
		return build(queue.Failing(fmt.Errorf("queue: unknown driver %q (want sync, db or redis)", cfg.Driver)))
	}
}
