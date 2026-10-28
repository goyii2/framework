package helpers

import (
	"strings"
	"testing"
)

func TestJsonMarshalEscapesHTML(t *testing.T) {
	// The XSS posture of the JSON helpers: markup-significant characters
	// must not survive in the encoded form.
	got, err := JsonMarshal(map[string]string{"q": `</script><img src=x onerror=alert(1)>`})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(got), "</script>") {
		t.Errorf("output embeds raw script tag: %s", got)
	}
	if !strings.Contains(string(got), `\u003c`) {
		t.Errorf("HTML characters should be \\u-escaped, got %s", got)
	}
}

func TestJsonMarshalString(t *testing.T) {
	got, err := JsonMarshalString([]string{"a", "b"})
	if err != nil {
		t.Fatal(err)
	}
	if got != `["a","b"]` {
		t.Errorf("got %s", got)
	}
	if _, err := JsonMarshalString(complex(1, 2)); err == nil {
		t.Error("unsupported types must return an error, not fail silently")
	}
}

func TestJsonUnmarshalRoundTrip(t *testing.T) {
	type payload struct {
		Name  string   `json:"name"`
		Tags  []string `json:"tags"`
		Scope int      `json:"scope"`
	}
	encoded, err := JsonMarshal(payload{Name: "web", Tags: []string{"a", "b"}, Scope: 3})
	if err != nil {
		t.Fatal(err)
	}
	var back payload
	if err := JsonUnmarshal(encoded, &back); err != nil {
		t.Fatal(err)
	}
	if back.Name != "web" || len(back.Tags) != 2 || back.Scope != 3 {
		t.Errorf("round trip lost data: %+v", back)
	}
}

func TestJsonUnmarshalRejectsMalformed(t *testing.T) {
	var dest map[string]any
	if err := JsonUnmarshal([]byte(`{"a":`), &dest); err == nil {
		t.Error("truncated JSON must be an error")
	}
	if err := JsonUnmarshal([]byte(`{} trailing`), &dest); err == nil {
		t.Error("trailing garbage must be an error")
	}
}
