// Package queue defers work: jobs are pushed from request code and executed
// later by a worker. Job code never travels with the queue — a push stores the
// job's registered name plus its JSON payload, and the worker rebuilds the job
// from the application's registry before executing it (the same "store the
// name, keep the behavior in code" trade the RBAC rules make).
//
// Three drivers ship: sync (execute inline on push; the development and test
// default), db (a relational table, schema provided by NewMigration) and redis
// (shared redis connection). New selects the driver from Config and takes the
// cross-driver dependencies through options — the db driver's connection with
// WithDB, the redis driver's client with WithRedis; a driver whose dependency
// is missing fails construction. NewWithDriver wraps an already-chosen driver
// (a custom one, or Failing for a deferred configuration error). All durable
// drivers share one execution state machine: waiting -> reserved -> done, with
// failed attempts requeued after a backoff delay, exhausted messages parked as
// failed for RetryFailed, and reservations whose TTR elapsed reclaimed to
// waiting so a crashed worker cannot lose a job.
package queue
