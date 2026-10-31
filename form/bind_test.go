package form

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"
)

// role is the encoding.TextUnmarshaler stand-in: it accepts two literal
// values and rejects everything else, proving custom types parse themselves.
type role struct {
	Name string
}

func (r *role) UnmarshalText(text []byte) error {
	switch s := string(text); s {
	case "admin", "guest":
		r.Name = s
		return nil
	default:
		return errors.New("unknown role")
	}
}

// allTypes carries one field of every supported type.
type allTypes struct {
	Name    string    `form:"name"`
	Age     int       `form:"age"`
	Small   int8      `form:"small"`
	Big     int64     `form:"big"`
	Count   uint      `form:"count"`
	Qty     uint16    `form:"qty"`
	Price   float32   `form:"price"`
	Weight  float64   `form:"weight"`
	Active  bool      `form:"active"`
	Tags    []string  `form:"tags"`
	Created time.Time `form:"created"`
	Role    role      `form:"role"`
}

func TestBindAssignsAllSupportedTypes(t *testing.T) {
	body := url.Values{
		"name":    {"ada"},
		"age":     {"42"},
		"small":   {"-100"},
		"big":     {"9999999999"},
		"count":   {"7"},
		"qty":     {"60000"},
		"price":   {"19.5"},
		"weight":  {"70.25"},
		"active":  {"true"},
		"tags":    {"go", "web"},
		"created": {"2026-10-31T12:30:00+08:00"},
		"role":    {"admin"},
	}
	f := allTypes{}
	if errs := Bind(formRequest(http.MethodPost, "/x", body), &f); errs != nil {
		t.Fatalf("Bind = %v, want nil", errs)
	}
	wantCreated := time.Date(2026, 10, 31, 4, 30, 0, 0, time.UTC) // 12:30 at +08:00
	checks := []struct {
		name string
		got  any
		want any
	}{
		{"Name", f.Name, "ada"},
		{"Age", f.Age, 42},
		{"Small", f.Small, int8(-100)},
		{"Big", f.Big, int64(9999999999)},
		{"Count", f.Count, uint(7)},
		{"Qty", f.Qty, uint16(60000)},
		{"Price", f.Price, float32(19.5)},
		{"Weight", f.Weight, 70.25},
		{"Active", f.Active, true},
		{"Tags", f.Tags, []string{"go", "web"}},
		{"Role", f.Role.Name, "admin"},
	}
	for _, c := range checks {
		if !reflect.DeepEqual(c.got, c.want) {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}
	if !f.Created.Equal(wantCreated) {
		t.Errorf("Created = %v, want %v", f.Created, wantCreated)
	}
	if got := f.Created.Format("-0700"); got != "+0800" {
		t.Errorf("Created offset = %s, want +0800 preserved", got)
	}
}

func TestBindQueryAssignsAllSupportedTypes(t *testing.T) {
	query := url.Values{
		"name":    {"ada"},
		"age":     {"42"},
		"active":  {"true"},
		"tags":    {"go", "web"},
		"created": {"2026-10-31T12:30:00Z"},
		"role":    {"guest"},
	}.Encode()
	f := allTypes{}
	if errs := BindQuery(httptest.NewRequest(http.MethodGet, "/posts?"+query, nil), &f); errs != nil {
		t.Fatalf("BindQuery = %v, want nil", errs)
	}
	if f.Name != "ada" || f.Age != 42 || !f.Active {
		t.Errorf("scalars = %+v, want name/age/active assigned", f)
	}
	if len(f.Tags) != 2 || f.Tags[0] != "go" || f.Tags[1] != "web" {
		t.Errorf("Tags = %q, want [go web]", f.Tags)
	}
	if !f.Created.Equal(time.Date(2026, 10, 31, 12, 30, 0, 0, time.UTC)) {
		t.Errorf("Created = %v, want the parsed instant", f.Created)
	}
	if f.Role.Name != "guest" {
		t.Errorf("Role.Name = %q, want guest", f.Role.Name)
	}
}

func TestTypeErrorsCollectPerField(t *testing.T) {
	body := url.Values{
		"name":    {"ada"}, // valid fields still assign while others fail
		"age":     {"not-a-number"},
		"count":   {"-3"}, // unsigned rejects negatives
		"price":   {"abc"},
		"active":  {"maybe"},
		"created": {"2026-13-99T99:99:99Z"},
		"role":    {"hacker"},
	}
	f := allTypes{}
	errs := Bind(formRequest(http.MethodPost, "/x", body), &f)
	if errs == nil {
		t.Fatal("Bind = nil, want per-field errors")
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

func TestEmptyValuesLeaveNonStringFieldsUntouched(t *testing.T) {
	// A lone blank value means the input was left unfilled: text fields take
	// the empty text, every other field is treated as not provided and keeps
	// its current value, and nothing reports a parse failure. Requiredness is
	// the validators' verdict, not the binder's.
	body := url.Values{
		"name":    {""},
		"age":     {""},
		"price":   {""},
		"active":  {""},
		"tags":    {""},
		"created": {""},
		"role":    {""},
	}
	f := allTypes{Age: 3, Active: true}
	if errs := Bind(formRequest(http.MethodPost, "/x", body), &f); errs != nil {
		t.Fatalf("Bind(blank inputs) = %v, want nil", errs)
	}
	if f.Name != "" {
		t.Errorf("Name = %q, want the empty text assigned", f.Name)
	}
	if f.Age != 3 {
		t.Errorf("Age = %d, want 3 kept: a blank value is not provided", f.Age)
	}
	if !f.Active {
		t.Error("Active = false, want true kept: a blank value is not provided")
	}
	if len(f.Tags) != 0 {
		t.Errorf("Tags = %q, want no assignment", f.Tags)
	}
	if !f.Created.IsZero() {
		t.Errorf("Created = %v, want the zero value", f.Created)
	}
	if f.Role.Name != "" {
		t.Errorf("Role.Name = %q, want the zero value", f.Role.Name)
	}
}

func TestScalarFieldsReadTheFirstValue(t *testing.T) {
	body := url.Values{"age": {"5", "6"}, "tags": {"a", "b", "c"}}
	f := allTypes{}
	if errs := Bind(formRequest(http.MethodPost, "/x", body), &f); errs != nil {
		t.Fatalf("Bind = %v, want nil", errs)
	}
	if f.Age != 5 {
		t.Errorf("Age = %d, want the first value", f.Age)
	}
	if len(f.Tags) != 3 {
		t.Errorf("Tags = %q, want all three values", f.Tags)
	}
}

func TestIntOverflowFailsPerField(t *testing.T) {
	f := allTypes{}
	errs := Bind(formRequest(http.MethodPost, "/x", url.Values{"small": {"200"}}), &f)
	if errs == nil {
		t.Fatal("Bind = nil, want small to fail: 200 overflows int8")
	}
	if got := errs.Fields["small"]; len(got) != 1 || got[0] != "not a valid integer" {
		t.Errorf(`Fields["small"] = %q, want the integer message`, got)
	}
	if f.Small != 0 {
		t.Errorf("Small = %d, want the zero value", f.Small)
	}
}

func TestUnsupportedTaggedFieldTypePanics(t *testing.T) {
	type payload struct {
		Meta map[string]string `form:"meta"`
	}
	req := formRequest(http.MethodPost, "/x", url.Values{"meta": {"1"}})
	mustPanic(t, "form: field of type", func() { _ = Bind(req, &payload{}) })
}

func TestMalformedBodyReportsRootError(t *testing.T) {
	// A body that cannot even be decoded as form pairs is a whole-request
	// failure, not a field failure: it lands under the record-level $root key
	// the validators package uses for record-wide issues.
	req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader("%zz=1"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	f := stringsForm{}
	errs := Bind(req, &f)
	if errs == nil {
		t.Fatal("Bind = nil, want a root error")
	}
	if errs.Scenario != ScenarioBind {
		t.Errorf("Scenario = %q, want %q", errs.Scenario, ScenarioBind)
	}
	if got := errs.Fields["$root"]; len(got) != 1 || got[0] != "malformed form body" {
		t.Errorf(`Fields["$root"] = %q, want the body message`, got)
	}
}

// stringsForm is the minimal binding target: two tagged string fields, one
// exported field without a tag, and one opted out with "-".
type stringsForm struct {
	Login    string `form:"login"`
	Password string `form:"password"`
	Untagged string
	OptedOut string `form:"-"`
}

// formRequest builds a request whose body is the encoded form body, carrying
// the x-www-form-urlencoded content type browsers send.
func formRequest(method, target string, body url.Values) *http.Request {
	req := httptest.NewRequest(method, target, strings.NewReader(body.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return req
}

func TestBindFormBodyAssignsTaggedStringFields(t *testing.T) {
	body := url.Values{
		"login":    {"ada"},
		"password": {"s3cret"},
		// The tag is the whitelist: an untagged field stays zero even when
		// the request carries a value for its name, and "-" opts a field out.
		"Untagged": {"nope"},
		"OptedOut": {"nope"},
		// Unknown fields — hidden inputs like _csrf among them — are ignored.
		"_csrf": {"token"},
	}
	f := stringsForm{}
	if errs := Bind(formRequest(http.MethodPost, "/login", body), &f); errs != nil {
		t.Fatalf("Bind = %v, want nil", errs)
	}
	if f.Login != "ada" {
		t.Errorf("Login = %q, want %q", f.Login, "ada")
	}
	if f.Password != "s3cret" {
		t.Errorf("Password = %q, want %q", f.Password, "s3cret")
	}
	if f.Untagged != "" {
		t.Errorf("Untagged = %q, want the zero value: no tag means no binding", f.Untagged)
	}
	if f.OptedOut != "" {
		t.Errorf(`OptedOut = %q, want the zero value: "-" opts out`, f.OptedOut)
	}
}

func TestBindQueryAssignsTaggedStringFields(t *testing.T) {
	query := url.Values{
		"login":    {"ada"},
		"Untagged": {"nope"},
		"_csrf":    {"token"},
	}.Encode()
	req := httptest.NewRequest(http.MethodGet, "/posts?"+query, nil)
	f := stringsForm{}
	if errs := BindQuery(req, &f); errs != nil {
		t.Fatalf("BindQuery = %v, want nil", errs)
	}
	if f.Login != "ada" {
		t.Errorf("Login = %q, want %q", f.Login, "ada")
	}
	if f.Untagged != "" {
		t.Errorf("Untagged = %q, want the zero value", f.Untagged)
	}
}

func TestEmptySourcesLeaveZeroValues(t *testing.T) {
	f := stringsForm{Login: "kept", Password: "kept"}
	if errs := Bind(formRequest(http.MethodPost, "/login", url.Values{}), &f); errs != nil {
		t.Fatalf("Bind(empty body) = %v, want nil", errs)
	}
	if errs := BindQuery(httptest.NewRequest(http.MethodGet, "/login", nil), &f); errs != nil {
		t.Fatalf("BindQuery(no query) = %v, want nil", errs)
	}
}

func TestBindIgnoresNonFormBodies(t *testing.T) {
	// A body that is not form-urlencoded binds nothing: only the declared
	// form source is read.
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(`{"login":"ada"}`))
	req.Header.Set("Content-Type", "application/json")
	f := stringsForm{}
	if errs := Bind(req, &f); errs != nil {
		t.Fatalf("Bind(json body) = %v, want nil", errs)
	}
	if f.Login != "" {
		t.Errorf("Login = %q, want the zero value", f.Login)
	}
}

func TestNonStructDestinationsPanic(t *testing.T) {
	req := formRequest(http.MethodPost, "/login", url.Values{"login": {"ada"}})
	mustPanic(t, "form: destination", func() { _ = Bind(req, stringsForm{}) })
	mustPanic(t, "form: destination", func() { _ = Bind(req, (*stringsForm)(nil)) })
}

func mustPanic(t *testing.T, want string, fn func()) {
	t.Helper()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected a panic, got none")
		}
		msg, ok := r.(string)
		if !ok {
			t.Fatalf("panic value = %#v, want a string", r)
		}
		if !strings.Contains(msg, want) {
			t.Errorf("panic = %q, want it to contain %q", msg, want)
		}
	}()
	fn()
}
