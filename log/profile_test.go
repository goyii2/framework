package log

import (
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func findRecord(recs []Record, message string) (Record, bool) {
	for _, rec := range recs {
		if rec.Message == message {
			return rec, true
		}
	}
	return Record{}, false
}

func attrValue(rec Record, key string) (any, bool) {
	for _, attr := range rec.Attrs {
		if attr.Key == key {
			return attr.Value.Any(), true
		}
	}
	return nil, false
}

func TestProfileEmitsPairedRecords(t *testing.T) {
	mem := &memTarget{}
	l := NewWithTargets(slog.LevelDebug, mem)

	p := l.BeginProfile("render posts")
	p.End()

	begin, ok := findRecord(mem.snapshot(), "profile begin")
	if !ok {
		t.Fatal("profile begin record missing")
	}
	if begin.Category != ProfileCategory || begin.Level != slog.LevelInfo {
		t.Errorf("begin record = category %q level %v, want %q info", begin.Category, begin.Level, ProfileCategory)
	}
	if token, _ := attrValue(begin, "token"); token != "render posts" {
		t.Errorf("begin token attr = %v, want %q", token, "render posts")
	}

	end, ok := findRecord(mem.snapshot(), "profile end")
	if !ok {
		t.Fatal("profile end record missing")
	}
	if token, _ := attrValue(end, "token"); token != "render posts" {
		t.Errorf("end token attr = %v, want %q (records must pair by token)", token, "render posts")
	}
	elapsed, ok := attrValue(end, "elapsed_ms")
	if !ok {
		t.Fatal("profile end record must carry the elapsed time")
	}
	ms, isNumber := elapsed.(float64)
	if !isNumber || ms < 0 {
		t.Errorf("elapsed_ms = %v, want a non-negative number", elapsed)
	}
}

func TestProfileRespectsCategoryFilters(t *testing.T) {
	profilesOnly := &memTarget{}
	l := NewWithTargets(slog.LevelDebug).WithTarget(profilesOnly, Filter{Includes: []string{ProfileCategory}})

	l.Info("noise")
	p := l.BeginProfile("kept")
	p.End()

	recs := profilesOnly.snapshot()
	if len(recs) != 2 {
		t.Fatalf("category filtering must isolate the profile pair, got %d records", len(recs))
	}
}

func TestProfileSilentWhenLevelFiltersInfo(t *testing.T) {
	mem := &memTarget{}
	l := NewWithTargets(slog.LevelError, mem)

	p := l.BeginProfile("dropped")
	p.End()

	if got := len(mem.snapshot()); got != 0 {
		t.Errorf("profile pair below the level floor must not be emitted, got %d records", got)
	}
}

func TestProfileEndIsIdempotent(t *testing.T) {
	mem := &memTarget{}
	l := NewWithTargets(slog.LevelDebug, mem)

	p := l.BeginProfile("once")
	p.End()
	p.End()

	ends := 0
	for _, rec := range mem.snapshot() {
		if rec.Message == "profile end" {
			ends++
		}
	}
	if ends != 1 {
		t.Errorf("End called twice emitted %d end records, want 1", ends)
	}
}

// TestProfileConcurrentTiming opens and closes profiling intervals from
// many goroutines against a shared file target; every begin must have its
// matching end and all lines must be intact. Run with -race.
func TestProfileConcurrentTiming(t *testing.T) {
	target, err := NewFileTarget(filepath.Join(t.TempDir(), "profile.log"), 1<<30, 5)
	if err != nil {
		t.Fatalf("NewFileTarget: %v", err)
	}
	t.Cleanup(func() { _ = target.Close() })
	l := NewWithTargets(slog.LevelDebug, target)

	const goroutines, intervals = 32, 8
	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < intervals; i++ {
				p := l.BeginProfile(strings.Join([]string{"block", string(rune('a' + g%26)), string(rune('0' + i))}, "-"))
				p.End()
			}
		}(g)
	}
	wg.Wait()

	data := readLogFile(t, target)
	lines := strings.Split(strings.TrimRight(data, "\n"), "\n")
	if len(lines) != goroutines*intervals*2 {
		t.Fatalf("got %d lines, want %d (pairs incomplete)", len(lines), goroutines*intervals*2)
	}
	begins, ends := 0, 0
	for _, line := range lines {
		switch {
		case strings.Contains(line, "profile begin"):
			begins++
		case strings.Contains(line, "profile end"):
			ends++
			if !strings.Contains(line, "elapsed_ms=") {
				t.Errorf("end record without elapsed time: %q", line)
			}
		default:
			t.Errorf("unexpected line in profile log: %q", line)
		}
	}
	if begins != ends {
		t.Errorf("begins = %d, ends = %d; every interval must produce a pair", begins, ends)
	}
}
