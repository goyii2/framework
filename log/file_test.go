package log

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestFileTargetAppendsToExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "append.log")

	if err := os.WriteFile(path, []byte("pre-existing line\n"), 0o644); err != nil {
		t.Fatalf("seeding file: %v", err)
	}

	target, err := NewFileTarget(path, 1<<20, 5)
	if err != nil {
		t.Fatalf("NewFileTarget: %v", err)
	}
	t.Cleanup(func() { _ = target.Close() })

	for i := 0; i < 2; i++ {
		if err := target.Write(Record{Time: time.Now(), Level: slog.LevelInfo, Category: "app", Message: fmt.Sprintf("line %d", i)}); err != nil {
			t.Fatalf("Write: %v", err)
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading log file: %v", err)
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("file must keep prior content and append, got %d lines: %q", len(lines), data)
	}
	if !strings.HasPrefix(lines[0], "pre-existing") {
		t.Errorf("first line = %q, want the pre-existing content preserved", lines[0])
	}
	if !strings.Contains(lines[1], "line 0") || !strings.Contains(lines[2], "line 1") {
		t.Errorf("appended lines out of order: %q", lines[1:])
	}
}

func TestFileTargetRotatesBySizeAndKeepsBackups(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rotate.log")

	// Each formatted line is ~90 bytes; a 100-byte limit forces a rotation
	// on every second write.
	target, err := NewFileTarget(path, 100, 2)
	if err != nil {
		t.Fatalf("NewFileTarget: %v", err)
	}
	t.Cleanup(func() { _ = target.Close() })

	for i := 0; i < 6; i++ {
		if err := target.Write(Record{Time: time.Now(), Level: slog.LevelInfo, Category: "app", Message: fmt.Sprintf("entry-%02d padding padding padding", i)}); err != nil {
			t.Fatalf("Write %d: %v", i, err)
		}
	}

	// Retention limit of 2: path, path.1 and path.2 exist, path.3 must not.
	for _, name := range []string{"rotate.log", "rotate.log.1", "rotate.log.2"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("expected %s to exist: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "rotate.log.3")); !os.IsNotExist(err) {
		t.Errorf("backups beyond the retention limit must be dropped, stat err = %v", err)
	}

	// Backup chain holds the oldest first; the current file holds the last
	// entry only.
	assertFileContains(t, filepath.Join(dir, "rotate.log"), "entry-05", "current file must hold the newest entry")
	assertFileNotContains(t, filepath.Join(dir, "rotate.log"), "entry-04", "previous entry must have been rotated away")
	assertFileContains(t, filepath.Join(dir, "rotate.log.1"), "entry-04", "newest backup holds the previous segment")
	assertFileContains(t, filepath.Join(dir, "rotate.log.2"), "entry-03", "oldest backup holds the earliest surviving segment")
	assertFileNotContains(t, filepath.Join(dir, "rotate.log.2"), "entry-00", "segments beyond retention must be gone")
}

func TestFileTargetUnlimitedRetention(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "keep.log")

	target, err := NewFileTarget(path, 100, 0)
	if err != nil {
		t.Fatalf("NewFileTarget: %v", err)
	}
	t.Cleanup(func() { _ = target.Close() })

	for i := 0; i < 4; i++ {
		if err := target.Write(Record{Time: time.Now(), Level: slog.LevelInfo, Category: "app", Message: fmt.Sprintf("entry-%02d padding padding padding", i)}); err != nil {
			t.Fatalf("Write %d: %v", i, err)
		}
	}
	for n := 1; n <= 3; n++ {
		if _, err := os.Stat(backupPath(path, n)); err != nil {
			t.Errorf("unlimited retention must keep backup %d: %v", n, err)
		}
	}
}

func TestFileTargetRequiresPath(t *testing.T) {
	if _, err := NewFileTarget("", 100, 2); err == nil {
		t.Fatal("an empty path must be rejected")
	}
}

func TestFileTargetCreatesParentDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "deeper", "out.log")
	target, err := NewFileTarget(path, 1<<20, 2)
	if err != nil {
		t.Fatalf("NewFileTarget: %v", err)
	}
	t.Cleanup(func() { _ = target.Close() })

	if err := target.Write(Record{Time: time.Now(), Level: slog.LevelInfo, Message: "made it"}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	assertFileContains(t, path, "made it", "missing parent directories must be created")
}

// TestFileTargetConcurrentWrites hammers one file target from many
// goroutines; every line must land intact (no interleaving), none may be
// lost. Run with -race.
func TestFileTargetConcurrentWrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "concurrent.log")
	target, err := NewFileTarget(path, 1<<30, 5)
	if err != nil {
		t.Fatalf("NewFileTarget: %v", err)
	}
	t.Cleanup(func() { _ = target.Close() })

	const goroutines, perGoroutine = 16, 50
	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < perGoroutine; i++ {
				target.Write(Record{
					Time:    time.Now(),
					Level:   slog.LevelInfo,
					Message: fmt.Sprintf("g%02d-i%02d", g, i),
				})
			}
		}(g)
	}
	wg.Wait()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading log file: %v", err)
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) != goroutines*perGoroutine {
		t.Fatalf("got %d lines, want %d (writes were lost)", len(lines), goroutines*perGoroutine)
	}
	for i, line := range lines {
		if !strings.Contains(line, " g") || !strings.Contains(line, "INFO") {
			t.Errorf("line %d mangled by interleaving: %q", i, line)
		}
	}
}

// readLogFile returns the current content of a file target's log file.
func readLogFile(t *testing.T, target *FileTarget) string {
	t.Helper()
	data, err := os.ReadFile(target.path)
	if err != nil {
		t.Fatalf("reading log file %s: %v", target.path, err)
	}
	return string(data)
}

func assertFileContains(t *testing.T, path, want, context string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%s: reading %s: %v", context, path, err)
	}
	if !strings.Contains(string(data), want) {
		t.Errorf("%s: %s content = %q, want it to contain %q", context, path, data, want)
	}
}

func assertFileNotContains(t *testing.T, path, want, context string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%s: reading %s: %v", context, path, err)
	}
	if strings.Contains(string(data), want) {
		t.Errorf("%s: %s content = %q, want it NOT to contain %q", context, path, data, want)
	}
}
