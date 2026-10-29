package i18n

import (
	"slices"
	"testing"
)

func TestAcceptLanguagesSortsByQuality(t *testing.T) {
	got := AcceptLanguages("zh-CN,zh;q=0.9,en;q=0.8")
	want := []string{"zh-CN", "zh", "en"}
	if !slices.Equal(got, want) {
		t.Errorf("AcceptLanguages = %v, want %v", got, want)
	}
}

func TestAcceptLanguagesDropsZeroQuality(t *testing.T) {
	got := AcceptLanguages("fr;q=0, en;q=0.5, de")
	want := []string{"de", "en"} // equal quality keeps header order: de first
	if !slices.Equal(got, want) {
		t.Errorf("AcceptLanguages = %v, want %v", got, want)
	}
}

func TestAcceptLanguagesEmptyHeader(t *testing.T) {
	if got := AcceptLanguages(""); got != nil {
		t.Errorf("AcceptLanguages(\"\") = %v, want nil", got)
	}
	if got := AcceptLanguages("   "); got != nil {
		t.Errorf("AcceptLanguages(blank) = %v, want nil", got)
	}
}

func TestAcceptLanguagesMalformedHeaderFallsBackLoosely(t *testing.T) {
	// x/text rejects the whole header over the bad entry; the loose parser
	// keeps the salvageable ones.
	got := AcceptLanguages("!!nonsense!!, de")
	if !slices.Equal(got, []string{"de"}) {
		t.Errorf("AcceptLanguages(malformed) = %v, want [de]", got)
	}
}

func TestNegotiate(t *testing.T) {
	src := NewMessageSource(testMessages()) // available: de-DE, zh, zh-CN; default en-US
	cases := []struct {
		prefs []string
		want  string
	}{
		{[]string{"zh-CN"}, "zh-CN"},       // exact
		{[]string{"zh"}, "zh"},             // the zh directory itself exists
		{[]string{"en-GB"}, "en-US"},       // same language, other region
		{[]string{"fr"}, "en-US"},          // no affinity: the default
		{[]string{"de"}, "de-DE"},          // base language onto the full tag
		{[]string{"nonsense"}, "en-US"},    // unparseable falls through
		{[]string{"nonsense", "zh"}, "zh"}, // later prefs get their chance
		{nil, "en-US"},                     // no preference at all
		{[]string{"zh-Hans-CN"}, "zh-CN"},  // script tag still resolves
	}
	for _, tc := range cases {
		if got := src.Negotiate(tc.prefs...); got != tc.want {
			t.Errorf("Negotiate(%v) = %q, want %q", tc.prefs, got, tc.want)
		}
	}
}

func TestNegotiateHonorsDefaultLanguage(t *testing.T) {
	src := NewMessageSource(testMessages(), WithDefaultLanguage("de-DE"))
	if got := src.Negotiate(); got != "de-DE" {
		t.Errorf("Negotiate() = %q, want the configured default de-DE", got)
	}
	if got := src.Negotiate("fr"); got != "de-DE" {
		t.Errorf("Negotiate(fr) = %q, want the configured default de-DE", got)
	}
}
