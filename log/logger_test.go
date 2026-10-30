package log

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// memTarget is a custom in-memory Target used across the tests to observe
// exactly what the dispatcher delivers — the same injection path a
// framework user takes for their own sinks.
type memTarget struct {
	mu      sync.Mutex
	records []Record
}

func (m *memTarget) Name() string { return "mem" }
func (m *memTarget) Write(rec Record) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.records = append(m.records, rec)
	return nil
}
func (m *memTarget) Close() error { return nil }

func (m *memTarget) snapshot() []Record {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]Record(nil), m.records...)
}

func (m *memTarget) find(message string) (Record, bool) {
	for _, rec := range m.snapshot() {
		if rec.Message == message {
			return rec, true
		}
	}
	return Record{}, false
}

func TestLoggerRespectsLevelFloor(t *testing.T) {
	mem := &memTarget{}
	l := NewWithTargets(slog.LevelWarn, mem)

	l.Debug("dbg")
	l.Info("inf")
	if got := len(mem.snapshot()); got != 0 {
		t.Fatalf("records below the warn floor leaked: %d", got)
	}

	l.Warn("warned")
	l.Error("failed")
	if got := len(mem.snapshot()); got != 2 {
		t.Fatalf("records at or above the floor must dispatch, got %d", got)
	}

	if l.Enabled(slog.LevelInfo) {
		t.Error("Enabled(info) must be false with a warn floor")
	}
	if !l.Enabled(slog.LevelError) {
		t.Error("Enabled(error) must be true with a warn floor")
	}
}

func TestLoggerDefaultCategory(t *testing.T) {
	mem := &memTarget{}
	l := NewWithTargets(slog.LevelDebug, mem)

	l.Info("hello", "key", "value")

	rec, ok := mem.find("hello")
	if !ok {
		t.Fatal("record not delivered")
	}
	if rec.Category != DefaultCategory {
		t.Errorf("Category = %q, want the default %q", rec.Category, DefaultCategory)
	}
	if len(rec.Attrs) != 1 || rec.Attrs[0].Key != "key" || rec.Attrs[0].Value.Any() != "value" {
		t.Errorf("Attrs = %v, want [key=value]", rec.Attrs)
	}
}

func TestLoggerCategoryScope(t *testing.T) {
	mem := &memTarget{}
	l := NewWithTargets(slog.LevelDebug, mem)

	db := l.Category("db")
	db.Info("connecting")
	l.Info("unrelated")

	dbRec, ok := mem.find("connecting")
	if !ok || dbRec.Category != "db" {
		t.Fatalf("scoped logger must stamp its category, got %+v", dbRec)
	}
	appRec, ok := mem.find("unrelated")
	if !ok || appRec.Category != DefaultCategory {
		t.Fatalf("receiver logger must keep its own category, got %+v", appRec)
	}

	// The derived logger shares the receiver's targets and level.
	if got := len(mem.snapshot()); got != 2 {
		t.Errorf("both records must land in the shared target, got %d", got)
	}

	if empty := l.Category(""); empty.category != DefaultCategory {
		t.Error("Category(\"\") must normalize to the default category")
	}
}

func TestLoggerExplicitLevelAndCategory(t *testing.T) {
	mem := &memTarget{}
	l := NewWithTargets(slog.LevelDebug, mem)

	l.Log(slog.LevelError, "web/session", "destroy failed", "id", "s1")

	rec, ok := mem.find("destroy failed")
	if !ok {
		t.Fatal("record not delivered")
	}
	if rec.Level != slog.LevelError || rec.Category != "web/session" {
		t.Errorf("record = level %v category %q, want error / web/session", rec.Level, rec.Category)
	}
}

func TestLoggerOddArgsCollectedAsBadKey(t *testing.T) {
	mem := &memTarget{}
	l := NewWithTargets(slog.LevelDebug, mem)

	l.Info("odd", "key", "value", 42)

	rec, _ := mem.find("odd")
	if len(rec.Attrs) != 2 {
		t.Fatalf("Attrs = %v, want the pair plus one !BADKEY", rec.Attrs)
	}
	if rec.Attrs[1].Key != "!BADKEY" {
		t.Errorf("odd trailing arg key = %q, want !BADKEY", rec.Attrs[1].Key)
	}
}

func TestLoggerCustomTargetInjectionWithFilter(t *testing.T) {
	all := &memTarget{}
	dbOnly := &memTarget{}

	base := NewWithTargets(slog.LevelDebug, all)
	l := base.WithTarget(dbOnly, Filter{Includes: []string{"db*"}})

	l.Category("db.query").Info("from db")
	l.Category("web").Info("from web")

	if got := len(all.snapshot()); got != 2 {
		t.Errorf("unfiltered target must see both records, got %d", got)
	}
	dbRecs := dbOnly.snapshot()
	if len(dbRecs) != 1 || dbRecs[0].Category != "db.query" {
		t.Errorf("filtered target must see only db records, got %+v", dbRecs)
	}

	// WithTarget must leave the receiver untouched.
	if len(base.bindings) != 1 || len(l.bindings) != 2 {
		t.Errorf("WithTarget must not mutate the receiver: base has %d bindings, derived %d", len(base.bindings), len(l.bindings))
	}
}

func TestLoggerDeliversToConsoleAndFileTargets(t *testing.T) {
	var buf bytes.Buffer
	console := NewConsoleTarget(&buf)

	path := filepath.Join(t.TempDir(), "both.log")
	file, err := NewFileTarget(path, 1<<20, 5)
	if err != nil {
		t.Fatalf("NewFileTarget: %v", err)
	}
	t.Cleanup(func() { _ = file.Close() })

	mem := &memTarget{}
	l := NewWithTargets(slog.LevelDebug, console, file, mem)

	l.Category("web").Warn("multi-target", "n", 1)

	want := "WARN [web] multi-target n=1"
	if line := buf.String(); !strings.Contains(line, want) {
		t.Errorf("console output = %q, want it to contain %q", line, want)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading log file: %v", err)
	}
	if line := string(data); !strings.Contains(line, want) {
		t.Errorf("file output = %q, want it to contain %q", line, want)
	}
	if _, ok := mem.find("multi-target"); !ok {
		t.Error("memory target did not receive the record")
	}
}

func TestLoggerTargetWriteErrorIsContained(t *testing.T) {
	all := &memTarget{}
	l := NewWithTargets(slog.LevelDebug, &failTarget{}, all)

	l.Info("still delivered")

	if _, ok := all.find("still delivered"); !ok {
		t.Error("a failing target must not stop delivery to the remaining ones")
	}
}

type failTarget struct{}

func (failTarget) Name() string       { return "fail" }
func (failTarget) Write(Record) error { return os.ErrClosed }
func (failTarget) Close() error       { return nil }

func TestLoggerCloseClosesTargets(t *testing.T) {
	file, err := NewFileTarget(filepath.Join(t.TempDir(), "close.log"), 1<<20, 5)
	if err != nil {
		t.Fatalf("NewFileTarget: %v", err)
	}
	l := NewWithTargets(slog.LevelDebug, file)

	if err := l.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := file.Write(Record{Message: "after close"}); err == nil {
		t.Error("writing a closed target must fail")
	}
}

func TestNewBuildsFilteredTargets(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cfg.log")
	cfg := Config{
		Level: "debug",
		Targets: []TargetConfig{
			{Type: "console", MinLevel: "warn", Categories: []string{"web*"}, Except: []string{"web/noisy"}},
			{Type: "file", Path: path, MinLevel: "debug"},
		},
	}
	l, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() })

	if !l.Enabled(slog.LevelDebug) {
		t.Error("configured level debug must enable debug records")
	}

	// The file target has no category rules, so everything lands there.
	l.Category("web").Info("kept by file")
	l.Category("web/noisy").Info("excluded on console only")

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading log file: %v", err)
	}
	if n := strings.Count(string(data), "\n"); n != 2 {
		t.Errorf("file target must receive both records, got %d lines", n)
	}
}

// TestNewZeroConfigIsConsoleInfo verifies the zero Config constructs a
// working logger: info level, one console target on stdout.
func TestNewZeroConfigIsConsoleInfo(t *testing.T) {
	l, err := New(Config{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() })

	if l.Enabled(slog.LevelDebug) || !l.Enabled(slog.LevelInfo) {
		t.Error("the zero config must log at info level")
	}
	if len(l.bindings) != 1 || l.bindings[0].target.Name() != "console" {
		t.Errorf("the zero config must install one console target, got %+v", l.bindings)
	}
}

func TestNewDefaultsToConsole(t *testing.T) {
	l, err := New(Config{Level: "warn"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() })

	if !l.Enabled(slog.LevelWarn) || l.Enabled(slog.LevelInfo) {
		t.Error("level parsing from the config string is broken")
	}
	if len(l.bindings) != 1 || l.bindings[0].target.Name() != "console" {
		t.Errorf("with no targets configured a console target must be installed, got %+v", l.bindings)
	}
}

func TestNewRejectsUnknownTargetType(t *testing.T) {
	_, err := New(Config{Targets: []TargetConfig{{Type: "syslog"}}})
	if err == nil || !strings.Contains(err.Error(), "unknown") {
		t.Fatalf("unknown target type must error, got %v", err)
	}
}

func TestNewRejectsUnwritableFile(t *testing.T) {
	// A directory can never be opened as an append-mode log file.
	_, err := New(Config{Targets: []TargetConfig{{Type: "file", Path: t.TempDir()}}})
	if err == nil {
		t.Fatal("an unwritable file target must error")
	}
}

func TestWrapForwardsByLevel(t *testing.T) {
	inner := &countingLogger{}
	l := Wrap(inner)

	l.Debug("d", "k", 1)
	l.Info("i")
	l.Warn("w")
	l.Error("e")
	l.Category("db").Info("scoped")

	got := inner.snapshot()
	want := []string{"debug:d", "info:i", "warn:w", "error:e", "info:scoped"}
	if len(got) != len(want) {
		t.Fatalf("forwarded calls = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("call %d = %q, want %q", i, got[i], want[i])
		}
	}
}

// countingLogger is a minimal fake recording level:message for each call.
type countingLogger struct {
	mu    sync.Mutex
	calls []string
}

func (c *countingLogger) Enabled(slog.Level) bool    { return true }
func (c *countingLogger) Debug(msg string, _ ...any) { c.record("debug:" + msg) }
func (c *countingLogger) Info(msg string, _ ...any)  { c.record("info:" + msg) }
func (c *countingLogger) Warn(msg string, _ ...any)  { c.record("warn:" + msg) }
func (c *countingLogger) Error(msg string, _ ...any) { c.record("error:" + msg) }

func (c *countingLogger) record(call string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls = append(c.calls, call)
}

func (c *countingLogger) snapshot() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.calls...)
}
