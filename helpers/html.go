package helpers

import (
	"fmt"
	"html"
	"sort"
	"strings"
)

// HTML is string content that is safe to embed in HTML output verbatim.
// Builders render HTML values as-is and escape everything else — plain
// strings, fmt.Stringer results, formatted values — so untrusted data stays
// escaped by default and only an explicit HtmlRaw or HtmlEscape call can
// produce verbatim output. The type exists to make "already handled" a
// decision the compiler can see, preventing the double-escape trap of
// escaping a string and passing it through a builder that escapes again.
type HTML string

// String returns the underlying content, so HTML values print and format
// like plain strings.
func (h HTML) String() string { return string(h) }

// HtmlEscape escapes every character significant in HTML text and attribute
// contexts (<, >, &, ', " through html.EscapeString) and returns the result
// as HTML: safe to embed anywhere in a document, and rendered verbatim by
// the builders rather than escaped a second time. This is the default
// treatment Yii2 gives output, made explicit.
func HtmlEscape(s string) HTML {
	return HTML(html.EscapeString(s))
}

// HtmlRaw marks s as trusted HTML that builders embed verbatim. It is the
// explicit opt-out from the default escaping and the only way unescaped
// markup enters builder output — use it for content you generated or
// sanitized yourself, never for user input.
func HtmlRaw(s string) HTML {
	return HTML(s)
}

// HtmlDecode unescapes HTML entities back to their characters
// ("&lt;b&gt;" -> "<b>"), the inverse of HtmlEscape and the counterpart of
// Yii2's Html::decode.
func HtmlDecode(s string) string {
	return html.UnescapeString(s)
}

// htmlContent coerces a builder value to HTML: HTML values pass through
// untouched, every other type is stringified and escaped.
func htmlContent(v any) HTML {
	switch t := v.(type) {
	case nil:
		return ""
	case HTML:
		return t
	case string:
		return HtmlEscape(t)
	case fmt.Stringer:
		return HtmlEscape(t.String())
	default:
		return HtmlEscape(fmt.Sprint(v))
	}
}

// HtmlTag renders "<tag attr=\"value\" ...>content</tag>" with attributes
// sorted by name for deterministic output. Content follows the package's
// escaping contract: plain values are escaped, HTML values (from HtmlEscape
// or HtmlRaw) are embedded verbatim. A nil content renders the void form
// "<img src=...>" without a closing tag for elements that never have one.
//
// Defense against markup injection through the code-controlled parts: tag
// and attribute names must be plain names (letters, digits, "-", "_":",
// no leading digit) — invalid ones are dropped, and an invalid tag name
// yields "" — and attribute values are always escaped, quotes included, so
// no value can break out of its quotes.
func HtmlTag(tag string, content any, attrs map[string]string) string {
	if !validName(tag) {
		return ""
	}
	var b strings.Builder
	b.WriteByte('<')
	b.WriteString(tag)
	writeAttrs(&b, attrs)
	b.WriteByte('>')
	if content == nil {
		return b.String()
	}
	b.WriteString(htmlContent(content).String())
	b.WriteString("</")
	b.WriteString(tag)
	b.WriteByte('>')
	return b.String()
}

// HtmlA renders an anchor with the href attribute — Html::a's common form;
// extra attributes go through HtmlTag directly. content obeys the same
// escaping contract as HtmlTag, and href is attribute-escaped.
func HtmlA(content any, href string) string {
	return HtmlTag("a", content, map[string]string{"href": href})
}

// writeAttrs appends the attributes in sorted name order. Invalid names are
// skipped so dynamic attribute tables cannot smuggle extra markup.
func writeAttrs(b *strings.Builder, attrs map[string]string) {
	if len(attrs) == 0 {
		return
	}
	names := make([]string, 0, len(attrs))
	for name := range attrs {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if !validName(name) {
			continue
		}
		b.WriteByte(' ')
		b.WriteString(name)
		b.WriteString(`="`)
		b.WriteString(html.EscapeString(attrs[name]))
		b.WriteByte('"')
	}
}

// validName accepts plain element/attribute names: a letter first, then
// letters, digits, "-", "_" or ":". Anything else — whitespace, quotes,
// slashes, "<" — is rejected.
func validName(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
		case r == '-' || r == '_' || r == ':':
		case r >= '0' && r <= '9':
			if i == 0 {
				return false
			}
		default:
			return false
		}
	}
	return true
}
