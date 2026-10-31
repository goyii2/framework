package form

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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

func TestBindJSONAssignsAllSupportedScalarTypes(t *testing.T) {
	// JSON sends typed values: numbers for numeric fields, booleans for bool
	// fields, and text only for text, time and self-decoding fields.
	body := `{
		"name": "ada",
		"age": 42,
		"small": -100,
		"big": 9999999999,
		"count": 7,
		"qty": 60000,
		"price": 19.5,
		"weight": 70.25,
		"active": true,
		"created": "2026-10-31T12:30:00+08:00",
		"role": "admin"
	}`
	f := allTypes{}
	if errs := Bind(jsonRequest(body), &f); errs != nil {
		t.Fatalf("Bind(json) = %v, want nil", errs)
	}
	if f.Name != "ada" || f.Age != 42 || f.Small != -100 || f.Big != 9999999999 {
		t.Errorf("ints = %+v, want the submitted numbers", f)
	}
	if f.Count != 7 || f.Qty != 60000 {
		t.Errorf("uints = %+v, want the submitted numbers", f)
	}
	if f.Price != 19.5 || f.Weight != 70.25 {
		t.Errorf("floats = %+v, want the submitted numbers", f)
	}
	if !f.Active {
		t.Error("Active = false, want true")
	}
	wantCreated := time.Date(2026, 10, 31, 4, 30, 0, 0, time.UTC) // 12:30 at +08:00
	if !f.Created.Equal(wantCreated) {
		t.Errorf("Created = %v, want %v", f.Created, wantCreated)
	}
	if got := f.Created.Format("-0700"); got != "+0800" {
		t.Errorf("Created offset = %s, want +0800 preserved", got)
	}
	if f.Role.Name != "admin" {
		t.Errorf("Role.Name = %q, want admin", f.Role.Name)
	}
}

func TestJSONTypeErrorsCollectPerField(t *testing.T) {
	// A value whose JSON kind does not fit the field is a field error, not a
	// request failure: every failing key reports the message of what its field
	// needed, and the valid fields in the same body still assign.
	body := `{
		"name": "ada",
		"age": "42",
		"count": -3,
		"small": 200,
		"price": "abc",
		"active": "maybe",
		"created": 12345,
		"role": {"name": "hacker"}
	}`
	f := allTypes{}
	errs := Bind(jsonRequest(body), &f)
	if errs == nil {
		t.Fatal("Bind(json) = nil, want per-field errors")
	}
	if errs.Scenario != ScenarioBind {
		t.Errorf("Scenario = %q, want %q", errs.Scenario, ScenarioBind)
	}
	if f.Name != "ada" {
		t.Errorf("Name = %q, want valid fields assigned", f.Name)
	}
	want := map[string][]string{
		"age":     {"not a valid integer"},
		"count":   {"not a valid integer"},
		"small":   {"not a valid integer"},
		"price":   {"not a valid number"},
		"active":  {"not a valid boolean"},
		"created": {"not a valid date-time"},
		"role":    {"not a valid value"},
	}
	if len(errs.Fields) != len(want) {
		t.Fatalf("Fields = %v, want exactly %d failing keys", errs.Fields, len(want))
	}
	for field, messages := range want {
		if got := errs.Fields[field]; len(got) != 1 || got[0] != messages[0] {
			t.Errorf("Fields[%s] = %q, want %q", field, got, messages)
		}
	}
}

func TestJSONValueKindMismatchOnStringFieldFails(t *testing.T) {
	// The strictness cuts both ways: a JSON number does not land in a text
	// field either — text fields are fed text.
	type noteForm struct {
		Note string `form:"note"`
	}
	f := noteForm{}
	errs := Bind(jsonRequest(`{"note": 42}`), &f)
	if errs == nil {
		t.Fatal("Bind = nil, want a field error")
	}
	if got := errs.Fields["note"]; len(got) != 1 || got[0] != "not a valid value" {
		t.Errorf(`Fields["note"] = %q, want the value message`, got)
	}
	if f.Note != "" {
		t.Errorf("Note = %q, want the zero value", f.Note)
	}
}

func TestJSONArrayValuesBindStringSlice(t *testing.T) {
	// A JSON array is the []string shape; a lone string mirrors a form with a
	// single submitted value and binds a one-element list.
	f := allTypes{}
	if errs := Bind(jsonRequest(`{"tags": ["go", "web"]}`), &f); errs != nil {
		t.Fatalf("Bind = %v, want nil", errs)
	}
	if len(f.Tags) != 2 || f.Tags[0] != "go" || f.Tags[1] != "web" {
		t.Errorf("Tags = %q, want [go web]", f.Tags)
	}
	f = allTypes{}
	if errs := Bind(jsonRequest(`{"tags": "solo"}`), &f); errs != nil {
		t.Fatalf("Bind(lone string) = %v, want nil", errs)
	}
	if len(f.Tags) != 1 || f.Tags[0] != "solo" {
		t.Errorf("Tags = %q, want [solo]", f.Tags)
	}
	// An explicitly empty array states "no values" and assigns an empty list.
	f = allTypes{}
	if errs := Bind(jsonRequest(`{"tags": []}`), &f); errs != nil {
		t.Fatalf("Bind(empty array) = %v, want nil", errs)
	}
	if f.Tags == nil || len(f.Tags) != 0 {
		t.Errorf("Tags = %#v, want an assigned empty list", f.Tags)
	}
}

func TestJSONArraysOfNonStringsAndNestedObjectsFailPerField(t *testing.T) {
	// Array elements must all be text; nested objects never bind a scalar
	// field — they are a type error under the field's own message, mirroring
	// the form source reporting its fields in field terms.
	f := allTypes{}
	errs := Bind(jsonRequest(`{"tags": ["go", 2]}`), &f)
	if errs == nil {
		t.Fatal("Bind = nil, want a field error")
	}
	if got := errs.Fields["tags"]; len(got) != 1 || got[0] != "not a valid value" {
		t.Errorf(`Fields["tags"] = %q, want the value message`, got)
	}
	if f.Tags != nil {
		t.Errorf("Tags = %q, want no assignment", f.Tags)
	}
	f = allTypes{}
	errs = Bind(jsonRequest(`{"age": {"value": 42}}`), &f)
	if errs == nil {
		t.Fatal("Bind = nil, want a field error")
	}
	if got := errs.Fields["age"]; len(got) != 1 || got[0] != "not a valid integer" {
		t.Errorf(`Fields["age"] = %q, want the integer message`, got)
	}
	if f.Age != 0 {
		t.Errorf("Age = %d, want the zero value", f.Age)
	}
}

func TestJSONEmptyTextAndNullMeanUnfilled(t *testing.T) {
	// A JSON empty string sent to a non-text field reads as "the input was
	// left unfilled" — the same verdict the form source gives a blank value —
	// so the field keeps its current value and nothing reports a failure.
	// null means the same: no value arrived. Whether a field may stay blank
	// stays the validators' verdict, not the binder's.
	body := `{
		"name": "",
		"age": "",
		"price": "",
		"active": "",
		"tags": "",
		"created": "",
		"role": "",
		"count": null
	}`
	f := allTypes{Age: 3, Active: true, Count: 9}
	if errs := Bind(jsonRequest(body), &f); errs != nil {
		t.Fatalf("Bind(blank inputs) = %v, want nil", errs)
	}
	if f.Name != "" {
		t.Errorf("Name = %q, want the empty text assigned", f.Name)
	}
	if f.Age != 3 {
		t.Errorf("Age = %d, want 3 kept: an empty string is not provided", f.Age)
	}
	if !f.Active {
		t.Error("Active = false, want true kept: an empty string is not provided")
	}
	if f.Count != 9 {
		t.Errorf("Count = %d, want 9 kept: null is not provided", f.Count)
	}
	if f.Tags != nil {
		t.Errorf("Tags = %q, want no assignment", f.Tags)
	}
	if !f.Created.IsZero() {
		t.Errorf("Created = %v, want the zero value", f.Created)
	}
	if f.Role.Name != "" {
		t.Errorf("Role.Name = %q, want the zero value", f.Role.Name)
	}
}
