package debug

import (
	"fmt"
	"maps"
	"slices"
	"strings"
)

// sortedKeys returns the map's keys sorted, for deterministic page rendering.
func sortedKeys(m map[string]any) []string {
	return slices.Sorted(maps.Keys(m))
}

// formatValue renders a session value for the page.
func formatValue(v any) string {
	switch v := v.(type) {
	case string:
		return v
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", v)
	}
}

// formatParams renders match parameters as "name=value" pairs.
func formatParams(m map[string]string) string {
	parts := make([]string, 0, len(m))
	for _, k := range slices.Sorted(maps.Keys(m)) {
		parts = append(parts, k+"="+m[k])
	}
	return strings.Join(parts, " ")
}

// formatAttrs renders log attributes as "key=value" pairs.
func formatAttrs(m map[string]any) string {
	parts := make([]string, 0, len(m))
	for _, k := range sortedKeys(m) {
		parts = append(parts, k+"="+formatValue(m[k]))
	}
	return strings.Join(parts, " ")
}
