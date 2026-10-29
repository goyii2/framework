package queue

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// DefaultTableName is the db driver's table when the configuration names
// none.
const DefaultTableName = "queue"

// Row statuses of the db-backed queue: waiting is runnable (when due),
// reserved is claimed by a worker, failed is parked after its attempts were
// exhausted.
const (
	statusWaiting  = 0
	statusReserved = 1
	statusFailed   = 2
)

// DBDriver is the relational driver: one table (created by NewMigration)
// holds every message with its state. All times are Unix nanoseconds in
// BIGINT columns; durations too (the TTR and the retry schedule), so the same
// schema serves every supported database without dialect-specific types.
//
// Reservations are claimed atomically with a single guarded UPDATE on the
// most due waiting row: workers racing for the same row — in one process or
// several — see exactly one winner, without table locks or SELECT FOR UPDATE
// (which SQLite cannot offer). Expired reservations are reclaimed to waiting
// at the head of every Reserve, so a crashed worker cannot lose a message.
type DBDriver struct {
	gorm  *gorm.DB
	table string
}

// NewDB creates the driver over the GORM handle. table is the queue table
// ("queue" when empty); the caller is responsible for having applied
// NewMigration(table) — the driver never issues DDL itself.
func NewDB(gormDB *gorm.DB, table string) *DBDriver {
	if table == "" {
		table = DefaultTableName
	}
	return &DBDriver{gorm: gormDB, table: table}
}

// Table returns the queue table name the driver works on.
func (d *DBDriver) Table() string { return d.table }

// queueRow is the GORM row shape of the queue table. Times are Unix
// nanoseconds; ReservedAt 0 and Claim "" mean "not held".
type queueRow struct {
	ID          int64  `gorm:"column:id;primaryKey;autoIncrement"`
	Name        string `gorm:"column:name"`
	Payload     []byte `gorm:"column:payload"`
	Status      int    `gorm:"column:status"`
	Priority    int    `gorm:"column:priority"`
	Attempts    int    `gorm:"column:attempts"`
	Attempt     int    `gorm:"column:attempt"`
	TTR         int64  `gorm:"column:ttr"`
	DueAt       int64  `gorm:"column:due_at"`
	ReservedAt  int64  `gorm:"column:reserved_at"`
	Claim       string `gorm:"column:claim"`
	PushedAt    int64  `gorm:"column:pushed_at"`
	RetryDelays []byte `gorm:"column:retry_delays"`
}

// row builds the storage row for m.
func (d *DBDriver) row(m *Message) (*queueRow, error) {
	delays, err := json.Marshal(m.RetryDelays)
	if err != nil {
		return nil, fmt.Errorf("queue: encode retry delays of %q: %w", m.Name, err)
	}
	return &queueRow{
		Name:        m.Name,
		Payload:     m.Payload,
		Status:      statusWaiting,
		Priority:    m.Priority,
		Attempts:    m.Attempts,
		Attempt:     m.Attempt,
		TTR:         int64(m.TTR),
		DueAt:       m.DueAt.UnixNano(),
		ReservedAt:  0,
		Claim:       "",
		PushedAt:    m.PushedAt.UnixNano(),
		RetryDelays: delays,
	}, nil
}

// message restores the queued message from the row.
func (r *queueRow) message() *Message {
	var delays []time.Duration
	if len(r.RetryDelays) > 0 {
		_ = json.Unmarshal(r.RetryDelays, &delays) // written by row(); treat damage as no schedule
	}
	return &Message{
		ID:          r.ID,
		Name:        r.Name,
		Payload:     r.Payload,
		Priority:    r.Priority,
		Attempts:    r.Attempts,
		Attempt:     r.Attempt,
		TTR:         time.Duration(r.TTR),
		RetryDelays: delays,
		DueAt:       time.Unix(0, r.DueAt),
		PushedAt:    time.Unix(0, r.PushedAt),
	}
}

// Push inserts m as a waiting row and assigns m.ID.
func (d *DBDriver) Push(ctx context.Context, m *Message) error {
	r, err := d.row(m)
	if err != nil {
		return err
	}
	if err := d.gorm.WithContext(ctx).Table(d.table).Create(r).Error; err != nil {
		return fmt.Errorf("queue: insert %q: %w", m.Name, err)
	}
	m.ID = r.ID
	return nil
}

// Reserve reclaims expired reservations, then atomically claims the next due
// waiting message (highest priority, then earliest due, then lowest id). It
// returns nil when nothing is due.
func (d *DBDriver) Reserve(ctx context.Context) (*Message, error) {
	now := time.Now().UnixNano()

	// Reclaim: reserved rows whose TTR elapsed return to waiting, due at
	// once. The attempt count survives — only executions increment it — so a
	// message wedged in crash loops stays due but never silently loses its
	// retry budget.
	if err := d.gorm.WithContext(ctx).Table(d.table).
		Where("status = ? AND reserved_at + ttr <= ?", statusReserved, now).
		Updates(map[string]any{
			"status":      statusWaiting,
			"reserved_at": 0,
			"claim":       "",
			"due_at":      now,
		}).Error; err != nil {
		return nil, fmt.Errorf("queue: reclaim expired reservations: %w", err)
	}

	// Claim: one guarded UPDATE over the most due waiting row. The subquery
	// is wrapped in a derived table because MySQL forbids selecting from the
	// table being updated; the wrap is harmless elsewhere.
	claim, err := newClaim()
	if err != nil {
		return nil, err
	}
	inner := d.gorm.WithContext(ctx).Table(d.table).
		Select("id").
		Where("status = ? AND due_at <= ?", statusWaiting, now).
		Order("priority DESC, due_at ASC, id ASC").
		Limit(1)
	res := d.gorm.WithContext(ctx).Table(d.table).
		Where("id = (?) AND status = ?", d.gorm.Table("(?) as due", inner).Select("id"), statusWaiting).
		Updates(map[string]any{
			"status":      statusReserved,
			"reserved_at": now,
			"claim":       claim,
		})
	if res.Error != nil {
		return nil, fmt.Errorf("queue: claim: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return nil, nil
	}

	var r queueRow
	if err := d.gorm.WithContext(ctx).Table(d.table).Where("claim = ?", claim).Take(&r).Error; err != nil {
		return nil, fmt.Errorf("queue: read claimed row: %w", err)
	}
	return r.message(), nil
}

// Complete deletes the finished row.
func (d *DBDriver) Complete(ctx context.Context, m *Message) error {
	if err := d.gorm.WithContext(ctx).Table(d.table).Where("id = ?", m.ID).
		Delete(&queueRow{}).Error; err != nil {
		return fmt.Errorf("queue: delete %q (id %d): %w", m.Name, m.ID, err)
	}
	return nil
}

// Requeue returns m to waiting, due after delay, persisting the attempts
// made so far.
func (d *DBDriver) Requeue(ctx context.Context, m *Message, delay time.Duration) error {
	if err := d.gorm.WithContext(ctx).Table(d.table).
		Where("id = ?", m.ID).
		Updates(map[string]any{
			"status":      statusWaiting,
			"due_at":      time.Now().Add(delay).UnixNano(),
			"reserved_at": 0,
			"claim":       "",
			"attempt":     m.Attempt,
		}).Error; err != nil {
		return fmt.Errorf("queue: requeue %q (id %d): %w", m.Name, m.ID, err)
	}
	return nil
}

// Fail parks m as failed: kept for RetryFailed, invisible to Reserve.
func (d *DBDriver) Fail(ctx context.Context, m *Message) error {
	if err := d.gorm.WithContext(ctx).Table(d.table).
		Where("id = ?", m.ID).
		Updates(map[string]any{
			"status":      statusFailed,
			"reserved_at": 0,
			"claim":       "",
			"attempt":     m.Attempt,
			"due_at":      time.Now().UnixNano(),
		}).Error; err != nil {
		return fmt.Errorf("queue: park %q (id %d) as failed: %w", m.Name, m.ID, err)
	}
	return nil
}

// RetryFailed moves every failed row back to waiting with its attempt count
// reset and reports how many moved.
func (d *DBDriver) RetryFailed(ctx context.Context) (int, error) {
	res := d.gorm.WithContext(ctx).Table(d.table).
		Where("status = ?", statusFailed).
		Updates(map[string]any{
			"status":      statusWaiting,
			"due_at":      time.Now().UnixNano(),
			"reserved_at": 0,
			"claim":       "",
			"attempt":     0,
		})
	if res.Error != nil {
		return 0, fmt.Errorf("queue: retry failed rows: %w", res.Error)
	}
	return int(res.RowsAffected), nil
}

// newClaim generates the single-reservation claim token: crypto-random per
// Reserve, so concurrent workers claiming and reading back never cross.
func newClaim() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("queue: generate claim token: %w", err)
	}
	return hex.EncodeToString(b), nil
}
