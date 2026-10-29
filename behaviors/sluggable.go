package behaviors

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"unicode"

	"gorm.io/gorm"
)

// Sluggable derives a URL slug from a source field of the embedding model
// and keeps it unique per table — the counterpart of Yii2's
// SluggableBehavior. The model states its configuration by implementing
//
//	func (a *Article) SlugOptions() behaviors.SlugOptions
//
// which shadows nothing the behavior provides, so the absence of the
// method is detectable and save refuses with an explanatory error rather
// than silently skipping the fill.
//
// Slugs generate when the slug is unset — on insert, and on update when
// the column is still empty. An update never rewrites an existing slug:
// URLs built on it keep resolving while the source field changes, the
// useful half of Yii2's immutable option made the default. Uniqueness is
// enforced by probing the table for the candidate and appending the
// suffix -2, -3, ... on collision; concurrent inserts racing between the
// probe and the insert need a unique index on the column to be airtight.
type Sluggable struct {
	Slug string `gorm:"column:slug"`
}

// SlugOptions configures the slug generation of one model.
type SlugOptions struct {
	// Source names the field whose value feeds the slug. Required.
	Source string

	// Separator joins the slug's words; empty means "-".
	Separator string

	// KeepCase keeps the source's letter case instead of lowercasing.
	KeepCase bool

	// KeepUnicode keeps letters and digits beyond ASCII as-is. The default
	// drops them, leaving an ASCII-only slug; without transliteration
	// (out of scope here) a source consisting solely of non-ASCII text
	// yields an empty slug. Applications serving such titles enable this
	// and carry the original text in the slug.
	KeepUnicode bool
}

// SlugOptionsProvider is the configuration seam: the embedding model
// implements it to state where the slug comes from and how it is shaped.
type SlugOptionsProvider interface {
	SlugOptions() SlugOptions
}

// maxSlugProbes bounds the uniqueness probing loop; a table holding more
// than this many colliding slugs is pathological and reported instead of
// looped through.
const maxSlugProbes = 10000

// onCreate fills the slug from the configured source field when it is
// unset, probing the table for a free value.
func (s *Sluggable) onCreate(model reflect.Value, tx *gorm.DB) error {
	if s.Slug != "" {
		return nil
	}
	return s.generate(model, tx)
}

// onUpdate fills the slug when the column is still empty; an existing slug
// is never regenerated.
func (s *Sluggable) onUpdate(model reflect.Value, tx *gorm.DB) error {
	if s.Slug != "" {
		return nil
	}
	return s.generate(model, tx)
}

// generate derives the slug and makes it unique for the table.
func (s *Sluggable) generate(model reflect.Value, tx *gorm.DB) error {
	if tx.Statement == nil || tx.Statement.Schema == nil {
		return fmt.Errorf("behaviors: Sluggable needs a parsed schema; save through the record's own methods")
	}
	options, err := slugOptionsOf(model)
	if err != nil {
		return err
	}
	source, err := sourceText(model, tx, options.Source)
	if err != nil {
		return err
	}
	base := slugify(source, options)
	if base == "" {
		// Nothing survived the filter; leaving the slug empty keeps the
		// save working, an explicit cue for the application to pick a
		// slug itself where empty ones are not acceptable.
		return nil
	}
	s.Slug, err = s.unique(model, tx, options, base)
	return err
}

// slugOptionsOf resolves the model's SlugOptions method and applies the
// defaults for the fields it left unset.
func slugOptionsOf(model reflect.Value) (SlugOptions, error) {
	provider, ok := model.Addr().Interface().(SlugOptionsProvider)
	if !ok {
		return SlugOptions{}, fmt.Errorf("behaviors: Sluggable needs the model %s to define SlugOptions() SlugOptions (state the source field)", model.Type())
	}
	options := provider.SlugOptions()
	if options.Separator == "" {
		options.Separator = "-"
	}
	return options, nil
}

// sourceText reads the configured source field of the model as text. The
// name may address the field directly or through embedding, the way GORM's
// own schema lookup does.
func sourceText(model reflect.Value, tx *gorm.DB, name string) (string, error) {
	field := tx.Statement.Schema.LookUpField(name)
	if field == nil {
		return "", fmt.Errorf("behaviors: slug source field %q not found on %s", name, model.Type())
	}
	value, _ := field.ValueOf(tx.Statement.Context, model)
	if text, ok := value.(string); ok {
		return text, nil
	}
	return fmt.Sprintf("%v", value), nil
}

// slugify reduces text to a slug under options: letters and digits are
// kept (ASCII-only unless unicode is allowed), everything else separates
// words, words join with the separator, and the result lowercases unless
// the case is explicitly kept.
func slugify(text string, options SlugOptions) string {
	keep := func(r rune) bool {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return false
		}
		if !options.KeepUnicode && r > unicode.MaxASCII {
			return false
		}
		return true
	}
	words := strings.FieldsFunc(text, func(r rune) bool { return !keep(r) })
	slug := strings.Join(words, options.Separator)
	if !options.KeepCase {
		slug = strings.ToLower(slug)
	}
	return slug
}

// unique probes the table until it finds a slug value not in use,
// appending the separator and a counter from 2 on collisions. The probe
// runs on the same connection the save uses, so inside a transaction it
// sees the transaction's own uncommitted rows too; a row saving itself is
// excluded by its primary key.
func (s *Sluggable) unique(model reflect.Value, tx *gorm.DB, options SlugOptions, base string) (string, error) {
	schema := tx.Statement.Schema
	slugField := schema.LookUpField("Slug")
	if slugField == nil || slugField.DBName == "" {
		return "", fmt.Errorf("behaviors: Sluggable needs its slug column mapped on %s", model.Type())
	}

	taken := func(candidate string) (bool, error) {
		probe := tx.Session(&gorm.Session{NewDB: true, SkipHooks: true}).
			Table(schema.Table).
			Where(slugField.DBName+" = ?", candidate)
		if key := schema.PrioritizedPrimaryField; key != nil && len(schema.PrimaryFields) == 1 && key.DBName != "" {
			if value, zero := key.ValueOf(tx.Statement.Context, model); !zero {
				probe = probe.Where(key.DBName+" <> ?", value)
			}
		}
		var n int64
		if err := probe.Count(&n).Error; err != nil {
			return false, fmt.Errorf("behaviors: probing slug uniqueness on %s: %w", schema.Table, err)
		}
		return n > 0, nil
	}

	candidate := base
	for attempt := 2; ; attempt++ {
		inUse, err := taken(candidate)
		if err != nil {
			return "", err
		}
		if !inUse {
			return candidate, nil
		}
		if attempt > maxSlugProbes {
			return "", fmt.Errorf("behaviors: no free slug for %q on %s after %d probes", base, schema.Table, maxSlugProbes)
		}
		candidate = base + options.Separator + strconv.Itoa(attempt)
	}
}
