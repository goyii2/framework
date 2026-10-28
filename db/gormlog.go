package db

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/goyii2/framework/log"
)

// GormCategory is the log category database records are emitted under.
const GormCategory = "db"

// GormLogger adapts the framework log component to GORM's logger contract.
// Query traces arrive per executed statement: errors log at error level,
// queries at or above the slow threshold log at warn level, everything else
// (including record-not-found, which is a routine miss rather than a fault)
// logs at debug level. The logger is resolved lazily on every record so the
// application's log component stays lazily initialized.
type GormLogger struct {
	get  func() *log.Logger
	slow time.Duration
}

// NewGormLogger builds the adapter over a logger supplier; slowThreshold
// <= 0 disables slow-query detection.
func NewGormLogger(get func() *log.Logger, slowThreshold time.Duration) gormlogger.Interface {
	return &GormLogger{get: get, slow: slowThreshold}
}

// LogMode keeps the adapter's behavior fixed: filtering happens through the
// framework log levels, so GORM's own level knob is ignored.
func (g *GormLogger) LogMode(gormlogger.LogLevel) gormlogger.Interface { return g }

// Info logs a GORM informational message at info level.
func (g *GormLogger) Info(_ context.Context, msg string, args ...any) {
	g.get().Log(slog.LevelInfo, GormCategory, fmt.Sprintf(msg, args...))
}

// Warn logs a GORM warning at warn level.
func (g *GormLogger) Warn(_ context.Context, msg string, args ...any) {
	g.get().Log(slog.LevelWarn, GormCategory, fmt.Sprintf(msg, args...))
}

// Error logs a GORM error at error level.
func (g *GormLogger) Error(_ context.Context, msg string, args ...any) {
	g.get().Log(slog.LevelError, GormCategory, fmt.Sprintf(msg, args...))
}

// Trace classifies one executed statement by its outcome and logs it at the
// matching level; the SQL text is only rendered when a record will actually
// be emitted, keeping the interpolation cost off filtered-out queries.
func (g *GormLogger) Trace(_ context.Context, begin time.Time, fc func() (string, int64), err error) {
	elapsed := time.Since(begin)
	l := g.get()

	switch {
	case err != nil && !errors.Is(err, gorm.ErrRecordNotFound):
		if l.Enabled(slog.LevelError) {
			sql, rows := fc()
			l.Log(slog.LevelError, GormCategory, "query failed",
				"elapsed_ms", elapsedMs(elapsed), "rows", rows, "sql", sql, "err", err.Error())
		}
	case g.slow > 0 && elapsed >= g.slow:
		if l.Enabled(slog.LevelWarn) {
			sql, rows := fc()
			l.Log(slog.LevelWarn, GormCategory, "slow query",
				"elapsed_ms", elapsedMs(elapsed), "threshold_ms", elapsedMs(g.slow), "rows", rows, "sql", sql)
		}
	default:
		if l.Enabled(slog.LevelDebug) {
			sql, rows := fc()
			msg := "query"
			if errors.Is(err, gorm.ErrRecordNotFound) {
				msg = "query missed (record not found)"
			}
			l.Log(slog.LevelDebug, GormCategory, msg,
				"elapsed_ms", elapsedMs(elapsed), "rows", rows, "sql", sql)
		}
	}
}

// elapsedMs renders a duration as fractional milliseconds.
func elapsedMs(d time.Duration) float64 {
	return float64(d.Nanoseconds()) / 1e6
}
