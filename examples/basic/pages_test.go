// Tests of the listing search plumbing.
package main

import "testing"

// TestLikePatternEscapesWildcards pins the escaping contract of the listing
// search: the wrapping wildcards provide the contains match while every
// wildcard the user typed is neutralized to its literal meaning, so input
// can never reshape the match semantics.
func TestLikePatternEscapesWildcards(t *testing.T) {
	bs := string(rune(92)) // one backslash, the LIKE escape character
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"plain text stays literal", "hello", "%hello%"},
		{"percent is escaped", "50%", "%50" + bs + "%%"},
		{"underscore is escaped", "sample_post", "%sample" + bs + "_post%"},
		{"escape character is escaped", "a" + bs + "b", "%a" + bs + bs + "b%"},
		{"all wildcards together", "%_", "%" + bs + "%" + bs + "_" + "%"},
		{"empty input still wraps", "", "%%"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := likePattern(tc.raw); got != tc.want {
				t.Fatalf("likePattern(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}
