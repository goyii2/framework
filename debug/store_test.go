package debug

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// testPanel builds a panel storing into a temp directory.
func testPanel(t *testing.T, keep int) *Panel {
	t.Helper()
	return &Panel{Dir: t.TempDir(), KeepFiles: keep}
}

func TestStoreWritesEntryWithID(t *testing.T) {
	p := testPanel(t, 10)
	e := &Entry{Time: time.Date(2026, 10, 29, 12, 30, 5, 123456789, time.UTC)}
	if err := p.store(e); err != nil {
		t.Fatalf("store: %v", err)
	}
	if e.ID == "" {
		t.Fatal("store left the entry's id empty")
	}
	data, err := os.ReadFile(filepath.Join(p.Dir, e.ID+fileSuffix))
	if err != nil {
		t.Fatalf("record file: %v", err)
	}
	if !strings.Contains(string(data), `"id": "`+e.ID+`"`) {
		t.Errorf("record JSON does not carry the id:\n%s", data)
	}
}

func TestStoreEvictsOldestBeyondKeep(t *testing.T) {
	p := testPanel(t, 3)
	base := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	var ids []string
	for i := range 5 {
		e := &Entry{Time: base.Add(time.Duration(i) * time.Second)}
		if err := p.store(e); err != nil {
			t.Fatalf("store %d: %v", i, err)
		}
		ids = append(ids, e.ID)
	}
	names, err := listEntries(p.Dir)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(names) != 3 {
		t.Fatalf("%d files retained, want 3: %v", len(names), names)
	}
	for _, id := range ids[:2] {
		if _, err := os.Stat(filepath.Join(p.Dir, id+fileSuffix)); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("oldest record %s still on disk (err=%v)", id, err)
		}
	}
	for _, id := range ids[2:] {
		if _, err := os.Stat(filepath.Join(p.Dir, id+fileSuffix)); err != nil {
			t.Errorf("newest record %s missing: %v", id, err)
		}
	}
}

func TestLoadRejectsUnsafeAndUnknownIDs(t *testing.T) {
	p := testPanel(t, 10)
	e := &Entry{Time: time.Now()}
	if err := p.store(e); err != nil {
		t.Fatalf("store: %v", err)
	}
	for _, id := range []string{"", "../escape", "a/b", "a\\b", "space id", "shapeok-but-missing"} {
		if _, err := p.load(id); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("load(%q) err = %v, want ErrNotExist", id, err)
		}
	}
	loaded, err := p.load(e.ID)
	if err != nil {
		t.Fatalf("load stored id: %v", err)
	}
	if loaded.ID != e.ID {
		t.Errorf("loaded id = %q, want %q", loaded.ID, e.ID)
	}
}

func TestHistoryNewestFirst(t *testing.T) {
	p := testPanel(t, 10)
	base := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	for i := range 3 {
		if err := p.store(&Entry{Time: base.Add(time.Duration(i) * time.Second)}); err != nil {
			t.Fatalf("store: %v", err)
		}
	}
	entries, err := p.history(10)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("history has %d entries, want 3", len(entries))
	}
	if !entries[0].Time.After(entries[2].Time) {
		t.Errorf("history not newest first: %v then %v", entries[0].Time, entries[2].Time)
	}
}

func TestHistoryEmptyDirectory(t *testing.T) {
	p := testPanel(t, 10)
	entries, err := p.history(10)
	if err != nil {
		t.Fatalf("history on empty dir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("history on empty dir returned %d entries", len(entries))
	}
}
