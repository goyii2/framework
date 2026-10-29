package behaviors

import (
	"errors"
	"testing"

	"github.com/goyii2/framework/db/ar"
	"gorm.io/gorm"
)

func TestOptimisticLockStartsAtOneOnInsert(t *testing.T) {
	ctx := newTestContext(t)

	doc := ar.New[lockedDoc](ctx).New()
	doc.Payload = "v1"
	if err := doc.Save(); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if doc.Version != 1 {
		t.Errorf("version = %d, want 1", doc.Version)
	}
}

func TestOptimisticLockIncrementsOnEachSave(t *testing.T) {
	ctx := newTestContext(t)

	doc := ar.New[lockedDoc](ctx).New()
	doc.Payload = "v1"
	if err := doc.Save(); err != nil {
		t.Fatalf("insert: %v", err)
	}
	doc.Payload = "v2"
	if err := doc.Save(); err != nil {
		t.Fatalf("second save: %v", err)
	}
	if doc.Version != 2 {
		t.Errorf("version = %d, want 2", doc.Version)
	}
	loaded, err := ar.FindByID[lockedDoc](ctx, doc.ID)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.Version != 2 {
		t.Errorf("stored version = %d, want 2", loaded.Version)
	}
}

func TestOptimisticLockRejectsConflictingSave(t *testing.T) {
	ctx := newTestContext(t)

	seed := ar.New[lockedDoc](ctx).New()
	seed.Payload = "v1"
	if err := seed.Save(); err != nil {
		t.Fatalf("insert: %v", err)
	}

	a, err := ar.FindByID[lockedDoc](ctx, seed.ID)
	if err != nil {
		t.Fatalf("load a: %v", err)
	}
	b, err := ar.FindByID[lockedDoc](ctx, seed.ID)
	if err != nil {
		t.Fatalf("load b: %v", err)
	}

	a.Payload = "written by a"
	if err := a.Save(); err != nil {
		t.Fatalf("first writer: %v", err)
	}

	b.Payload = "written by b"
	err = b.Save()
	if !errors.Is(err, ErrStaleVersion) {
		t.Fatalf("conflicting save error = %v, want ErrStaleVersion", err)
	}

	// The loser keeps the version it was loaded with, ready for a
	// reload-and-retry cycle.
	if b.Version != 1 {
		t.Errorf("loser version = %d, want 1 restored", b.Version)
	}
	if b.Payload != "written by b" {
		t.Errorf("loser payload = %q, want the in-memory edit kept", b.Payload)
	}

	// The winner's write is what the row carries; the loser's never
	// landed.
	loaded, err := ar.FindByID[lockedDoc](ctx, seed.ID)
	if err != nil {
		t.Fatalf("load final: %v", err)
	}
	if loaded.Payload != "written by a" || loaded.Version != 2 {
		t.Errorf("row = (%q, version %d), want (written by a, 2)", loaded.Payload, loaded.Version)
	}

	// After reloading, the retry succeeds.
	if err := b.Reload(); err != nil {
		t.Fatalf("reload: %v", err)
	}
	b.Payload = "written by b after retry"
	if err := b.Save(); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if b.Version != 3 {
		t.Errorf("retry version = %d, want 3", b.Version)
	}
}

func TestOptimisticLockConflictSurvivesErrorWrapping(t *testing.T) {
	ctx := newTestContext(t)

	seed := ar.New[lockedDoc](ctx).New()
	seed.Payload = "v1"
	if err := seed.Save(); err != nil {
		t.Fatalf("insert: %v", err)
	}

	a, err := ar.FindByID[lockedDoc](ctx, seed.ID)
	if err != nil {
		t.Fatalf("load a: %v", err)
	}
	b, err := ar.FindByID[lockedDoc](ctx, seed.ID)
	if err != nil {
		t.Fatalf("load b: %v", err)
	}
	if err := a.Save(); err != nil {
		t.Fatalf("first writer: %v", err)
	}

	// ar.Save wraps the hook error with its own context; the sentinel
	// must still be detectable through the wrapping.
	err = b.Save()
	if !errors.Is(err, ErrStaleVersion) {
		t.Fatalf("wrapped error = %v, want errors.Is ErrStaleVersion", err)
	}
	// And it is not, say, the record-not-found sentinel.
	if errors.Is(err, gorm.ErrRecordNotFound) {
		t.Errorf("conflict error = %v, must not be ErrRecordNotFound", err)
	}
}

func TestOptimisticLockDeletedRowReadsAsConflict(t *testing.T) {
	ctx := newTestContext(t)

	doc := ar.New[lockedDoc](ctx).New()
	doc.Payload = "v1"
	if err := doc.Save(); err != nil {
		t.Fatalf("insert: %v", err)
	}

	loaded, err := ar.FindByID[lockedDoc](ctx, doc.ID)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if err := doc.Delete(); err != nil {
		t.Fatalf("delete original: %v", err)
	}

	loaded.Payload = "update of a ghost"
	err = loaded.Save()
	if !errors.Is(err, ErrStaleVersion) {
		t.Fatalf("update of deleted row = %v, want ErrStaleVersion (the expected version is gone either way)", err)
	}
}
