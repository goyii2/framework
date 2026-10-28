package helpers

import (
	"strings"
	"testing"
)

func TestHtmlEscapeCoversMarkupSignificantCharacters(t *testing.T) {
	// html.EscapeString's exact mapping: < > & " ' all leave the string as
	// entities (quotes as &#34;/&#39;).
	in := `a < b & c > d "e" 'f'`
	want := `a &lt; b &amp; c &gt; d &#34;e&#34; &#39;f&#39;`
	if got := HtmlEscape(in); got.String() != want {
		t.Errorf("got %s, want %s", got, want)
	}
	if strings.ContainsAny(string(HtmlEscape(in)), "<>'\"") {
		t.Error("raw markup characters survived escaping")
	}
	if HtmlDecode(want) != in {
		t.Errorf("HtmlDecode should invert HtmlEscape, got %s", HtmlDecode(want))
	}
	if string(HtmlEscape("")) != "" {
		t.Error("empty string should stay empty")
	}
}

func TestHtmlRawIsTheExplicitOptOut(t *testing.T) {
	got := HtmlRaw(`<b>bold</b>`)
	if got.String() != `<b>bold</b>` {
		t.Errorf("HtmlRaw must keep the markup verbatim, got %s", got)
	}
}

func TestHtmlTagEscapesContentByDefault(t *testing.T) {
	payload := `<img src=x onerror=alert(1)>`
	got := HtmlTag("p", payload, nil)
	if strings.Contains(got, "<img") {
		t.Fatalf("unescaped markup leaked into output: %s", got)
	}
	want := `<p>&lt;img src=x onerror=alert(1)&gt;</p>`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestHtmlTagEscapesAttributeValues(t *testing.T) {
	got := HtmlTag("div", "x", map[string]string{"title": `" onmouseover="alert(1)`})
	if strings.Contains(got, `" onmouseover="`) {
		t.Errorf("attribute value broke out of its quotes: %s", got)
	}
	// Quotes become &#34; entities, so the value stays inside its quotes.
	want := `<div title="&#34; onmouseover=&#34;alert(1)">x</div>`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestHtmlTagRendersHtmlValuesVerbatim(t *testing.T) {
	// HtmlEscape output is already safe; builders must not double-escape it.
	got := HtmlTag("p", HtmlEscape("a < b"), nil)
	if got != `<p>a &lt; b</p>` {
		t.Errorf("double escape detected: %s", got)
	}
	// HtmlRaw is the explicit verbatim channel.
	got = HtmlTag("p", HtmlRaw("<em>hi</em>"), nil)
	if got != `<p><em>hi</em></p>` {
		t.Errorf("HtmlRaw content must pass through, got %s", got)
	}
}

func TestHtmlTagVoidFormAndEmptyContent(t *testing.T) {
	// nil content: void element, no closing tag.
	if got := HtmlTag("img", nil, map[string]string{"src": "a.png", "alt": "x"}); got != `<img alt="x" src="a.png">` {
		t.Errorf("void form got %s", got)
	}
	// Empty string content is a real (typed) value: closing tag included.
	if got := HtmlTag("p", "", nil); got != `<p></p>` {
		t.Errorf("empty content got %s", got)
	}
}

func TestHtmlTagDeterministicAttributeOrder(t *testing.T) {
	a := HtmlTag("input", nil, map[string]string{"z": "1", "a": "2", "m": "3"})
	b := HtmlTag("input", nil, map[string]string{"a": "2", "m": "3", "z": "1"})
	if a != b {
		t.Errorf("attribute order must be sorted/deterministic: %s vs %s", a, b)
	}
}

func TestHtmlTagRejectsInjectedNames(t *testing.T) {
	if got := HtmlTag("script src=x", "y", nil); got != "" {
		t.Errorf("invalid tag name must yield empty output, got %q", got)
	}
	// Invalid attribute names are dropped, valid ones survive.
	got := HtmlTag("div", "x", map[string]string{"onclick=alert(1)": "1", "class": "ok"})
	if strings.Contains(got, "onclick") {
		t.Errorf("invalid attribute name leaked: %s", got)
	}
	if got != `<div class="ok">x</div>` {
		t.Errorf("got %s", got)
	}
	if got := HtmlTag("div", "x", map[string]string{"": "v"}); got != `<div>x</div>` {
		t.Errorf("empty attribute name must be dropped, got %s", got)
	}
}

func TestHtmlTagNonStringContent(t *testing.T) {
	if got := HtmlTag("span", 42, nil); got != `<span>42</span>` {
		t.Errorf("numeric content got %s", got)
	}
	if got := HtmlTag("span", 1.5, nil); got != `<span>1.5</span>` {
		t.Errorf("float content got %s", got)
	}
	// A Stringer is stringified through its String() method and the result
	// is escaped like plain text.
	shouty := testStringer("<b>")
	if got := HtmlTag("span", shouty, nil); got != `<span>&lt;B&gt;!</span>` {
		t.Errorf("Stringer content got %s", got)
	}
}

type testStringer string

func (s testStringer) String() string { return strings.ToUpper(string(s)) + "!" }

func TestHtmlA(t *testing.T) {
	if got := HtmlA("Home", "/"); got != `<a href="/">Home</a>` {
		t.Errorf("got %s", got)
	}
	// Untrusted link text is escaped; so is the href value.
	got := HtmlA(`<script>`, `x"y`)
	if strings.Contains(got, "<script>") || strings.Contains(got, `x"y`) {
		t.Errorf("unescaped content in anchor: %s", got)
	}
	// Raw content composes explicitly.
	if got := HtmlA(HtmlRaw("<b>Home</b>"), "/home"); got != `<a href="/home"><b>Home</b></a>` {
		t.Errorf("got %s", got)
	}
}
