package helpers

import (
	"reflect"
	"testing"
)

func TestArrayGetResolvesNestedPaths(t *testing.T) {
	data := map[string]any{
		"user": map[string]any{
			"name": "Alice",
			"roles": []any{
				map[string]any{"name": "admin"},
				map[string]any{"name": "editor"},
			},
		},
	}
	roles := data["user"].(map[string]any)["roles"].([]any)
	cases := []struct {
		path string
		want any
	}{
		{"user.name", "Alice"},
		{"user.roles.0.name", "admin"},
		{"user.roles.1.name", "editor"},
		{"user.roles.1", roles[1]},
	}
	for _, tc := range cases {
		if got, ok := ArrayGet[any](data, tc.path); !ok || !reflect.DeepEqual(got, tc.want) {
			t.Errorf("ArrayGet(%q) = %#v, %v; want %#v, true", tc.path, got, ok, tc.want)
		}
	}
}

func TestArrayGetMissingPathsReportNotFound(t *testing.T) {
	data := map[string]any{
		"a":    map[string]any{"b": "c"},
		"list": []any{"x"},
	}
	missing := []string{
		"",       // empty path never resolves
		"z",      // unknown top-level key
		"a.z",    // unknown nested key
		"a.b.c",  // scalar is not a container
		"list.1", // index out of range
		"list.x", // non-numeric index on a slice
		"a.b.c.d",
	}
	for _, path := range missing {
		if _, ok := ArrayGet[any](data, path); ok {
			t.Errorf("ArrayGet(%q) unexpectedly found a value", path)
		}
	}
}

func TestArrayGetTypedResults(t *testing.T) {
	data := map[string]any{
		"name":  "Alice",
		"count": float64(7), // the shape JSON decoding produces
		"ok":    true,
	}
	if v, ok := ArrayGet[string](data, "name"); !ok || v != "Alice" {
		t.Errorf("string read got %q, %v", v, ok)
	}
	if v, ok := ArrayGet[float64](data, "count"); !ok || v != 7 {
		t.Errorf("float64 read got %v, %v", v, ok)
	}
	if v, ok := ArrayGet[bool](data, "ok"); !ok || !v {
		t.Errorf("bool read got %v, %v", v, ok)
	}
	// Wrong type is a miss, not a panic and not a coerced value.
	if _, ok := ArrayGet[string](data, "count"); ok {
		t.Error("reading float64 as string should not be found")
	}
}

func TestArrayGetOrFallsBack(t *testing.T) {
	data := map[string]any{"host": "db1"}
	if got := ArrayGetOr(data, "host", "fallback"); got != "db1" {
		t.Errorf("present key got %q", got)
	}
	if got := ArrayGetOr(data, "port", 3306); got != 3306 {
		t.Errorf("missing key got %v", got)
	}
	if got := ArrayGetOr(data, "host", 3306); got != 3306 {
		t.Errorf("type-mismatched key should fall back to %v, got %v", 3306, got)
	}
}

func TestArrayMergeYii2Semantics(t *testing.T) {
	base := map[string]any{
		"app": "web",
		"db": map[string]any{
			"host": "localhost",
			"port": float64(3306),
		},
		"tags":       []any{"a", "b"},
		"onlyInBase": "kept",
	}
	override := map[string]any{
		"db":             map[string]any{"host": "db1", "user": "root"},
		"tags":           []any{"c"},
		"onlyInOverride": "added",
	}
	got := ArrayMerge(base, override)

	// Scalars override, maps recurse per key, slices append.
	want := map[string]any{
		"app": "web",
		"db": map[string]any{
			"host": "db1",
			"port": float64(3306),
			"user": "root",
		},
		"tags":           []any{"a", "b", "c"},
		"onlyInBase":     "kept",
		"onlyInOverride": "added",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("merge result\n got  %#v\n want %#v", got, want)
	}
}

func TestArrayMergeReturnsIndependentTree(t *testing.T) {
	base := map[string]any{"db": map[string]any{"port": float64(3306), "opts": []any{1.0}}}
	override := map[string]any{"db": map[string]any{"host": "h"}}
	merged := ArrayMerge(base, override)

	// Mutate everything the result exposes; neither input may notice.
	nested := merged["db"].(map[string]any)
	nested["host"] = "changed"
	nested["port"] = float64(0)
	nested["opts"] = append(nested["opts"].([]any), 99.0)

	baseDB := base["db"].(map[string]any)
	if baseDB["port"] != float64(3306) {
		t.Error("merged map aliases the input map")
	}
	if got := len(baseDB["opts"].([]any)); got != 1 {
		t.Errorf("merged slice aliases the input slice; base now has %d items", got)
	}
	if override["db"].(map[string]any)["host"] != "h" {
		t.Error("merged map aliases the override map")
	}
}

func TestArrayMergeEmptyInputs(t *testing.T) {
	if got := ArrayMerge(nil, nil); len(got) != 0 {
		t.Errorf("nil merge should be empty, got %v", got)
	}
	got := ArrayMerge(nil, map[string]any{"a": 1.0})
	if v, ok := got["a"].(float64); !ok || v != 1 {
		t.Errorf("override-only merge got %v", got)
	}
}

func TestArrayMapFilter(t *testing.T) {
	nums := []int{1, 2, 3, 4}
	doubled := ArrayMap(nums, func(n int) int { return n * 2 })
	if !reflect.DeepEqual(doubled, []int{2, 4, 6, 8}) {
		t.Errorf("Map got %v", doubled)
	}
	strs := ArrayMap(nums, func(n int) string {
		if n%2 == 0 {
			return "even"
		}
		return "odd"
	})
	if !reflect.DeepEqual(strs, []string{"odd", "even", "odd", "even"}) {
		t.Errorf("Map with type change got %v", strs)
	}
	evens := ArrayFilter(nums, func(n int) bool { return n%2 == 0 })
	if !reflect.DeepEqual(evens, []int{2, 4}) {
		t.Errorf("Filter got %v", evens)
	}
	// Empty results stay non-nil so they encode as [] instead of null.
	if got := ArrayMap(nil, func(n int) int { return n }); got == nil || len(got) != 0 {
		t.Errorf("Map(nil) should be empty non-nil, got %#v", got)
	}
	if got := ArrayFilter(nums, func(int) bool { return false }); got == nil || len(got) != 0 {
		t.Errorf("all-filtered result should be empty non-nil, got %#v", got)
	}
}

func TestArrayColumn(t *testing.T) {
	rows := []map[string]any{
		{"id": float64(1), "name": "Alice"},
		{"id": float64(2)}, // no name: skipped, not padded
		{"id": float64(3), "name": "Bob"},
	}
	got := ArrayColumn(rows, "name")
	if !reflect.DeepEqual(got, []any{"Alice", "Bob"}) {
		t.Errorf("Column got %#v", got)
	}
	if got := ArrayColumn(rows, "missing"); got == nil || len(got) != 0 {
		t.Errorf("Column on absent key should be empty non-nil, got %#v", got)
	}
}

func TestArrayIndex(t *testing.T) {
	type user struct{ Name string }
	users := []user{{"alice"}, {"bob"}, {"carol"}}
	byName := ArrayIndex(users, func(u user) string { return u.Name })
	if len(byName) != 3 || byName["bob"].Name != "bob" {
		t.Errorf("Index got %#v", byName)
	}
	// Duplicate keys: later item wins, matching Yii2's index.
	dupes := []user{{"same"}, {"later"}}
	if got := ArrayIndex(dupes, func(u user) string { return "same" }); got["same"].Name != "later" {
		t.Errorf("Index duplicate key should keep the later item, got %#v", got)
	}
}
