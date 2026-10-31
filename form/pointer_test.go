package form

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"
	"time"
)

// nullableTypes carries the pointer shape of a model with nullable columns:
// every supported base type behind a pointer. Blank and absent inputs must
// leave the current pointee — nil included — untouched; arriving values
// allocate and bind into the pointee.
type nullableTypes struct {
	Name    string     `form:"name"`
	Count   *int       `form:"count"`
	Price   *float64   `form:"price"`
	Active  *bool      `form:"active"`
	At      *time.Time `form:"at"`
	Note    *string    `form:"note"`
	Ignored *string    // no tag: never assigned
}

func TestBindAssignsNullablePointerFields(t *testing.T) {
	at := "2026-10-31T12:30:00Z"
	body := url.Values{
		"count":  {"7"},
		"price":  {"19.5"},
		"active": {"true"},
		"at":     {at},
		"note":   {"hello"},
	}
	f := nullableTypes{Name: "ada"}
	if errs := Bind(formRequest(http.MethodPost, "/x", body), &f); errs != nil {
		t.Fatalf("Bind = %v, want nil", errs)
	}
	if f.Count == nil || *f.Count != 7 {
		t.Errorf("Count = %v, want *7", f.Count)
	}
	if f.Price == nil || *f.Price != 19.5 {
		t.Errorf("Price = %v, want *19.5", f.Price)
	}
	if f.Active == nil || *f.Active != true {
		t.Errorf("Active = %v, want *true", f.Active)
	}
	wantAt, _ := time.Parse(time.RFC3339, at)
	if f.At == nil || !f.At.Equal(wantAt) {
		t.Errorf("At = %v, want %v", f.At, wantAt)
	}
	if f.Note == nil || *f.Note != "hello" {
		t.Errorf("Note = %v, want *hello", f.Note)
	}
	if f.Ignored != nil {
		t.Errorf("Ignored = %v, want nil: an untagged pointer field never assigns", *f.Ignored)
	}
}

func TestBindQueryAssignsNullablePointerFields(t *testing.T) {
	query := url.Values{"count": {"42"}, "note": {"kept"}}.Encode()
	f := nullableTypes{}
	if errs := BindQuery(httptest.NewRequest(http.MethodGet, "/x?"+query, nil), &f); errs != nil {
		t.Fatalf("BindQuery = %v, want nil", errs)
	}
	if f.Count == nil || *f.Count != 42 {
		t.Errorf("Count = %v, want *42", f.Count)
	}
	if f.Note == nil || *f.Note != "kept" {
		t.Errorf("Note = %v, want *kept", f.Note)
	}
}

func TestBindJSONAssignsNullablePointerFields(t *testing.T) {
	body := `{"count": 3, "active": false, "note": "json"}`
	f := nullableTypes{}
	if errs := Bind(jsonRequest(body), &f); errs != nil {
		t.Fatalf("Bind(JSON) = %v, want nil", errs)
	}
	if f.Count == nil || *f.Count != 3 {
		t.Errorf("Count = %v, want *3", f.Count)
	}
	if f.Active == nil || *f.Active != false {
		t.Errorf("Active = %v, want *false", f.Active)
	}
	if f.Note == nil || *f.Note != "json" {
		t.Errorf("Note = %v, want *json", f.Note)
	}
}

// TestBindBlankAndNullLeaveNullableFieldsUntouched pins the unfilled
// semantics on pointer fields: a lone blank form value, a JSON empty string
// and a JSON null all keep the current pointee — nil stays nil, a stored
// value survives an update that left the input empty, and nothing is ever
// allocated for the blank.
func TestBindBlankAndNullLeaveNullableFieldsUntouched(t *testing.T) {
	stored := 11
	f := nullableTypes{Count: &stored}
	blank := url.Values{"count": {""}, "note": {""}}
	if errs := Bind(formRequest(http.MethodPost, "/x", blank), &f); errs != nil {
		t.Fatalf("Bind(blank form) = %v, want nil", errs)
	}
	if f.Count != &stored || *f.Count != 11 {
		t.Errorf("Count = %v, want the stored *11", f.Count)
	}
	if f.Note != nil {
		t.Errorf("Note = %v, want nil: a blank never allocates", *f.Note)
	}

	g := nullableTypes{}
	if errs := Bind(jsonRequest(`{"count": "", "note": null}`), &g); errs != nil {
		t.Fatalf("Bind(JSON blanks) = %v, want nil", errs)
	}
	if g.Count != nil || g.Note != nil {
		t.Errorf("Count, Note = %v, %v, want nil, nil: empty text and null bind nothing", g.Count, g.Note)
	}
}

func TestBindPointerTypeErrorsCollectPerField(t *testing.T) {
	f := nullableTypes{}
	errs := Bind(formRequest(http.MethodPost, "/x", url.Values{"count": {"x"}, "at": {"mayhem"}}), &f)
	if errs == nil {
		t.Fatal("Bind(bad pointer values) = nil, want per-field errors")
	}
	if got := errs.Fields["count"]; len(got) != 1 || got[0] != "not a valid integer" {
		t.Errorf("count messages = %v, want [not a valid integer]", got)
	}
	if got := errs.Fields["at"]; len(got) != 1 || got[0] != "not a valid date-time" {
		t.Errorf("at messages = %v, want [not a valid date-time]", got)
	}
	if f.Count != nil || f.At != nil {
		t.Error("a value that failed to parse must not allocate its pointee")
	}
}

func TestValuesExportsNullablePointerFields(t *testing.T) {
	note := "hi"
	count := 5
	at := time.Date(2026, 10, 31, 12, 30, 0, 0, time.UTC)
	got := Values(&nullableTypes{Note: &note, Count: &count, At: &at})
	want := map[string]string{
		"name":   "",
		"count":  "5",
		"price":  "",
		"active": "",
		"at":     at.Format(time.RFC3339),
		"note":   "hi",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Values = %v, want %v", got, want)
	}
}

// stamped formats itself only through its pointer receiver: a non-nil
// pointer field still renders, proving the text-form check runs on the
// pointer before the pointee takes over.
type stamped struct {
	Raw string
}

func (s *stamped) MarshalText() ([]byte, error) { return []byte("stamped:" + s.Raw), nil }

func TestValuesRendersPointerReceiverMarshalers(t *testing.T) {
	model := struct {
		Tag   *stamped `form:"tag"`
		Empty *stamped `form:"empty"`
	}{Tag: &stamped{Raw: "x"}}
	got := Values(&model)
	want := map[string]string{"tag": "stamped:x", "empty": ""}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Values = %v, want %v", got, want)
	}
}
