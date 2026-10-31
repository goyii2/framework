package form

import (
	"net/http"
	"net/url"
	"testing"

	z "github.com/Oudwins/zog"

	"github.com/goyii2/framework/validators"
)

// loginScenario is the custom scenario the ruled test models validate under.
const loginScenario validators.Scenario = "login"

// signupForm is the ruled counterpart of stringsForm: the login scenario
// requires name and age and bounds the age, while count carries no rule so a
// parse failure there fails binding alone.
type signupForm struct {
	Name  string `form:"name"`
	Age   int    `form:"age"`
	Count int    `form:"count"`
}

func (f *signupForm) Rules() validators.RuleSet {
	return validators.RuleSet{
		loginScenario: z.Struct(z.Shape{
			"name": z.String().Required(z.Message("name cannot be blank")),
			"age": z.Int().Required(z.Message("age cannot be blank")).
				GT(17, z.Message("age must exceed 17")),
		}),
	}
}

// strictForm fails as a record: its schema carries a struct-level test that
// always rejects, landing under the "$root" key.
type strictForm struct {
	Name string `form:"name"`
}

func (f *strictForm) Rules() validators.RuleSet {
	return validators.RuleSet{
		loginScenario: z.Struct(z.Shape{
			"name": z.String().Required(z.Message("name cannot be blank")),
		}).TestFunc(func(val any, ctx z.Ctx) bool { return false },
			z.Message("record as a whole is off")),
	}
}

func TestLoadBindsAndValidatesInOneStep(t *testing.T) {
	f := signupForm{}
	req := formRequest(http.MethodPost, "/signup", url.Values{
		"name": {"ada"}, "age": {"42"}, "count": {"7"},
	})
	errs := Load(req.Context(), req, &f, loginScenario)
	if errs != nil {
		t.Fatalf("Load(valid) = %v, want nil", errs)
	}
	if f.Name != "ada" || f.Age != 42 || f.Count != 7 {
		t.Errorf("model = %+v, want every submitted value bound", f)
	}
}

func TestLoadBindsTheJSONSource(t *testing.T) {
	// Load rides Bind's content-type dispatch: a JSON body binds the same
	// model and validates under the same scenario.
	f := signupForm{}
	req := jsonRequest(`{"name": "ada", "age": 42}`)
	errs := Load(req.Context(), req, &f, loginScenario)
	if errs != nil {
		t.Fatalf("Load(json) = %v, want nil", errs)
	}
	if f.Name != "ada" || f.Age != 42 {
		t.Errorf("model = %+v, want the JSON values bound", f)
	}
}

func TestLoadMergesBindAndRuleErrorsInOrder(t *testing.T) {
	// age fails to parse and keeps zero, so its required rule fires on the
	// unbound value: the field carries both messages, the parse message
	// first. name never arrived, so only its rule reports.
	f := signupForm{}
	req := formRequest(http.MethodPost, "/signup", url.Values{"age": {"abc"}})
	errs := Load(req.Context(), req, &f, loginScenario)
	if errs == nil {
		t.Fatal("Load = nil, want merged errors")
	}
	if errs.Scenario != loginScenario {
		t.Errorf("Scenario = %q, want the scenario Load was asked for", errs.Scenario)
	}
	if got := errs.Fields["age"]; len(got) != 2 || got[0] != "not a valid integer" || got[1] != "age cannot be blank" {
		t.Errorf(`Fields["age"] = %q, want the parse message ahead of the rule message`, got)
	}
	if got := errs.Fields["name"]; len(got) != 1 || got[0] != "name cannot be blank" {
		t.Errorf(`Fields["name"] = %q, want the rule message`, got)
	}
}

func TestLoadRejectsParsedValuesTheRulesDislike(t *testing.T) {
	// Everything parses, so the failure is the rules' verdict on the bound
	// values alone — the bound-side message never appears.
	f := signupForm{}
	req := formRequest(http.MethodPost, "/signup", url.Values{"name": {"ada"}, "age": {"7"}})
	errs := Load(req.Context(), req, &f, loginScenario)
	if errs == nil {
		t.Fatal("Load = nil, want the rule failure")
	}
	if got := errs.Fields["age"]; len(got) != 1 || got[0] != "age must exceed 17" {
		t.Errorf(`Fields["age"] = %q, want the bound check alone`, got)
	}
}

func TestLoadReportsRuleFailuresAlone(t *testing.T) {
	// Everything parses but the rules reject: the failure is the validation
	// one, labeled with the requested scenario.
	f := signupForm{}
	req := formRequest(http.MethodPost, "/signup", url.Values{"name": {""}, "age": {"42"}})
	errs := Load(req.Context(), req, &f, loginScenario)
	if errs == nil {
		t.Fatal("Load = nil, want the rule failure")
	}
	want := map[string][]string{"name": {"name cannot be blank"}}
	if len(errs.Fields) != len(want) {
		t.Fatalf("Fields = %v, want exactly %v", errs.Fields, want)
	}
	if got := errs.Fields["name"]; len(got) != 1 || got[0] != want["name"][0] {
		t.Errorf(`Fields["name"] = %q, want %q`, got, want["name"])
	}
}

func TestLoadReportsBindFailuresOnRulelessFields(t *testing.T) {
	// count carries no rule in any scenario, so a value that cannot parse
	// fails on binding alone through the same merged shape.
	f := signupForm{}
	req := formRequest(http.MethodPost, "/signup", url.Values{
		"name": {"ada"}, "age": {"42"}, "count": {"abc"},
	})
	errs := Load(req.Context(), req, &f, loginScenario)
	if errs == nil {
		t.Fatal("Load = nil, want the parse failure")
	}
	if got := errs.Fields["count"]; len(got) != 1 || got[0] != "not a valid integer" {
		t.Errorf(`Fields["count"] = %q, want the parse message`, got)
	}
}

func TestLoadWithoutRulesOnlyBinds(t *testing.T) {
	// A model that implements no Rules only binds: values that parse return
	// nil, values that fail report through the same shape, and nothing
	// panics along the way.
	f := allTypes{}
	ok := formRequest(http.MethodPost, "/x", url.Values{"name": {"ada"}, "age": {"42"}})
	if errs := Load(ok.Context(), ok, &f, validators.ScenarioInsert); errs != nil {
		t.Fatalf("Load(valid, no rules) = %v, want nil", errs)
	}
	bad := formRequest(http.MethodPost, "/x", url.Values{"age": {"abc"}})
	errs := Load(bad.Context(), bad, &f, validators.ScenarioInsert)
	if errs == nil {
		t.Fatal("Load(bad value, no rules) = nil, want the parse failure")
	}
	if errs.Scenario != validators.ScenarioInsert {
		t.Errorf("Scenario = %q, want the requested scenario", errs.Scenario)
	}
	if got := errs.Fields["age"]; len(got) != 1 || got[0] != "not a valid integer" {
		t.Errorf(`Fields["age"] = %q, want the parse message`, got)
	}
}

func TestLoadScenarioWithoutSchemaPasses(t *testing.T) {
	// A scenario absent from the RuleSet has no rules to fail, so a body
	// that parses passes whatever the model's other scenarios reject.
	f := signupForm{}
	req := formRequest(http.MethodPost, "/signup", url.Values{})
	errs := Load(req.Context(), req, &f, validators.Scenario("checkout"))
	if errs != nil {
		t.Fatalf("Load(scenario without schema) = %v, want nil", errs)
	}
}

func TestLoadKeepsRootErrorsFromBothStages(t *testing.T) {
	// A body that cannot be decoded and a schema that rejects the record
	// both land under "$root": the parse message first, the struct-level
	// rule message after.
	f := strictForm{}
	req := jsonRequest(`{"name": `)
	errs := Load(req.Context(), req, &f, loginScenario)
	if errs == nil {
		t.Fatal("Load = nil, want root errors")
	}
	got := errs.Fields["$root"]
	if len(got) != 2 || got[0] != "malformed JSON body" || got[1] != "record as a whole is off" {
		t.Errorf(`Fields["$root"] = %q, want the body message ahead of the rule message`, got)
	}

	// Without rules the root failure is the body's alone.
	plain := stringsForm{}
	req = jsonRequest(`{"login": `)
	errs = Load(req.Context(), req, &plain, loginScenario)
	if errs == nil {
		t.Fatal("Load(no rules) = nil, want the root body error")
	}
	if got := errs.Fields["$root"]; len(got) != 1 || got[0] != "malformed JSON body" {
		t.Errorf(`Fields["$root"] = %q, want the body message alone`, got)
	}
}

func TestLoadTranslatesThroughTheRequestContext(t *testing.T) {
	// Both stages translate with their own path: the parse message picks up
	// the fixture's entry, the rule message — an id the fixture lacks —
	// stays verbatim.
	req := formRequest(http.MethodPost, "/signup", url.Values{"age": {"abc"}}).
		WithContext(translatedCtx("zh-CN"))
	f := signupForm{}
	errs := Load(translatedCtx("zh-CN"), req, &f, loginScenario)
	if errs == nil {
		t.Fatal("Load = nil, want merged errors")
	}
	if got := errs.Fields["age"]; len(got) != 2 || got[0] != "不是有效整数" || got[1] != "age cannot be blank" {
		t.Errorf(`Fields["age"] = %q, want the translated parse message ahead of the verbatim rule message`, got)
	}
}
