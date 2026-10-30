package db

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/goyii2/framework/log"
)

// captureTarget records everything the adapter logs.
type captureTarget struct {
	mu   sync.Mutex
	recs []log.Record
}

func (c *captureTarget) Name() string { return "capture" }
func (c *captureTarget) Write(rec log.Record) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.recs = append(c.recs, rec)
	return nil
}
func (c *captureTarget) Close() error { return nil }

func (c *captureTarget) drain() []log.Record {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]log.Record, len(c.recs))
	copy(out, c.recs)
	c.recs = nil
	return out
}

// attrOf returns the value of one attribute on a record.
func attrOf(rec log.Record, key string) (any, bool) {
	for _, attr := range rec.Attrs {
		if attr.Key == key {
			return attr.Value.Any(), true
		}
	}
	return nil, false
}

// newAdaptedLogger builds a GormLogger over a logger with a capture target
// and returns both.
func newAdaptedLogger(t *testing.T, level slog.Level, slow time.Duration) (*GormLogger, *captureTarget) {
	t.Helper()
	capt := &captureTarget{}
	l := log.NewWithTargets(level, capt)
	return &GormLogger{get: func() *log.Logger { return l }, slow: slow}, capt
}

func traceFunc(sql string) func() (string, int64) {
	return func() (string, int64) { return sql, 3 }
}

func TestGormLoggerTraceNormalQuery(t *testing.T) {
	adapter, capt := newAdaptedLogger(t, slog.LevelDebug, 50*time.Millisecond)

	adapter.Trace(context.Background(), time.Now(), traceFunc("SELECT * FROM posts"), nil)

	recs := capt.drain()
	if len(recs) != 1 {
		t.Fatalf("got %d records, want 1", len(recs))
	}
	rec := recs[0]
	if rec.Level != slog.LevelDebug || rec.Category != GormCategory {
		t.Errorf("record = level %v category %q, want debug / %q", rec.Level, rec.Category, GormCategory)
	}
	if rec.Message != "query" {
		t.Errorf("message = %q, want %q", rec.Message, "query")
	}
	if sql, _ := attrOf(rec, "sql"); sql != "SELECT * FROM posts" {
		t.Errorf("sql attr = %v", sql)
	}
}

func TestGormLoggerTraceSlowQuery(t *testing.T) {
	adapter, capt := newAdaptedLogger(t, slog.LevelDebug, 50*time.Millisecond)

	begin := time.Now().Add(-200 * time.Millisecond)
	adapter.Trace(context.Background(), begin, traceFunc("SELECT SLEEP(1)"), nil)

	recs := capt.drain()
	if len(recs) != 1 || recs[0].Level != slog.LevelWarn {
		t.Fatalf("slow query must log at warn level, got %+v", recs)
	}
	if recs[0].Message != "slow query" {
		t.Errorf("message = %q, want %q", recs[0].Message, "slow query")
	}
	if _, ok := attrOf(recs[0], "threshold_ms"); !ok {
		t.Error("slow query record must carry the configured threshold")
	}
}

func TestGormLoggerTraceError(t *testing.T) {
	adapter, capt := newAdaptedLogger(t, slog.LevelError, 0)

	adapter.Trace(context.Background(), time.Now(), traceFunc("INSERT INTO posts"), errors.New("syntax error"))

	recs := capt.drain()
	if len(recs) != 1 || recs[0].Level != slog.LevelError {
		t.Fatalf("query failure must log at error level, got %+v", recs)
	}
	if recs[0].Message != "query failed" {
		t.Errorf("message = %q, want %q", recs[0].Message, "query failed")
	}
	if errText, _ := attrOf(recs[0], "err"); errText != "syntax error" {
		t.Errorf("err attr = %v", errText)
	}
}

func TestGormLoggerTraceRecordNotFoundIsRoutine(t *testing.T) {
	adapter, capt := newAdaptedLogger(t, slog.LevelDebug, 0)

	adapter.Trace(context.Background(), time.Now(), traceFunc("SELECT 1"), gorm.ErrRecordNotFound)

	recs := capt.drain()
	if len(recs) != 1 || recs[0].Level != slog.LevelDebug {
		t.Fatalf("record-not-found is a routine miss, want a debug record, got %+v", recs)
	}
	if recs[0].Message != "query missed (record not found)" {
		t.Errorf("message = %q", recs[0].Message)
	}
}

func TestGormLoggerTraceSkipsSQLWhenFilteredOut(t *testing.T) {
	adapter, capt := newAdaptedLogger(t, slog.LevelWarn, 0)

	rendered := false
	adapter.Trace(context.Background(), time.Now(), func() (string, int64) {
		rendered = true
		return "SELECT 1", 0
	}, nil)

	if rendered {
		t.Error("SQL must not be rendered when the record is filtered out by the level")
	}
	if recs := capt.drain(); len(recs) != 0 {
		t.Errorf("got %d records, want none below the level floor", len(recs))
	}
}

func TestGormLoggerInfoWarnErrorMessages(t *testing.T) {
	adapter, capt := newAdaptedLogger(t, slog.LevelDebug, 0)
	ctx := context.Background()

	adapter.Info(ctx, "connected to %s", "db1")
	adapter.Warn(ctx, "retrying")
	adapter.Error(ctx, "boom")

	recs := capt.drain()
	if len(recs) != 3 {
		t.Fatalf("got %d records, want 3", len(recs))
	}
	wantLevels := []slog.Level{slog.LevelInfo, slog.LevelWarn, slog.LevelError}
	wantMsgs := []string{"connected to db1", "retrying", "boom"}
	for i, rec := range recs {
		if rec.Level != wantLevels[i] || rec.Message != wantMsgs[i] || rec.Category != GormCategory {
			t.Errorf("record %d = %+v, want level %v message %q", i, rec, wantLevels[i], wantMsgs[i])
		}
	}
}

func TestGormLoggerLogModeReturnsSelf(t *testing.T) {
	adapter, capt := newAdaptedLogger(t, slog.LevelDebug, 0)
	if returned := adapter.LogMode(0); returned != adapter {
		t.Error("LogMode must return the adapter itself (fixed behavior)")
	}
	if recs := capt.drain(); len(recs) != 0 {
		t.Errorf("LogMode must not log, got %d records", len(recs))
	}
}
