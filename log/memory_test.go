package log

import (
	"log/slog"
	"testing"
	"time"
)

func TestMemoryTargetStoresInOrder(t *testing.T) {
	target := NewMemoryTarget(10)
	now := time.Now()
	for i, msg := range []string{"first", "second", "third"} {
		if err := target.Write(Record{Time: now.Add(time.Duration(i) * time.Second), Level: slog.LevelInfo, Message: msg}); err != nil {
			t.Fatalf("write %q: %v", msg, err)
		}
	}
	records := target.Records()
	if len(records) != 3 {
		t.Fatalf("stored %d records, want 3", len(records))
	}
	for i, want := range []string{"first", "second", "third"} {
		if records[i].Message != want {
			t.Errorf("record %d = %q, want %q", i, records[i].Message, want)
		}
	}
	if target.Len() != 3 {
		t.Errorf("Len() = %d, want 3", target.Len())
	}
}

func TestMemoryTargetEvictsOldestOverCapacity(t *testing.T) {
	target := NewMemoryTarget(3)
	for i := range 5 {
		_ = target.Write(Record{Message: string(rune('a' + i))})
	}
	records := target.Records()
	if len(records) != 3 {
		t.Fatalf("stored %d records, want 3", len(records))
	}
	for i, want := range []string{"c", "d", "e"} {
		if records[i].Message != want {
			t.Errorf("record %d = %q, want %q", i, records[i].Message, want)
		}
	}
}

func TestMemoryTargetRecordsAreCopies(t *testing.T) {
	target := NewMemoryTarget(10)
	_ = target.Write(Record{Message: "original"})
	records := target.Records()
	records[0].Message = "mutated"
	if got := target.Records()[0].Message; got != "original" {
		t.Errorf("mutating the returned slice changed the target: %q", got)
	}
}

func TestWithLevelLowersDispatchFloor(t *testing.T) {
	target := NewMemoryTarget(10)
	// A logger at info level drops debug records before any target runs;
	// WithLevel returns a copy dispatching at the new level.
	l := NewWithTargets(slog.LevelInfo, target)
	l.Debug("dropped")
	if target.Len() != 0 {
		t.Fatalf("info-level logger dispatched a debug record: %d", target.Len())
	}
	lowed := l.WithLevel(slog.LevelDebug)
	lowed.Debug("kept")
	if target.Len() != 1 {
		t.Fatalf("lowered logger did not dispatch the debug record: %d", target.Len())
	}
	if got := target.Records()[0].Message; got != "kept" {
		t.Errorf("dispatched %q, want %q", got, "kept")
	}
	// The receiver keeps its own level.
	l.Debug("still dropped")
	if target.Len() != 1 {
		t.Fatalf("receiver logger changed level: %d records", target.Len())
	}
}
