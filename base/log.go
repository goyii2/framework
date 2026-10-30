package base

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/goyii2/framework/config"
	"github.com/goyii2/framework/log"
)

// Log is the logging component contract of the Application. The default
// implementation is the framework log package's Logger (levels, categories,
// targets, profiling); alternative implementations can be injected with
// Application.SetLog — the full API is reachable through Application.Logger,
// which wraps foreign implementations.
type Log interface {
	Enabled(level slog.Level) bool
	Debug(msg string, args ...any)
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
}

// newDefaultLog builds the default logging component: the framework log
// package driven by the log configuration section. When a configured target
// cannot be built (unknown type, unwritable path), the failure is reported
// on stderr and the component degrades to console-only logging so the
// application still starts.
func newDefaultLog(cfg config.Log) Log {
	l, err := log.New(logConfigOf(cfg))
	if err != nil {
		fmt.Fprintf(os.Stderr, "goyii: %v; falling back to console logging\n", err)
		// The fallback names no targets, so it lands on the console default
		// and cannot fail.
		l, _ = log.New(log.Config{Level: cfg.Level})
	}
	return l
}

// logConfigOf is the transitional struct-equivalence conversion from the
// config package's log section to the log package's parameter bundle; it
// goes away once the application stops carrying a config object.
func logConfigOf(cfg config.Log) log.Config {
	out := log.Config{
		Level: cfg.Level,
		Gorm: log.GormConfig{
			Disable:         cfg.Gorm.Disable,
			SlowThresholdMs: cfg.Gorm.SlowThresholdMs,
		},
	}
	for _, t := range cfg.Targets {
		out.Targets = append(out.Targets, log.TargetConfig{
			Type:       t.Type,
			MinLevel:   t.MinLevel,
			Categories: t.Categories,
			Except:     t.Except,
			Path:       t.Path,
			MaxSizeMB:  t.MaxSizeMB,
			MaxFiles:   t.MaxFiles,
		})
	}
	return out
}

// loggerOf resolves a log component holder to its rich concrete type. The
// framework default (and anything installed as *log.Logger) is returned
// directly; foreign implementations of the minimal Log contract are wrapped
// so callers always receive the full API (categories, targets, profiling),
// with delivery forwarded to the wrapped implementation.
func loggerOf(c *component[Log]) *log.Logger {
	if l, ok := c.get().(*log.Logger); ok {
		return l
	}
	return log.Wrap(c.get())
}
