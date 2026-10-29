package i18n

import (
	"strconv"
	"strings"

	"golang.org/x/text/language"
)

// AcceptLanguages parses an Accept-Language header into language tags
// ordered by quality value (highest first, q=0 dropped), mirroring the
// browser's stated preference. Unparseable entries are skipped; an empty or
// fully unparseable header yields no tags.
func AcceptLanguages(header string) []string {
	if strings.TrimSpace(header) == "" {
		return nil
	}
	tags, weights, err := language.ParseAcceptLanguage(header)
	if err != nil {
		// x/text rejects the whole header on some malformed entries; fall
		// back to an entry-by-entry split so one bad tag does not erase the
		// rest of the preferences.
		return parseAcceptLoose(header)
	}
	var langs []string
	for i, tag := range tags {
		if weights[i] <= 0 {
			continue
		}
		langs = append(langs, tag.String())
	}
	return langs
}

// parseAcceptLoose parses "a;q=x, b;q=y" entries one by one, keeping only
// entries whose tag and quality both parse.
func parseAcceptLoose(header string) []string {
	type weighted struct {
		lang string
		q    float32
	}
	var entries []weighted
	for _, part := range strings.Split(header, ",") {
		fields := strings.Split(part, ";")
		tag, err := language.Parse(strings.TrimSpace(fields[0]))
		if err != nil {
			continue
		}
		q := float32(1)
		for _, param := range fields[1:] {
			value, ok := strings.CutPrefix(strings.TrimSpace(param), "q=")
			if !ok {
				continue
			}
			if parsed, err := strconv.ParseFloat(value, 32); err == nil {
				q = float32(parsed)
			}
		}
		if q > 0 {
			entries = append(entries, weighted{tag.String(), q})
		}
	}
	// Stable insertion sort by descending quality: ties keep the header's
	// own order.
	for i := 1; i < len(entries); i++ {
		for j := i; j > 0 && entries[j].q > entries[j-1].q; j-- {
			entries[j], entries[j-1] = entries[j-1], entries[j]
		}
	}
	langs := make([]string, len(entries))
	for i, e := range entries {
		langs[i] = e.lang
	}
	return langs
}

// Negotiate resolves the display language for one request. prefs lists
// language preferences highest priority first (query parameter, session
// value, then Accept-Language tags in quality order); the first preference
// that maps onto a translation directory wins — an exact hit, or the closest
// available language by x/text matching ("zh" resolves to a "zh-CN"
// directory when only that exists, "en-GB" to "en-US"). When no preference
// maps, the source's default language is returned. The result is always one
// of the source's language keys or its default, never a decorated variant.
func (m *MessageSource) Negotiate(prefs ...string) string {
	for _, pref := range prefs {
		tag, err := language.Parse(pref)
		if err != nil {
			continue
		}
		_, idx, conf := m.matcher.Match(tag)
		if conf == language.No {
			continue
		}
		return m.matched[idx]
	}
	return m.fallback.String()
}
