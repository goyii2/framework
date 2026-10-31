package form

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// jsonRequest builds a request whose body is the given JSON text, carrying
// the content type an API client sends — charset suffix included, which the
// dispatcher tolerates exactly like the form path does.
func jsonRequest(body string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	return req
}

func TestBindJSONBodyAssignsTaggedStringFields(t *testing.T) {
	body := `{
		"login": "ada",
		"password": "s3cret",
		"Untagged": "nope",
		"untagged": "nope",
		"UNTAGGED": "nope",
		"OptedOut": "nope",
		"_csrf": "token"
	}`
	f := stringsForm{}
	if errs := Bind(jsonRequest(body), &f); errs != nil {
		t.Fatalf("Bind(json) = %v, want nil", errs)
	}
	if f.Login != "ada" {
		t.Errorf("Login = %q, want %q", f.Login, "ada")
	}
	if f.Password != "s3cret" {
		t.Errorf("Password = %q, want %q", f.Password, "s3cret")
	}
	// The tag is the whitelist under the JSON source too: keys never reach
	// untagged fields however their case is twisted — encoding/json's
	// case-insensitive field matching is exactly the hole the map detour
	// closes — and "-" still opts a field out.
	if f.Untagged != "" {
		t.Errorf("Untagged = %q, want the zero value: no JSON key case assigns it", f.Untagged)
	}
	if f.OptedOut != "" {
		t.Errorf(`OptedOut = %q, want the zero value: "-" opts out`, f.OptedOut)
	}
}
