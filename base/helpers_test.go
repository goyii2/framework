package base

import (
	"log/slog"
	"os"
	"sync"

	"github.com/goyii2/framework/log"
)

// dbGormOff is the disabled GORM adapter configuration for tests that only
// need a connection, not its query logging.
func dbGormOff() log.GormConfig { return log.GormConfig{Disable: true} }

// countingLog is a concurrency-safe Log fake that records calls.
type countingLog struct {
	mu    sync.Mutex
	calls []string
}

func (l *countingLog) Enabled(level slog.Level) bool { return true }

func (l *countingLog) record(level, msg string) {
	l.mu.Lock()
	l.calls = append(l.calls, level+":"+msg)
	l.mu.Unlock()
}

func (l *countingLog) Debug(msg string, args ...any) { l.record("debug", msg) }
func (l *countingLog) Info(msg string, args ...any)  { l.record("info", msg) }
func (l *countingLog) Warn(msg string, args ...any)  { l.record("warn", msg) }
func (l *countingLog) Error(msg string, args ...any) { l.record("error", msg) }

func (l *countingLog) snapshot() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.calls...)
}

// readFile is os.ReadFile for tests.
func readFile(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
