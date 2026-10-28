package log

import (
	"log/slog"
	"time"
)

// Profiler is one profiling interval opened by Logger.BeginProfile: the
// begin record was emitted on open, and End emits the matching end record
// carrying the elapsed time. End is idempotent, so a deferred call after an
// explicit one is safe; a profiler whose begin was filtered out by the
// logger level stays silent.
type Profiler struct {
	logger *Logger
	name   string
	begin  time.Time
	armed  bool
	done   bool
}

// End closes the profiling interval and logs the end record with the
// elapsed milliseconds.
func (p *Profiler) End() {
	if p == nil || p.done || !p.armed {
		return
	}
	p.done = true
	elapsed := time.Since(p.begin)
	p.logger.write(slog.LevelInfo, ProfileCategory, "profile end", []any{
		"token", p.name,
		"elapsed_ms", float64(elapsed.Nanoseconds()) / 1e6,
	})
}
