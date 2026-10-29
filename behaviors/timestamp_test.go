package behaviors

import (
	"context"
	"testing"
	"time"

	"github.com/goyii2/framework/db/ar"
)

func TestTimestampFillsBothColumnsOnInsert(t *testing.T) {
	ctx := newTestContext(t)
	before := time.Now().Add(-time.Second)

	doc := ar.New[tsDoc](ctx).New()
	doc.Title = "first"
	if err := doc.Save(); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if doc.CreatedAt.Before(before) {
		t.Errorf("CreatedAt = %v, want at least %v", doc.CreatedAt, before)
	}
	if doc.UpdatedAt.Before(before) {
		t.Errorf("UpdatedAt = %v, want at least %v", doc.UpdatedAt, before)
	}

	loaded, err := ar.FindByID[tsDoc](ctx, doc.ID)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !loaded.CreatedAt.Equal(doc.CreatedAt) || !loaded.UpdatedAt.Equal(doc.UpdatedAt) {
		t.Errorf("round-trip: loaded (%v, %v), saved (%v, %v)",
			loaded.CreatedAt, loaded.UpdatedAt, doc.CreatedAt, doc.UpdatedAt)
	}
}

func TestTimestampRefreshesUpdatedOnlyOnUpdate(t *testing.T) {
	ctx := newTestContext(t)

	doc := ar.New[tsDoc](ctx).New()
	doc.Title = "first"
	if err := doc.Save(); err != nil {
		t.Fatalf("insert: %v", err)
	}
	created := doc.CreatedAt

	time.Sleep(60 * time.Millisecond) // the column keeps sub-second precision; cross enough of it to differ
	doc.Title = "renamed"
	if err := doc.Save(); err != nil {
		t.Fatalf("update: %v", err)
	}
	if !doc.CreatedAt.Equal(created) {
		t.Errorf("CreatedAt changed on update: %v, want %v", doc.CreatedAt, created)
	}
	if !doc.UpdatedAt.After(created) {
		t.Errorf("UpdatedAt = %v, want after %v", doc.UpdatedAt, created)
	}

	loaded, err := ar.FindByID[tsDoc](ctx, doc.ID)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !loaded.CreatedAt.Equal(created) || !loaded.UpdatedAt.Equal(doc.UpdatedAt) {
		t.Errorf("round-trip: loaded (%v, %v), in-memory (%v, %v)",
			loaded.CreatedAt, loaded.UpdatedAt, created, doc.UpdatedAt)
	}
}

func TestTimestampUnixStoresSeconds(t *testing.T) {
	ctx := newTestContext(t)
	before := time.Now().Add(-2 * time.Second).Unix()

	doc := ar.New[unixDoc](ctx).New()
	doc.Title = "seconds"
	if err := doc.Save(); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if doc.CreatedAt < before || doc.UpdatedAt < before {
		t.Errorf("stamps = (%d, %d), want at least %d", doc.CreatedAt, doc.UpdatedAt, before)
	}

	time.Sleep(1100 * time.Millisecond)
	doc.Title = "resaved"
	if err := doc.Save(); err != nil {
		t.Fatalf("update: %v", err)
	}
	if doc.UpdatedAt <= doc.CreatedAt {
		t.Errorf("UpdatedAt = %d, want after CreatedAt %d", doc.UpdatedAt, doc.CreatedAt)
	}

	loaded, err := ar.FindByID[unixDoc](ctx, doc.ID)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.CreatedAt != doc.CreatedAt || loaded.UpdatedAt != doc.UpdatedAt {
		t.Errorf("round-trip: loaded (%d, %d), in-memory (%d, %d)",
			loaded.CreatedAt, loaded.UpdatedAt, doc.CreatedAt, doc.UpdatedAt)
	}
}

// TestTimestampInTransactionUsesTransactionContext proves the fills keep
// working inside a transaction and the committed row carries them.
func TestTimestampInTransactionUsesTransactionContext(t *testing.T) {
	ctx := newTestContext(t)

	err := ar.Transaction(ctx, func(tctx context.Context) error {
		doc := ar.New[tsDoc](tctx).New()
		doc.Title = "tx"
		return doc.Save()
	})
	if err != nil {
		t.Fatalf("transaction: %v", err)
	}
	docs, err := ar.New[tsDoc](ctx).All()
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(docs) != 1 || docs[0].CreatedAt.IsZero() || docs[0].UpdatedAt.IsZero() {
		t.Fatalf("committed row = %+v, want timestamps filled", docs)
	}
}
