package log

import (
	"sync"
)

// MemoryTarget keeps the most recent records in memory: a bounded ring of
// records that callers read back with Records. It is the sink per-request
// collectors attach (via Logger.WithTarget) to observe the records one
// request emitted — the debug panel's log capture is the built-in consumer,
// and tests use it to assert on dispatched records without a file or
// console. Safe for concurrent use.
type MemoryTarget struct {
	mu      sync.Mutex
	records []Record
	cap     int
}

// NewMemoryTarget builds a memory target keeping at most capacity records;
// capacity <= 0 means 1000. Once full, the oldest record is dropped on every
// write.
func NewMemoryTarget(capacity int) *MemoryTarget {
	if capacity <= 0 {
		capacity = 1000
	}
	return &MemoryTarget{cap: capacity}
}

// Name identifies the target in diagnostics.
func (t *MemoryTarget) Name() string { return "memory" }

// Write appends the record, evicting the oldest when the capacity is
// reached.
func (t *MemoryTarget) Write(rec Record) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.records) == t.cap {
		copy(t.records, t.records[1:])
		t.records[len(t.records)-1] = rec
		return nil
	}
	t.records = append(t.records, rec)
	return nil
}

// Records returns a copy of the stored records in dispatch order; mutating
// the result affects no target state.
func (t *MemoryTarget) Records() []Record {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]Record, len(t.records))
	copy(out, t.records)
	return out
}

// Len returns the number of stored records.
func (t *MemoryTarget) Len() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.records)
}

// Close releases nothing; a memory target holds no resources.
func (t *MemoryTarget) Close() error { return nil }
