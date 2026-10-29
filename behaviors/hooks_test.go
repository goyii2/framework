package behaviors

import (
	"testing"
	"time"

	"github.com/goyii2/framework/db/ar"
	"gorm.io/gorm"
)

// dispatchBase nests behaviors one embedding level down: the walk must
// find them through the anonymous base field.
type dispatchBase struct {
	Timestamp
	Blameable
}

type nestedDoc struct {
	ar.Model
	Hooks
	dispatchBase
	ID int64 `gorm:"column:id;primaryKey"`
}

func (nestedDoc) TableName() string { return "nested_docs" }

func TestDispatchReachesBehaviorsNestedInABase(t *testing.T) {
	ctx := asUser(newTestContext(t), 5)
	if err := exec(ctx, "CREATE TABLE nested_docs (id INTEGER PRIMARY KEY AUTOINCREMENT, created_at DATETIME, updated_at DATETIME, created_by INTEGER NOT NULL DEFAULT 0, updated_by INTEGER NOT NULL DEFAULT 0)"); err != nil {
		t.Fatalf("create table: %v", err)
	}

	doc := ar.New[nestedDoc](ctx).New()
	if err := doc.Save(); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if doc.CreatedAt.IsZero() || doc.UpdatedAt.IsZero() {
		t.Errorf("timestamps = (%v, %v), want filled through the nested base", doc.CreatedAt, doc.UpdatedAt)
	}
	if doc.CreatedBy != 5 || doc.UpdatedBy != 5 {
		t.Errorf("stamps = (%d, %d), want (5, 5)", doc.CreatedBy, doc.UpdatedBy)
	}
}

// customDoc shadows every hook with its own, delegating to the dispatch
// entry points — the escape hatch for models that need extra lifecycle
// logic on top of their behaviors.
type customDoc struct {
	ar.Model
	Hooks
	Timestamp
	OptimisticLock
	ID      int64  `gorm:"column:id;primaryKey"`
	Title   string `gorm:"column:title"`
	created bool
}

func (customDoc) TableName() string { return "custom_docs" }

func (c *customDoc) BeforeCreate(tx *gorm.DB) error {
	if err := DispatchCreate(c, tx); err != nil {
		return err
	}
	c.created = true
	return nil
}

func (c *customDoc) BeforeUpdate(tx *gorm.DB) error {
	return DispatchUpdate(c, tx)
}

func (c *customDoc) AfterUpdate(tx *gorm.DB) error {
	return DispatchUpdated(c, tx)
}

func TestCustomHooksDelegateToDispatch(t *testing.T) {
	ctx := newTestContext(t)
	if err := exec(ctx, "CREATE TABLE custom_docs (id INTEGER PRIMARY KEY AUTOINCREMENT, title TEXT NOT NULL, created_at DATETIME, updated_at DATETIME, version INTEGER NOT NULL)"); err != nil {
		t.Fatalf("create table: %v", err)
	}

	doc := ar.New[customDoc](ctx).New()
	doc.Title = "custom"
	if err := doc.Save(); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if !doc.created {
		t.Error("the model's own BeforeCreate did not run")
	}
	if doc.CreatedAt.IsZero() || doc.Version != 1 {
		t.Errorf("fills through delegation: created %v, version %d; want filled and 1", doc.CreatedAt, doc.Version)
	}

	doc.Title = "resaved"
	if err := doc.Save(); err != nil {
		t.Fatalf("update: %v", err)
	}
	if doc.Version != 2 {
		t.Errorf("version = %d, want 2 through the delegated update hooks", doc.Version)
	}

	// The conflict path works through delegation too: a stale copy must
	// refuse with the sentinel error.
	stale, err := ar.FindByID[customDoc](ctx, doc.ID)
	if err != nil {
		t.Fatalf("load stale: %v", err)
	}
	if err := doc.Save(); err != nil {
		t.Fatalf("winner save: %v", err)
	}
	stale.Title = "too late"
	if err := stale.Save(); err == nil {
		t.Error("stale save through custom hooks returned nil, want conflict")
	}
}

// TestSluggableStaysSilentWithoutHooks proves the wiring contract: a
// behavior without the Hooks embed next to it does nothing. (Timestamp is
// a poor witness here — its column names carry GORM's own auto-fill — so
// the slug is the witness.)
func TestSluggableStaysSilentWithoutHooks(t *testing.T) {
	ctx := newTestContext(t)
	if err := exec(ctx, "CREATE TABLE bare_docs (id INTEGER PRIMARY KEY AUTOINCREMENT, title TEXT NOT NULL, slug TEXT)"); err != nil {
		t.Fatalf("create table: %v", err)
	}

	doc := ar.New[bareDoc](ctx).New()
	doc.Title = "should not slug"
	if err := doc.Save(); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if doc.Slug != "" {
		t.Errorf("slug = %q, want empty: without Hooks the behavior stays off", doc.Slug)
	}
}

type bareDoc struct {
	ar.Model
	Sluggable
	ID    int64  `gorm:"column:id;primaryKey"`
	Title string `gorm:"column:title"`
}

func (bareDoc) TableName() string { return "bare_docs" }

func (bareDoc) SlugOptions() SlugOptions { return SlugOptions{Source: "Title"} }

func TestDispatchRejectsUnaddressableModel(t *testing.T) {
	err := DispatchCreate(customDoc{Title: "by value"}, nil)
	if err == nil {
		t.Fatal("DispatchCreate on a struct value returned nil, want the pointer error")
	}
}

// TestAllBehaviorsComposeOnOneModel is the composition proof: one record,
// four behaviors, one save and one update, everything filled and stepping
// together.
func TestAllBehaviorsComposeOnOneModel(t *testing.T) {
	ctx := asUser(newTestContext(t), 9)

	doc := ar.New[allDoc](ctx).New()
	doc.Title = "All Together Now"
	if err := doc.Save(); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if doc.Slug != "all-together-now" {
		t.Errorf("slug = %q, want all-together-now", doc.Slug)
	}
	if doc.Version != 1 {
		t.Errorf("version = %d, want 1", doc.Version)
	}
	if doc.CreatedBy != 9 || doc.UpdatedBy != 9 {
		t.Errorf("stamps = (%d, %d), want (9, 9)", doc.CreatedBy, doc.UpdatedBy)
	}
	created := doc.CreatedAt

	time.Sleep(20 * time.Millisecond)
	other := asUser(ctx, 4)
	loaded, err := ar.FindByID[allDoc](other, doc.ID)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	loaded.Title = "Renamed, Slug Stays"
	if err := loaded.Save(); err != nil {
		t.Fatalf("update: %v", err)
	}
	if loaded.Slug != "all-together-now" {
		t.Errorf("slug after rename = %q, want unchanged", loaded.Slug)
	}
	if loaded.Version != 2 {
		t.Errorf("version = %d, want 2", loaded.Version)
	}
	if loaded.CreatedBy != 9 || loaded.UpdatedBy != 4 {
		t.Errorf("stamps = (%d, %d), want (9, 4)", loaded.CreatedBy, loaded.UpdatedBy)
	}
	if !loaded.CreatedAt.Equal(created) || !loaded.UpdatedAt.After(created) {
		t.Errorf("timestamps = (%v, %v), want created kept and updated after", loaded.CreatedAt, loaded.UpdatedAt)
	}
}
