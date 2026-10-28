package helpers

import (
	"strings"
	"testing"
)

func TestStrRandomShape(t *testing.T) {
	s := StrRandom(32)
	if len(s) != 32 {
		t.Fatalf("length got %d, want 32", len(s))
	}
	const pool = strAlphabet
	for _, r := range s {
		if !strings.ContainsRune(pool, r) {
			t.Fatalf("character %q outside the alnum pool", r)
		}
	}
	if StrRandom(0) != "" || StrRandom(-3) != "" {
		t.Error("non-positive lengths should give an empty string")
	}
}

func TestStrRandomUnpredictableEnough(t *testing.T) {
	// Not a randomness proof: a cheap regression net catching a degenerate
	// generator (same seed, single character, obvious bias). A few thousand
	// draws of length 8 must be all distinct and use most of the alphabet.
	seen := make(map[string]struct{}, 2000)
	chars := make(map[rune]struct{})
	for i := 0; i < 2000; i++ {
		s := StrRandom(8)
		if _, dup := seen[s]; dup {
			t.Fatalf("duplicate value %q after %d draws", s, i)
		}
		seen[s] = struct{}{}
		for _, r := range s {
			chars[r] = struct{}{}
		}
	}
	if len(chars) < 50 {
		t.Errorf("only %d distinct characters observed; distribution looks skewed", len(chars))
	}
}

func TestStrCamel(t *testing.T) {
	cases := map[string]string{
		"hello_world":  "helloWorld",
		"hello-world":  "helloWorld",
		"hello world":  "helloWorld",
		"Hello World":  "helloWorld",
		"hello":        "hello",
		"HELLO_WORLD":  "hELLOWORLD", // Yii2 camelize preserves inner casing
		"getUserID":    "getUserID",  // already camel: untouched
		"_leading":     "leading",
		"trailing_":    "trailing",
		"":             "",
		"a_b_c":        "aBC",
		"multi  space": "multiSpace",
	}
	for in, want := range cases {
		if got := StrCamel(in); got != want {
			t.Errorf("StrCamel(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestStrSnake(t *testing.T) {
	cases := map[string]string{
		"helloWorld":   "hello_world",
		"HelloWorld":   "hello_world",
		"XMLParser":    "xml_parser", // capital run collapses into one word
		"getUserID":    "get_user_id",
		"hello_world":  "hello_world",
		"hello-world":  "hello_world",
		"Hello World":  "hello_world",
		"hello  world": "hello_world", // separator runs collapse
		"HELLO":        "hello",
		"user2Name":    "user2_name",
		"__x__":        "x",
		"":             "",
	}
	for in, want := range cases {
		if got := StrSnake(in); got != want {
			t.Errorf("StrSnake(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestStrUpperFirstLowerFirst(t *testing.T) {
	if got := StrUpperFirst("hello"); got != "Hello" {
		t.Errorf("UpperFirst got %q", got)
	}
	if got := StrUpperFirst("hELLO"); got != "HELLO" {
		t.Errorf("UpperFirst should only touch the first rune, got %q", got)
	}
	if got := StrUpperFirst(""); got != "" {
		t.Errorf("UpperFirst(\"\") got %q", got)
	}
	if got := StrUpperFirst("über"); got != "Über" {
		t.Errorf("UpperFirst should be unicode aware, got %q", got)
	}
	if got := StrLowerFirst("Hello"); got != "hello" {
		t.Errorf("LowerFirst got %q", got)
	}
	if got := StrLowerFirst("ÜBER"); got != "üBER" {
		t.Errorf("LowerFirst should be unicode aware, got %q", got)
	}
}

func TestStrTruncate(t *testing.T) {
	cases := []struct {
		in     string
		length int
		suffix string
		want   string
	}{
		{"hello", 10, "...", "hello"},         // under the limit: untouched
		{"hello", 5, "...", "hello"},          // exactly at the limit
		{"hello world", 8, "...", "hello..."}, // cut, trailing space trimmed
		{"hello world", 11, "...", "hello world"},
		{"abcdef", 4, "..", "ab.."}, // suffix counted in the budget
		{"abcdef", 3, "...", "..."}, // no room for content
		{"", 5, "...", ""},
		{"日本語テキスト", 4, "…", "日本語…"}, // multibyte counted by rune
	}
	for _, tc := range cases {
		if got := StrTruncate(tc.in, tc.length, tc.suffix); got != tc.want {
			t.Errorf("StrTruncate(%q, %d, %q) = %q, want %q", tc.in, tc.length, tc.suffix, got, tc.want)
		}
	}
}
