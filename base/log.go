package base

import (
	"context"
	"log/slog"
	"os"
	"strings"

	"github.com/goyii2/framework/config"
)

// Log is the logging component contract of the Application. The default
// implementation writes log/slog records to stdout at the configured level;
// richer implementations can be injected with Application.SetLog.
type Log interface {
	Enabled(level slog.Level) bool
	Debug(msg string, args ...any)
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
}

// defaultLog adapts a *slog.Logger with a stdout text handler to the Log
// contract.
type defaultLog struct {
	logger *slog.Logger
}

func (l *defaultLog) Enabled(level slog.Level) bool {
	return l.logger.Enabled(context.Background(), level)
}

func (l *defaultLog) Debug(msg string, args ...any) { l.logger.Debug(msg, args...) }
func (l *defaultLog) Info(msg string, args ...any)  { l.logger.Info(msg, args...) }
func (l *defaultLog) Warn(msg string, args ...any)  { l.logger.Warn(msg, args...) }
func (l *defaultLog) Error(msg string, args ...any) { l.logger.Error(msg, args...) }

// newDefaultLog builds the default logging component for the given settings.
func newDefaultLog(cfg config.Log) *defaultLog {
	return &defaultLog{logger: slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: parseLevel(cfg.Level),
	}))}
}

// parseLevel maps a configured level name to a slog level; unknown names log
// at info level.
func parseLevel(name string) slog.Level {
	switch strings.ToLower(name) {
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
