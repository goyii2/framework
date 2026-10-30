package behaviors

import (
	"context"
	"strings"
	"testing"

	"github.com/goyii2/framework/base"
	"github.com/goyii2/framework/db/ar"
)

func TestSluggableDerivesFromSourceOnInsert(t *testing.T) {
	ctx := newTestContext(t)

	doc := ar.New[article](ctx).New()
	doc.Title = "Hello, Big World!"
	if err := doc.Save(); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if doc.Slug != "hello-big-world" {
		t.Errorf("slug = %q, want %q", doc.Slug, "hello-big-world")
	}
}

func TestSluggableAppendsCounterOnCollision(t *testing.T) {
	ctx := newTestContext(t)

	want := []string{"dup", "dup-2", "dup-3", "dup-4"}
	for i, title := range []string{"dup", "DUP", "dup!", "   dup   "} {
		doc := ar.New[article](ctx).New()
		doc.Title = title
		if err := doc.Save(); err != nil {
			t.Fatalf("insert %d: %v", i, err)
		}
		if doc.Slug != want[i] {
			t.Errorf("insert %d (%q): slug = %q, want %q", i, title, doc.Slug, want[i])
		}
	}
}

func TestSluggableRespectsExplicitSlugOnInsert(t *testing.T) {
	ctx := newTestContext(t)

	first := ar.New[article](ctx).New()
	first.Title = "ignored source"
	first.Slug = "hand-picked"
	if err := first.Save(); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if first.Slug != "hand-picked" {
		t.Errorf("slug = %q, want the explicit hand-picked", first.Slug)
	}

	// The second row colliding with the explicit slug gets a suffix.
	second := ar.New[article](ctx).New()
	second.Title = "Hand Picked"
	if err := second.Save(); err != nil {
		t.Fatalf("insert colliding: %v", err)
	}
	if second.Slug != "hand-picked-2" {
		t.Errorf("colliding slug = %q, want hand-picked-2", second.Slug)
	}
}

func TestSluggableKeepsExistingSlugOnUpdate(t *testing.T) {
	ctx := newTestContext(t)

	doc := ar.New[article](ctx).New()
	doc.Title = "original title"
	if err := doc.Save(); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if doc.Slug != "original-title" {
		t.Fatalf("slug = %q, want original-title", doc.Slug)
	}

	loaded, err := ar.FindByID[article](ctx, doc.ID)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	loaded.Title = "renamed beyond recognition"
	if err := loaded.Save(); err != nil {
		t.Fatalf("update: %v", err)
	}
	if loaded.Slug != "original-title" {
		t.Errorf("slug after rename = %q, want original-title (URLs must keep resolving)", loaded.Slug)
	}
}

func TestSluggableFillsEmptySlugOnUpdate(t *testing.T) {
	ctx := newTestContext(t)

	doc := ar.New[article](ctx).New()
	doc.Title = "first title"
	if err := doc.Save(); err != nil {
		t.Fatalf("insert: %v", err)
	}

	// Wipe the slug directly in the database; the next save backfills it.
	if err := exec(ctx, "UPDATE articles SET slug = '' WHERE id = ?", doc.ID); err != nil {
		t.Fatalf("wipe slug: %v", err)
	}
	loaded, err := ar.FindByID[article](ctx, doc.ID)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.Slug != "" {
		t.Fatalf("precondition: slug = %q, want empty", loaded.Slug)
	}
	loaded.Title = "second title"
	if err := loaded.Save(); err != nil {
		t.Fatalf("update: %v", err)
	}
	if loaded.Slug != "second-title" {
		t.Errorf("backfilled slug = %q, want second-title", loaded.Slug)
	}
}

func TestSluggableCustomOptions(t *testing.T) {
	ctx := newTestContext(t)

	first := ar.New[shoutArticle](ctx).New()
	first.Title = "Keep MY Case!"
	if err := first.Save(); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if first.Slug != "Keep_MY_Case" {
		t.Errorf("slug = %q, want Keep_MY_Case", first.Slug)
	}

	second := ar.New[shoutArticle](ctx).New()
	second.Title = "keep my case"
	if err := second.Save(); err != nil {
		t.Fatalf("insert colliding: %v", err)
	}
	if second.Slug != "keep_my_case" {
		t.Errorf("colliding slug = %q, want keep_my_case (case-sensitive probe)", second.Slug)
	}
}

func TestSluggableNonASCIIDroppedByDefault(t *testing.T) {
	ctx := newTestContext(t)

	doc := ar.New[article](ctx).New()
	doc.Title = "中文标题 pure"
	if err := doc.Save(); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if doc.Slug != "pure" {
		t.Errorf("slug = %q, want pure (non-ASCII dropped, no transliteration)", doc.Slug)
	}

	// A source reduced to nothing by the filter leaves the slug empty
	// rather than failing the save.
	only := ar.New[article](ctx).New()
	only.Title = "仅中文"
	if err := only.Save(); err != nil {
		t.Fatalf("insert non-ASCII-only: %v", err)
	}
	if only.Slug != "" {
		t.Errorf("slug = %q, want empty", only.Slug)
	}
}

func TestSluggableKeepUnicodeCarriesOriginalText(t *testing.T) {
	ctx := newTestContext(t)

	doc := ar.New[unicodeArticle](ctx).New()
	doc.Title = "中文 标题 42"
	if err := doc.Save(); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if doc.Slug != "中文-标题-42" {
		t.Errorf("slug = %q, want 中文-标题-42", doc.Slug)
	}

	other := ar.New[unicodeArticle](ctx).New()
	other.Title = "中文 标题 42"
	if err := other.Save(); err != nil {
		t.Fatalf("insert colliding: %v", err)
	}
	if other.Slug != "中文-标题-42-2" {
		t.Errorf("colliding slug = %q, want 中文-标题-42-2", other.Slug)
	}
}

func TestSluggableWithoutOptionsRefusesSave(t *testing.T) {
	ctx := newTestContext(t)

	// A model embedding Sluggable without the SlugOptions method must
	// fail loudly instead of silently skipping the fill.
	doc := ar.New[optionless](ctx).New()
	doc.Title = "anything"
	err := doc.Save()
	if err == nil {
		t.Fatal("Save without SlugOptions returned nil error")
	}
	if want := "SlugOptions"; !strings.Contains(err.Error(), want) {
		t.Errorf("error = %v, want it to mention %q", err, want)
	}
}

func TestSluggableUniqueIndexCatchesWhatTheProbeMisses(t *testing.T) {
	ctx := newTestContext(t)

	// Two same-title rows inside one transaction: the probe must see the
	// first, still-uncommitted row and hand the second a suffix.
	err := ar.Transaction(ctx, func(tctx context.Context) error {
		first := ar.New[article](tctx).New()
		first.Title = "tx clash"
		if err := first.Save(); err != nil {
			return err
		}
		second := ar.New[article](tctx).New()
		second.Title = "tx clash"
		if err := second.Save(); err != nil {
			return err
		}
		if first.Slug != "tx-clash" || second.Slug != "tx-clash-2" {
			t.Errorf("slugs = (%q, %q), want (tx-clash, tx-clash-2)", first.Slug, second.Slug)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("transaction: %v", err)
	}
}

// optionless embeds Sluggable but defines no SlugOptions method.
type optionless struct {
	ar.Model
	Hooks
	Sluggable
	ID    int64  `gorm:"column:id;primaryKey"`
	Title string `gorm:"column:title"`
}

func (optionless) TableName() string { return "articles" }

// exec runs a raw statement on the connection backing ctx.
func exec(ctx context.Context, query string, args ...any) error {
	conn := base.FromContext(ctx).DB()
	return conn.Exec(query, args...).Error
}
