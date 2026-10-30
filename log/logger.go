package log

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"
)

// DefaultCategory is the category records fall under when none is given,
// mirroring Yii2's "application" default.
const DefaultCategory = "application"

// ProfileCategory is the category profiler records are emitted under.
const ProfileCategory = "profile"

// binding couples one target with the delivery rules the dispatcher applies
// on its behalf.
type binding struct {
	target Target
	filter Filter
}

// Logger is the framework's logging component: leveled and categorized
// records dispatched to one or more targets. It satisfies the minimal log
// contract used across the framework (Enabled/Debug/Info/Warn/Error) and
// adds category scoping, per-target filtering and profiling on top of
// log/slog's level model. A Logger is safe for concurrent use once built;
// WithTarget returns a new Logger instead of mutating.
type Logger struct {
	level    slog.Level
	category string
	bindings []binding

	// forward is set on loggers wrapping a foreign minimal implementation
	// (see Wrap); records are forwarded there instead of dispatching to
	// targets.
	forward LevelLogger
}

// NewWithTargets builds a logger dispatching to already-constructed targets
// with no additional filtering (every target receives every record at or
// above level). It is the programmatic counterpart of New: use it when the
// sinks are built in code — custom targets, test captures — rather than
// named in a Config.
func NewWithTargets(level slog.Level, targets ...Target) *Logger {
	l := &Logger{level: level, category: DefaultCategory}
	for _, t := range targets {
		if t == nil {
			continue
		}
		l.bindings = append(l.bindings, binding{target: t, filter: Filter{MinLevel: slog.LevelDebug}})
	}
	return l
}

// WithLevel returns a copy of the logger dispatching at the given level; the
// receiver is unchanged. The level is the global floor every record must
// pass before target filters apply — the debug panel lowers it to debug so
// query and profile records reach its per-request target without rebuilding
// the configured targets.
func (l *Logger) WithLevel(level slog.Level) *Logger {
	clone := *l
	clone.level = level
	return &clone
}

// WithTarget returns a copy of the logger with one more target bound to the
// given filter; the receiver is unchanged.
func (l *Logger) WithTarget(t Target, f Filter) *Logger {
	if t == nil {
		return l
	}
	clone := *l
	clone.bindings = append(append([]binding(nil), l.bindings...), binding{target: t, filter: f})
	return &clone
}

// ParseLevel maps a configured level name to a slog level; the empty name
// and unknown names log at info level.
func ParseLevel(name string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// Enabled reports whether records at the level would be dispatched.
func (l *Logger) Enabled(level slog.Level) bool {
	return level >= l.level
}

// Debug logs at debug level under the logger's category.
func (l *Logger) Debug(msg string, args ...any) { l.write(slog.LevelDebug, l.category, msg, args) }

// Info logs at info level under the logger's category.
func (l *Logger) Info(msg string, args ...any) { l.write(slog.LevelInfo, l.category, msg, args) }

// Warn logs at warn level under the logger's category.
func (l *Logger) Warn(msg string, args ...any) { l.write(slog.LevelWarn, l.category, msg, args) }

// Error logs at error level under the logger's category.
func (l *Logger) Error(msg string, args ...any) { l.write(slog.LevelError, l.category, msg, args) }

// Log emits one record with an explicit level and category.
func (l *Logger) Log(level slog.Level, category, msg string, args ...any) {
	l.write(level, category, msg, args)
}

// Category returns a logger scoped to the given category: every record it
// emits (including profiles) is stamped with it. The receiver is unchanged;
// the derived logger shares the receiver's targets and level.
func (l *Logger) Category(category string) *Logger {
	if category == "" {
		category = DefaultCategory
	}
	clone := *l
	clone.category = category
	return &clone
}

// BeginProfile starts a profiling interval and immediately emits its begin
// record; the returned Profiler's End emits the matching end record with
// the elapsed time. Categories, levels and targets apply to the pair like
// to any other record.
func (l *Logger) BeginProfile(name string) *Profiler {
	p := &Profiler{logger: l, name: name, begin: time.Now(), armed: l.Enabled(slog.LevelInfo)}
	if p.armed {
		l.write(slog.LevelInfo, ProfileCategory, "profile begin", []any{"token", name})
	}
	return p
}

// Close releases every target's resources.
func (l *Logger) Close() error {
	var errs []string
	for _, b := range l.bindings {
		if err := b.target.Close(); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", b.target.Name(), err))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("log: close targets: %s", strings.Join(errs, "; "))
	}
	return nil
}

// write applies the level floor and hands the record to every target whose
// filter accepts it; wrapped loggers forward instead. A target write error
// is reported on stderr and never fails the caller nor stops the remaining
// targets — logging must not take the application down with it.
func (l *Logger) write(level slog.Level, category, msg string, args []any) {
	if !l.Enabled(level) {
		return
	}
	if l.forward != nil {
		forwardRecord(l.forward, level, msg, args)
		return
	}
	rec := Record{
		Time:     time.Now(),
		Level:    level,
		Category: category,
		Message:  msg,
		Attrs:    argsToAttrs(args),
	}
	for _, b := range l.bindings {
		if !b.filter.Accept(category, level) {
			continue
		}
		if err := b.target.Write(rec); err != nil {
			fmt.Fprintf(os.Stderr, "goyii: log target %s: %v\n", b.target.Name(), err)
		}
	}
}

// argsToAttrs converts key/value argument pairs to slog attributes, keeping
// slog's behavior for malformed input (odd args collapse into !BADKEY).
func argsToAttrs(args []any) []slog.Attr {
	attrs := make([]slog.Attr, 0, len(args)/2)
	for i := 0; i < len(args); i++ {
		switch v := args[i].(type) {
		case slog.Attr:
			attrs = append(attrs, v)
		case string:
			if i+1 < len(args) {
				attrs = append(attrs, slog.Any(v, args[i+1]))
				i++
			} else {
				attrs = append(attrs, slog.Any("!BADKEY", v))
			}
		default:
			attrs = append(attrs, slog.Any("!BADKEY", v))
		}
	}
	return attrs
}

// forwardRecord routes one record to a wrapped minimal logger by level.
func forwardRecord(inner LevelLogger, level slog.Level, msg string, args []any) {
	switch {
	case level < slog.LevelInfo:
		inner.Debug(msg, args...)
	case level < slog.LevelWarn:
		inner.Info(msg, args...)
	case level < slog.LevelError:
		inner.Warn(msg, args...)
	default:
		inner.Error(msg, args...)
	}
}

// LevelLogger is the minimal leveled log contract the framework's
// application uses; it matches the base package's Log interface so any
// implementation satisfies it structurally.
type LevelLogger interface {
	Enabled(level slog.Level) bool
	Debug(msg string, args ...any)
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
}

// Wrap adapts a foreign minimal logger (a test fake, a custom
// implementation installed on the application) to the full Logger API.
// Records are forwarded by level; category, target and filter APIs operate
// on the wrapper but delivery always goes to the wrapped logger.
func Wrap(inner LevelLogger) *Logger {
	return &Logger{
		level:    slog.LevelDebug,
		category: DefaultCategory,
		forward:  inner,
	}
}
