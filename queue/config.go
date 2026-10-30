package queue

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/goyii2/framework/db"
)

// Config carries the queue component values. The zero value selects the sync
// driver; the db driver additionally requires a connection injected with
// WithDB, the redis driver a client injected with WithRedis.
type Config struct {
	Driver    string `yaml:"driver"`    // sync | db | redis; empty = sync
	TableName string `yaml:"tableName"` // db driver table; default queue

	// PollIntervalMs is how long the listen worker waits between drain
	// passes when idle; a short value trades CPU for latency in tests.
	// Values below one select the one-second default.
	PollIntervalMs int `yaml:"pollIntervalMs"`
}

// Option configures a Queue at construction; options carry cross-component
// dependencies (WithDB, WithRedis) and the event dispatcher, values live in
// Config. Both New and NewWithDriver take them.
type Option func(*options)

// options accumulates what the options set. The db and redis dependencies
// are resolution functions so a queue whose driver needs neither never
// touches them — WithDBFunc installs a lazily opening one.
type options struct {
	db     func() (*db.Conn, error)
	redis  func() redis.UniversalClient
	events Dispatcher
	poll   time.Duration
}

// WithDB hands the queue the application's database connection for the db
// driver — the connection the application's DML runs on, so jobs see the
// same data the pushing request committed. The sync and redis drivers
// ignore it.
func WithDB(conn *db.Conn) Option {
	return func(o *options) {
		o.db = func() (*db.Conn, error) { return conn, nil }
	}
}

// WithDBFunc is the lazy WithDB: New opens the connection through open when
// (and only when) the db driver needs it, so the sync and redis drivers
// never open a database. Applications assembling from a retryable holder
// (db.Component.Get) pass it directly.
func WithDBFunc(open func() (*db.Conn, error)) Option {
	return func(o *options) { o.db = open }
}

// WithRedis hands the queue the application's shared redis client for the
// redis driver — shared unconditionally, because a queue coordinates
// processes and a private client would hand out messages nobody else sees.
// The sync and db drivers ignore it.
func WithRedis(client redis.UniversalClient) Option {
	return func(o *options) {
		o.redis = func() redis.UniversalClient { return client }
	}
}

// WithRedisFunc is the lazy WithRedis: New resolves the client through get
// when (and only when) the redis driver needs it, so the sync and db
// drivers never touch the redis connection.
func WithRedisFunc(get func() redis.UniversalClient) Option {
	return func(o *options) { o.redis = get }
}

// WithDispatcher routes the five lifecycle events through d.
func WithDispatcher(d Dispatcher) Option { return func(o *options) { o.events = d } }

// WithPollInterval sets how long Listen waits between drain passes when the
// queue is idle; d must be positive. NewWithDriver honors it; New takes the
// interval from Config's PollIntervalMs.
func WithPollInterval(d time.Duration) Option {
	return func(o *options) {
		if d > 0 {
			o.poll = d
		}
	}
}

// applyOptions folds opts into o, skipping nil entries.
func applyOptions(opts []Option, o *options) {
	for _, opt := range opts {
		if opt != nil {
			opt(o)
		}
	}
}

// pollOf resolves the listen poll interval from the Config values: the
// configured milliseconds, one second when unset or invalid.
func pollOf(cfg Config) time.Duration {
	if cfg.PollIntervalMs > 0 {
		return time.Duration(cfg.PollIntervalMs) * time.Millisecond
	}
	return time.Second
}

// errDBRequired is the construction error of a db driver whose connection
// dependency is absent (no option, or a nil resolution).
var errDBRequired = errors.New("queue: db driver requires the database connection (pass one with WithDB)")

// errRedisRequired is the construction error of a redis driver whose client
// dependency is absent (no option, or a nil resolution).
var errRedisRequired = errors.New("queue: redis driver requires a redis client (pass one with WithRedis)")

// resolveDB resolves the db driver's connection requirement: a nil provider
// or nil connection is the same missing dependency, a failing open keeps its
// cause.
func resolveDB(get func() (*db.Conn, error)) (*db.Conn, error) {
	if get == nil {
		return nil, errDBRequired
	}
	conn, err := get()
	if err != nil {
		return nil, fmt.Errorf("queue: db driver needs the database connection: %w", err)
	}
	if conn == nil {
		return nil, errDBRequired
	}
	return conn, nil
}

// resolveRedis resolves the redis driver's client requirement: a nil
// provider or nil client is the same missing dependency.
func resolveRedis(get func() redis.UniversalClient) (redis.UniversalClient, error) {
	if get == nil {
		return nil, errRedisRequired
	}
	client := get()
	if client == nil {
		return nil, errRedisRequired
	}
	return client, nil
}

// New builds the queue Config selects: "sync" (also the empty default)
// executes jobs inline on push, "db" stores messages in a relational table
// (schema from NewMigration) over the connection injected with WithDB, and
// "redis" keeps them in redis over the client injected with WithRedis.
// Omitted values fall back to the defaults: the "queue" table name and the
// one-second listen poll interval. A driver Config cannot name — or a
// dependency its driver needs but no option injected, or one whose lazy
// resolution fails — returns an error instead of a queue.
func New(cfg Config, opts ...Option) (*Queue, error) {
	var o options
	applyOptions(opts, &o)
	poll := pollOf(cfg)
	switch strings.ToLower(strings.TrimSpace(cfg.Driver)) {
	case "", "sync":
		return newOverDriver(NewSync(), nil, o, poll), nil
	case "db":
		conn, err := resolveDB(o.db)
		if err != nil {
			return nil, err
		}
		return newOverDriver(NewDB(conn.DB, cfg.TableName), nil, o, poll), nil
	case "redis":
		client, err := resolveRedis(o.redis)
		if err != nil {
			return nil, err
		}
		return newOverDriver(NewRedis(client, ""), nil, o, poll), nil
	default:
		return nil, fmt.Errorf("queue: unknown driver %q (want sync, db or redis)", cfg.Driver)
	}
}
