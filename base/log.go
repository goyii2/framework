package base

import (
	"log/slog"

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

// Log returns the logging component.
func (a *Application) Log() Log { return a.log }

// Logger returns the logging component under its full API (category
// scoping, target management, profiling). It is the concrete default
// implementation; a foreign component installed via SetLog is wrapped, with
// delivery forwarded to it.
func (a *Application) Logger() *log.Logger { return loggerOf(a.log) }

// SetLog replaces the logging component (e.g. a test fake or the logger
// built from the project's configuration); nil restores the console
// fallback.
func (a *Application) SetLog(l Log) {
	if l == nil {
		l = newDefaultLog()
	}
	a.log = l
}

// newDefaultLog builds the fallback logging component: console logging at
// info level. The zero-value log.Config selects exactly that, so this
// constructor cannot fail; a failure here is a framework bug and panics.
func newDefaultLog() Log {
	l, err := log.New(log.Config{})
	if err != nil {
		panic("goyii: building the default log component: " + err.Error())
	}
	return l
}

// loggerOf resolves a log component to its rich concrete type. The framework
// default (and anything installed as *log.Logger) is returned directly;
// foreign implementations of the minimal Log contract are wrapped so callers
// always receive the full API (categories, targets, profiling), with
// delivery forwarded to the wrapped implementation.
func loggerOf(l Log) *log.Logger {
	if lv, ok := l.(*log.Logger); ok {
		return lv
	}
	return log.Wrap(l)
}
