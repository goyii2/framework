package helpers

import (
	"strconv"
	"strings"
)

// ArrayGet resolves a dot-separated path against JSON-shaped data (nested
// map[string]any and []any, the shape JsonUnmarshal produces) and returns the
// value it names. It corresponds to Yii2's ArrayHelper::getValue for the
// multi-level path form: each segment descends one level — "user.name" reads
// data["user"]["name"], "items.0.id" reads data["items"][0]["id"]. A numeric
// segment on a map is an ordinary key lookup; only slices index numerically.
//
// found is false when any segment is missing, when a numeric segment hits a
// non-slice, or when the final value is not assignable to T. It never panics
// on malformed paths or an empty path.
func ArrayGet[T any](data any, path string) (zero T, found bool) {
	if path == "" {
		return zero, false
	}
	next := pathIter(path)
	node := data
	for {
		seg, more := next()
		switch n := node.(type) {
		case map[string]any:
			v, ok := n[seg]
			if !ok {
				return zero, false
			}
			node = v
		case []any:
			i, err := strconv.Atoi(seg)
			if err != nil || i < 0 || i >= len(n) {
				return zero, false
			}
			node = n[i]
		default:
			return zero, false
		}
		if !more {
			break
		}
	}
	v, ok := node.(T)
	if !ok {
		return zero, false
	}
	return v, true
}

// ArrayGetOr is ArrayGet with a fallback: when the path does not resolve or
// the value has the wrong type, fallback is returned. This mirrors Yii2's
// getValue($array, $key, $defaultValue) shape for the common read-with-
// default call sites.
func ArrayGetOr[T any](data any, path string, fallback T) T {
	if v, ok := ArrayGet[T](data, path); ok {
		return v
	}
	return fallback
}

// pathIter yields the segments of a dot-separated path one at a time; the
// boolean reports whether further segments remain after the yielded one.
func pathIter(path string) func() (string, bool) {
	rest := path
	return func() (seg string, more bool) {
		if rest == "" {
			return "", false
		}
		if i := strings.IndexByte(rest, '.'); i >= 0 {
			seg, rest = rest[:i], rest[i+1:]
			return seg, true
		}
		seg, rest = rest, ""
		return seg, false
	}
}

// ArrayMerge merges override into base with Yii2's ArrayHelper::merge
// semantics: where both sides hold a map the merge recurses, where both hold
// a slice the override items are appended to the base items (Yii2's
// integer-key arrays), and everything else from override replaces base. Keys
// present in only one side are kept.
//
// The result is a fresh tree — nested maps and slices are copied, never
// shared with the inputs — so later mutation of the result cannot be observed
// through base or override and vice versa. nil maps behave as empty.
func ArrayMerge(base, override map[string]any) map[string]any {
	out := make(map[string]any, len(base)+len(override))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range override {
		bv, exists := out[k]
		if !exists {
			out[k] = v
			continue
		}
		out[k] = mergeValues(bv, v)
	}
	return out
}

// mergeValues combines one key's two values: maps recurse, slices append,
// scalars defer to the override side.
func mergeValues(base, override any) any {
	bm, bok := base.(map[string]any)
	om, ook := override.(map[string]any)
	if bok && ook {
		return ArrayMerge(bm, om)
	}
	bs, bok := base.([]any)
	os, ook := override.([]any)
	if bok && ook {
		return append(append([]any{}, bs...), os...)
	}
	return override
}

// ArrayMap transforms every element of s with fn, corresponding to Yii2's
// array map usage; the result is non-nil so it serializes as [] rather than
// null.
func ArrayMap[T, U any](s []T, fn func(T) U) []U {
	out := make([]U, 0, len(s))
	for _, v := range s {
		out = append(out, fn(v))
	}
	return out
}

// ArrayFilter keeps the elements of s for which keep reports true. The
// result is non-nil so it serializes as [] rather than null.
func ArrayFilter[T any](s []T, keep func(T) bool) []T {
	out := make([]T, 0, len(s))
	for _, v := range s {
		if keep(v) {
			out = append(out, v)
		}
	}
	return out
}

// ArrayColumn collects the values under key from each row, corresponding to
// Yii2's ArrayHelper::getColumn for array rows: rows that do not carry key
// are skipped rather than padded with nils, which keeps the result usable
// without post-filtering. The result is non-nil.
func ArrayColumn(rows []map[string]any, key string) []any {
	out := make([]any, 0, len(rows))
	for _, row := range rows {
		if v, ok := row[key]; ok {
			out = append(out, v)
		}
	}
	return out
}

// ArrayIndex reindexes items into a map keyed by whatever keyOf extracts,
// corresponding to Yii2's ArrayHelper::index. When keyOf yields the same key
// twice the later item wins, matching Yii2.
func ArrayIndex[T any](items []T, keyOf func(T) string) map[string]T {
	out := make(map[string]T, len(items))
	for _, item := range items {
		out[keyOf(item)] = item
	}
	return out
}
