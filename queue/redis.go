package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// defaultRedisPrefix namespaces the queue's keys so it can share a redis
// database with caches, sessions and locks.
const defaultRedisPrefix = "goyii:queue:"

// RedisDriver is the redis-backed driver: one shared redis (single node,
// cluster or ring through redis.UniversalClient) holds every message and its
// state. The layout under the prefix:
//
//	seq                 INCR counter assigning message ids
//	msg:<id>            STRING  the message as JSON (payload + options + state)
//	prios               SET     the priority levels in use
//	waiting:<priority>  ZSET    member id, score due time (unix milliseconds)
//	reserved            ZSET    member id, score the reservation's TTR expiry
//	failed              ZSET    member id, score the time it was parked
//
// Sorted-set scores are 64-bit floats, which stop representing integers
// exactly beyond 2^53 — Unix nanoseconds are far past that line — so every
// score is Unix milliseconds, exact in a float and fine-grained enough for
// queue scheduling.
//
// Reservations are claimed by ZREM races: the pop reads the earliest due
// member of a waiting set and removes it — only one racer's ZREM succeeds —
// so workers in any number of processes hand every message to exactly one of
// them. Expired reservations are reclaimed to waiting at the head of every
// Reserve, so a crashed worker cannot lose a message.
type RedisDriver struct {
	client redis.UniversalClient
	prefix string
}

// NewRedis creates the driver over client. prefix is prepended to every key
// ("goyii:queue:" when empty) so independent queue deployments can share one
// database without sharing messages.
func NewRedis(client redis.UniversalClient, prefix string) *RedisDriver {
	if prefix == "" {
		prefix = defaultRedisPrefix
	}
	return &RedisDriver{client: client, prefix: prefix}
}

// msgKey returns the payload key of one message id.
func msgKey(id int64) string { return "msg:" + strconv.FormatInt(id, 10) }

// waitingKey returns the waiting set of one priority level.
func (d *RedisDriver) waitingKey(priority int) string {
	return d.prefix + "waiting:" + strconv.Itoa(priority)
}

// Push assigns m an id and stores it as due at m.DueAt.
func (d *RedisDriver) Push(ctx context.Context, m *Message) error {
	id, err := d.client.Incr(ctx, d.prefix+"seq").Result()
	if err != nil {
		return fmt.Errorf("queue: assign id: %w", err)
	}
	m.ID = id
	data, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("queue: encode %q: %w", m.Name, err)
	}
	pipe := d.client.TxPipeline()
	pipe.Set(ctx, d.prefix+msgKey(id), data, 0)
	pipe.ZAdd(ctx, d.waitingKey(m.Priority), redis.Z{Score: float64(m.DueAt.UnixMilli()), Member: id})
	pipe.SAdd(ctx, d.prefix+"prios", m.Priority)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("queue: store %q: %w", m.Name, err)
	}
	return nil
}

// Reserve reclaims expired reservations, then claims the next due waiting
// message (highest priority first, then earliest due). It returns nil when
// nothing is due.
//
// The claim itself is two commands — ZREM from the waiting set, then ZADD to
// reserved — because branching atomically across two keys needs a Lua script
// with both keys on one hash slot, which redis cluster cannot promise. A
// worker crashing in the microsecond window between the two loses the
// message's membership (its payload key remains). The db driver claims in a
// single statement and has no such window; use it when that gap matters.
func (d *RedisDriver) Reserve(ctx context.Context) (*Message, error) {
	now := time.Now().UnixMilli()

	// Reclaim: reservations whose TTR expiry passed return to waiting, due
	// at once. The attempt count survives — only executions increment it.
	expired, err := d.client.ZRangeByScore(ctx, d.prefix+"reserved", &redis.ZRangeBy{
		Min: "-inf", Max: strconv.FormatInt(now, 10),
	}).Result()
	if err != nil {
		return nil, fmt.Errorf("queue: read expired reservations: %w", err)
	}
	for _, id := range expired {
		m, err := d.load(ctx, id)
		if err != nil {
			return nil, err
		}
		if m == nil { // payload gone: nothing to reclaim, drop the residue
			d.client.ZRem(ctx, d.prefix+"reserved", id)
			continue
		}
		pipe := d.client.TxPipeline()
		pipe.ZAdd(ctx, d.waitingKey(m.Priority), redis.Z{Score: float64(now), Member: id})
		pipe.ZRem(ctx, d.prefix+"reserved", id)
		if _, err := pipe.Exec(ctx); err != nil {
			return nil, fmt.Errorf("queue: reclaim %q (id %s): %w", m.Name, id, err)
		}
	}

	// Claim: scan the priority levels from highest to lowest; in each, the
	// earliest due member is claimed by winning its ZREM. A lost race just
	// moves the scan on.
	prios, err := d.client.SMembers(ctx, d.prefix+"prios").Result()
	if err != nil {
		return nil, fmt.Errorf("queue: read priorities: %w", err)
	}
	levels := make([]int, 0, len(prios))
	for _, p := range prios {
		if n, err := strconv.Atoi(p); err == nil {
			levels = append(levels, n)
		}
	}
	sort.Sort(sort.Reverse(sort.IntSlice(levels)))

	for _, prio := range levels {
		key := d.waitingKey(prio)
		entries, err := d.client.ZRangeWithScores(ctx, key, 0, 0).Result()
		if err != nil {
			return nil, fmt.Errorf("queue: read waiting set: %w", err)
		}
		if len(entries) == 0 || int64(entries[0].Score) > now {
			continue
		}
		id := entries[0].Member.(string)
		removed, err := d.client.ZRem(ctx, key, id).Result()
		if err != nil {
			return nil, fmt.Errorf("queue: claim: %w", err)
		}
		if removed == 0 {
			continue // a racing worker claimed it first
		}
		m, err := d.load(ctx, id)
		if err != nil {
			return nil, err
		}
		if m == nil { // payload gone: drop the residue, keep scanning
			continue
		}
		if err := d.client.ZAdd(ctx, d.prefix+"reserved", redis.Z{
			Score:  float64(time.Now().Add(m.TTR).UnixMilli()),
			Member: id,
		}).Err(); err != nil {
			return nil, fmt.Errorf("queue: mark %q (id %s) reserved: %w", m.Name, id, err)
		}
		return m, nil
	}
	return nil, nil
}

// Complete removes every trace of the finished message.
func (d *RedisDriver) Complete(ctx context.Context, m *Message) error {
	pipe := d.client.TxPipeline()
	pipe.Del(ctx, d.prefix+msgKey(m.ID))
	pipe.ZRem(ctx, d.prefix+"reserved", m.ID)
	pipe.ZRem(ctx, d.waitingKey(m.Priority), m.ID)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("queue: complete %q (id %d): %w", m.Name, m.ID, err)
	}
	return nil
}

// Requeue returns m to waiting, due after delay, storing the attempts made
// so far.
func (d *RedisDriver) Requeue(ctx context.Context, m *Message, delay time.Duration) error {
	if err := d.store(ctx, m); err != nil {
		return err
	}
	pipe := d.client.TxPipeline()
	pipe.ZAdd(ctx, d.waitingKey(m.Priority), redis.Z{
		Score:  float64(time.Now().Add(delay).UnixMilli()),
		Member: m.ID,
	})
	pipe.ZRem(ctx, d.prefix+"reserved", m.ID)
	pipe.SAdd(ctx, d.prefix+"prios", m.Priority)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("queue: requeue %q (id %d): %w", m.Name, m.ID, err)
	}
	return nil
}

// Fail parks m in the failed set: kept for RetryFailed, invisible to
// Reserve.
func (d *RedisDriver) Fail(ctx context.Context, m *Message) error {
	if err := d.store(ctx, m); err != nil {
		return err
	}
	pipe := d.client.TxPipeline()
	pipe.ZAdd(ctx, d.prefix+"failed", redis.Z{Score: float64(time.Now().UnixMilli()), Member: m.ID})
	pipe.ZRem(ctx, d.prefix+"reserved", m.ID)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("queue: park %q (id %d) as failed: %w", m.Name, m.ID, err)
	}
	return nil
}

// RetryFailed moves every failed message back to waiting with its attempt
// count reset and reports how many moved.
func (d *RedisDriver) RetryFailed(ctx context.Context) (int, error) {
	ids, err := d.client.ZRange(ctx, d.prefix+"failed", 0, -1).Result()
	if err != nil {
		return 0, fmt.Errorf("queue: read failed set: %w", err)
	}
	moved := 0
	for _, id := range ids {
		m, err := d.load(ctx, id)
		if err != nil {
			return moved, err
		}
		if m == nil { // payload gone: drop the residue
			d.client.ZRem(ctx, d.prefix+"failed", id)
			continue
		}
		m.Attempt = 0
		if err := d.store(ctx, m); err != nil {
			return moved, err
		}
		pipe := d.client.TxPipeline()
		pipe.ZAdd(ctx, d.waitingKey(m.Priority), redis.Z{Score: float64(time.Now().UnixMilli()), Member: m.ID})
		pipe.SAdd(ctx, d.prefix+"prios", m.Priority)
		pipe.ZRem(ctx, d.prefix+"failed", m.ID)
		if _, err := pipe.Exec(ctx); err != nil {
			return moved, fmt.Errorf("queue: retry %q (id %s): %w", m.Name, id, err)
		}
		moved++
	}
	return moved, nil
}

// store writes m's current state back to its payload key.
func (d *RedisDriver) store(ctx context.Context, m *Message) error {
	data, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("queue: encode %q (id %d): %w", m.Name, m.ID, err)
	}
	if err := d.client.Set(ctx, d.prefix+msgKey(m.ID), data, 0).Err(); err != nil {
		return fmt.Errorf("queue: store %q (id %d): %w", m.Name, m.ID, err)
	}
	return nil
}

// load reads one message by its id string; nil when the payload is gone.
func (d *RedisDriver) load(ctx context.Context, id string) (*Message, error) {
	data, err := d.client.Get(ctx, d.prefix+msgKey(atoiID(id))).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("queue: read message %s: %w", id, err)
	}
	var m Message
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("queue: decode message %s: %w", id, err)
	}
	return &m, nil
}

// atoiID parses a member id back to int64; a malformed one reads as 0, which
// simply misses every payload key (members always come from this driver's
// own ZADDs, so it cannot happen outside external tampering).
func atoiID(id string) int64 {
	n, _ := strconv.ParseInt(id, 10, 64)
	return n
}
