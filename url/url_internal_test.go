package url

import (
	"strings"
	"testing"
)

// absoluteURL keeps the explicit-error contract behind the builder's
// error-swallowing front: without host info it must refuse rather than emit
// a relative URL, and a missing path parameter must surface as an error
// naming it.
func TestAbsoluteURLInternalErrors(t *testing.T) {
	m := NewManager()
	m.Add("post/view", "/post/{id}")

	_, err := m.absoluteURL("post/view", P{"id": "7"})
	if err == nil || !strings.Contains(err.Error(), "host info") {
		t.Fatalf("absoluteURL without host info error = %v, want one naming the host info", err)
	}

	m.SetHostInfo("https://example.com")
	_, err = m.absoluteURL("post/view", nil)
	if err == nil || !strings.Contains(err.Error(), "id") {
		t.Fatalf("absoluteURL missing path parameter error = %v, want one naming the parameter", err)
	}

	got, err := m.absoluteURL("post/view", P{"id": "7"})
	if err != nil || got != "https://example.com/post/7" {
		t.Fatalf("absoluteURL = %q, %v; want https://example.com/post/7, nil", got, err)
	}
}
