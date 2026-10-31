package web

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	z "github.com/Oudwins/zog"

	"github.com/goyii2/framework/form"
	"github.com/goyii2/framework/session"
	"github.com/goyii2/framework/validators"
)

func TestContextErrRecordsAndReadsError(t *testing.T) {
	c := newContext(nil, httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	if c.Err() != nil {
		t.Fatalf("fresh context error = %v, want nil", c.Err())
	}
	err := NotFound("nope")
	c.SetError(err)
	if c.Err() != err {
		t.Fatalf("Err() = %v, want the recorded error", c.Err())
	}
}

func TestSwapWriterRedirectsActionWritesAndRestores(t *testing.T) {
	rec := httptest.NewRecorder()
	c := newContext(nil, rec, httptest.NewRequest(http.MethodGet, "/", nil))

	cw := NewCaptureWriter(httptest.NewRecorder())
	restore := c.SwapWriter(cw)

	// Writes through the Context (how actions write) land in the swap's
	// writer, not the original one.
	c.Response().Header().Set("Content-Type", "text/html")
	c.Response().WriteHeader(http.StatusTeapot)
	if _, err := c.Response().Write([]byte("captured")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("original writer received bytes: %q", rec.Body.String())
	}
	if cw.Body() == nil || string(cw.Body()) != "captured" {
		t.Errorf("swap writer captured %q", cw.Body())
	}
	if cw.Header().Get("Content-Type") != "text/html" {
		t.Errorf("headers went to the wrong writer: %v", cw.Header())
	}

	restore()
	// After the restore the Context writes to the original writer again.
	if _, err := c.Response().Write([]byte("restored")); err != nil {
		t.Fatalf("write after restore: %v", err)
	}
	if got := rec.Body.String(); got != "restored" {
		t.Errorf("original writer got %q, want %q", got, "restored")
	}
	// The restored writer reports the status it wrote itself.
	if c.StatusCode() != http.StatusOK {
		t.Errorf("restored status = %d, want 200", c.StatusCode())
	}
}

func TestSessionAllReturnsIndependentCopy(t *testing.T) {
	m, err := session.New(session.Config{Name: "s", Backend: "file", Path: t.TempDir()})
	if err != nil {
		t.Fatalf("session.New: %v", err)
	}
	s := m.Begin(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	s.Set("cart", "books")

	all := s.All()
	if all["cart"] != "books" {
		t.Fatalf("All() = %v, want cart=books", all)
	}
	// Mutating the snapshot affects no live state.
	all["cart"] = "mutated"
	all["extra"] = "x"
	if s.Get("cart") != "books" {
		t.Errorf("mutating All() changed the session: %v", s.Get("cart"))
	}
	if s.Get("extra") != nil {
		t.Errorf("mutating All() wrote into the session: %v", s.Get("extra"))
	}
}

// loadFormModel carries both a parse-sensitive field and a rule, so a merged
// LoadForm failure is observable end to end.
type loadFormModel struct {
	Name string `form:"name"`
	Age  int    `form:"age"`
}

func (m *loadFormModel) Rules() validators.RuleSet {
	return validators.RuleSet{
		validators.Scenario("login"): z.Struct(z.Shape{
			"name": z.String().Required(z.Message("name cannot be blank")),
		}),
	}
}

// loadFormRequest builds a form-encoded login post.
func loadFormRequest(body string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return req
}

func TestLoadFormBindsAndValidates(t *testing.T) {
	c := newContext(nil, httptest.NewRecorder(), loadFormRequest("name=ada&age=42"))
	f := loadFormModel{}
	if errs := c.LoadForm(&f, validators.Scenario("login")); errs != nil {
		t.Fatalf("LoadForm(valid) = %v, want nil", errs)
	}
	if f.Name != "ada" || f.Age != 42 {
		t.Errorf("model = %+v, want the submitted values bound", f)
	}
}

func TestLoadFormMatchesCallingFormLoadDirectly(t *testing.T) {
	// The convenience method is a thin delegate: on the same request it
	// returns exactly what form.Load on the request's own context returns —
	// here a parse failure on age merged with the missing-name rule failure.
	req := loadFormRequest("age=abc")
	c := newContext(nil, httptest.NewRecorder(), req)
	var viaContext, direct loadFormModel
	viaErrs := c.LoadForm(&viaContext, validators.Scenario("login"))
	directErrs := form.Load(c.Context(), c.Request(), &direct, validators.Scenario("login"))
	if viaErrs == nil || directErrs == nil {
		t.Fatalf("LoadForm = %v, form.Load = %v, want both to reject", viaErrs, directErrs)
	}
	if !reflect.DeepEqual(viaErrs, directErrs) {
		t.Errorf("LoadForm = %v, form.Load = %v, want identical failures", viaErrs, directErrs)
	}
	if !reflect.DeepEqual(viaContext, direct) {
		t.Errorf("models differ: LoadForm bound %+v, form.Load bound %+v", viaContext, direct)
	}
	if got := viaErrs.Fields["age"]; len(got) != 1 || got[0] != "not a valid integer" {
		t.Errorf(`Fields["age"] = %q, want the parse message`, got)
	}
	if got := viaErrs.Fields["name"]; len(got) != 1 || got[0] != "name cannot be blank" {
		t.Errorf(`Fields["name"] = %q, want the rule message`, got)
	}
}
