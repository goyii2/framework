package form

import (
	"reflect"
	"testing"
	"time"
)

func TestValuesExportsEveryTaggedFieldAsText(t *testing.T) {
	f := allTypes{
		Name:    "ada",
		Age:     42,
		Small:   -100,
		Big:     9999999999,
		Count:   7,
		Qty:     60000,
		Price:   19.5,
		Weight:  70.25,
		Active:  true,
		Tags:    []string{"go", "web"},
		Created: time.Date(2026, 10, 31, 4, 30, 0, 0, time.UTC),
		Role:    role{Name: "admin"},
	}
	got := Values(f)
	want := map[string]string{
		"name":    "ada",
		"age":     "42",
		"small":   "-100",
		"big":     "9999999999",
		"count":   "7",
		"qty":     "60000",
		"price":   "19.5",
		"weight":  "70.25",
		"active":  "true",
		"tags":    "go,web",
		"created": "2026-10-31T04:30:00Z",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Values() = %v, want %v", got, want)
	}
}

func TestValuesRendersZeroAndEmptyStates(t *testing.T) {
	// The zero model fixes the empty-state rulings: false renders "false",
	// a nil list renders "", the zero time renders its RFC 3339 text, and
	// fractional seconds survive so an edit form re-submits the stored
	// instant.
	f := allTypes{}
	got := Values(f)
	if got["active"] != "false" {
		t.Errorf(`active = %q, want "false"`, got["active"])
	}
	if got["tags"] != "" {
		t.Errorf(`tags = %q, want "" for a nil list`, got["tags"])
	}
	if got["created"] != "0001-01-01T00:00:00Z" {
		t.Errorf(`created = %q, want the zero time's RFC 3339 text`, got["created"])
	}
	f.Created = time.Date(2026, 10, 31, 4, 30, 0, 500000000, time.UTC)
	if got := Values(f)["created"]; got != "2026-10-31T04:30:00.5Z" {
		t.Errorf(`created = %q, want fractional seconds preserved`, got)
	}
	f.Tags = []string{}
	if got := Values(f)["tags"]; got != "" {
		t.Errorf(`tags = %q, want "" for an empty list`, got)
	}
}

func TestValuesOmitsFieldsWithoutATextForm(t *testing.T) {
	// role parses itself but formats nothing: it has no honest refill text,
	// so it stays out of the map instead of a guessed rendering.
	f := allTypes{Name: "ada", Role: role{Name: "admin"}}
	if _, ok := Values(f)["role"]; ok {
		t.Error(`Values() carries "role", want it omitted: Unmarshaler without a Marshaler`)
	}
}

func TestValuesOmitsUntaggedAndOptedOutFields(t *testing.T) {
	f := stringsForm{Login: "ada", Password: "s3cret", Untagged: "nope", OptedOut: "nope"}
	want := map[string]string{"login": "ada", "password": "s3cret"}
	if got := Values(f); !reflect.DeepEqual(got, want) {
		t.Errorf("Values(untagged fields set) = %v, want %v", got, want)
	}
}

func TestValuesAcceptsValueAndPointerModels(t *testing.T) {
	f := stringsForm{Login: "ada"}
	if got, want := Values(f), Values(&f); !reflect.DeepEqual(got, want) {
		t.Errorf("Values(value) = %v, Values(pointer) = %v, want identical maps", got, want)
	}
}

func TestValuesPanicsOnNonStructModels(t *testing.T) {
	mustPanic(t, "form: model", func() { _ = Values(42) })
	mustPanic(t, "form: model", func() { _ = Values("form") })
	mustPanic(t, "form: model", func() { _ = Values((*stringsForm)(nil)) })
}
