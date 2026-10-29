package validators

import (
	"context"
	"strings"
	"testing"

	z "github.com/Oudwins/zog"
	"github.com/Oudwins/zog/conf"
	"github.com/Oudwins/zog/zconst"
)

// member is the plain test model: no persistence base, only the rule sets.
// Name is required on insert but free on update; the custom "login" scenario
// reuses the required check.
type member struct {
	Name  string
	Email string
	Age   int
}

func (m *member) Rules() RuleSet {
	return RuleSet{
		ScenarioInsert: z.Struct(z.Shape{
			"name":  z.String().Required(z.Message("name cannot be blank")),
			"email": z.String().Email(z.Message("email is not a valid address")),
		}),
		ScenarioUpdate: z.Struct(z.Shape{
			"email": z.String().Email(z.Message("email is not a valid address")),
		}),
		Scenario("login"): z.Struct(z.Shape{
			"name": z.String().Required(z.Message("name cannot be blank")),
		}),
	}
}

// scripted is a minimal Ruled model running whatever rule set the test
// assigns, so schemas built inline per case can ride the same Validate path.
type scripted struct {
	Name  string
	Email string
	Age   int
	rules RuleSet
}

func (s *scripted) Rules() RuleSet { return s.rules }

func TestValidateCollectsPerFieldMessages(t *testing.T) {
	m := &member{Name: "", Email: "not-an-email"}
	errs := Validate(context.Background(), m, ScenarioInsert)
	if errs == nil {
		t.Fatal("Validate = nil, want field errors for name and email")
	}
	if errs.Scenario != ScenarioInsert {
		t.Errorf("Scenario = %q, want %q", errs.Scenario, ScenarioInsert)
	}
	want := map[string][]string{
		"name":  {"name cannot be blank"},
		"email": {"email is not a valid address"},
	}
	for field, messages := range want {
		if got := errs.Fields[field]; !equalMessages(got, messages) {
			t.Errorf("Fields[%s] = %q, want %q", field, got, messages)
		}
	}
	if len(errs.Fields) != len(want) {
		t.Errorf("Fields = %v, want exactly the keys name and email", errs.Fields)
	}
}

func TestValidateSucceedsOnValidRecord(t *testing.T) {
	m := &member{Name: "ada", Email: "ada@example.com"}
	if errs := Validate(context.Background(), m, ScenarioInsert); errs != nil {
		t.Fatalf("Validate(valid) = %v, want nil", errs)
	}
}

// TestScenariosSelectDifferentRuleSets proves the scenario switch: the same
// record is rejected under one scenario and accepted under another, custom
// scenarios address their own schema, and a scenario without a schema passes.
func TestScenariosSelectDifferentRuleSets(t *testing.T) {
	// Name blank: rejected on insert (required), accepted on update (rule
	// absent from the update schema).
	blankName := &member{Name: "", Email: "ada@example.com"}
	if errs := Validate(context.Background(), blankName, ScenarioInsert); errs == nil {
		t.Error("insert scenario accepted a blank name, want rejection")
	}
	if errs := Validate(context.Background(), blankName, ScenarioUpdate); errs != nil {
		t.Errorf("update scenario rejected a blank name: %v, want acceptance", errs)
	}

	// Custom scenario runs its own schema.
	login := Validate(context.Background(), &member{Email: "ada@example.com"}, Scenario("login"))
	if login == nil {
		t.Fatal(`Validate(login) = nil, want rejection for the blank name`)
	}
	if got := login.Fields["name"]; !equalMessages(got, []string{"name cannot be blank"}) {
		t.Errorf(`login Fields["name"] = %q, want the required message`, got)
	}
}

func TestValidatePassesModelWithoutRules(t *testing.T) {
	type plain struct{ Name string }
	if errs := Validate(context.Background(), &plain{Name: ""}, ScenarioInsert); errs != nil {
		t.Errorf("Validate(model without Rules) = %v, want nil", errs)
	}
}

func TestValidatePassesScenarioWithoutSchema(t *testing.T) {
	// member declares no schema for this scenario, so nothing can fail.
	if errs := Validate(context.Background(), &member{Name: "", Email: ""}, Scenario("checkout")); errs != nil {
		t.Errorf("Validate(scenario without schema) = %v, want nil", errs)
	}
}

func TestValidateCollectsSeveralMessagesPerField(t *testing.T) {
	// Both bounds run on the value 7, so the field collects both messages.
	m := &scripted{Age: 7, rules: RuleSet{ScenarioInsert: z.Struct(z.Shape{
		"age": z.Int().GT(10, z.Message("age must exceed 10")).LT(5, z.Message("age must be below 5")),
	})}}
	errs := Validate(context.Background(), m, ScenarioInsert)
	if errs == nil {
		t.Fatal("Validate = nil, want two age messages")
	}
	if got := errs.Fields["age"]; !equalMessages(got, []string{"age must exceed 10", "age must be below 5"}) {
		t.Errorf(`Fields["age"] = %q, want both bound messages`, got)
	}
}

// TestGlobalTemplateOverride exercises the template seam: replacing an entry
// of zog's global default table restyles every message produced without a
// per-rule override, placeholders included.
func TestGlobalTemplateOverride(t *testing.T) {
	templates := conf.DefaultIssueMessageMap[zconst.TypeString]
	original, hadKey := templates[zconst.IssueCodeRequired]
	templates[zconst.IssueCodeRequired] = "{{value}} 不能为空"
	t.Cleanup(func() {
		if hadKey {
			templates[zconst.IssueCodeRequired] = original
		} else {
			delete(templates, zconst.IssueCodeRequired)
		}
	})

	m := &scripted{rules: RuleSet{ScenarioInsert: z.Struct(z.Shape{
		"name": z.String().Required(),
	})}}
	errs := Validate(context.Background(), m, ScenarioInsert)
	if errs == nil {
		t.Fatal("Validate = nil, want the required message")
	}
	got := errs.Fields["name"]
	if len(got) != 1 || !strings.Contains(got[0], "不能为空") {
		t.Errorf(`Fields["name"] = %q, want the overridden template rendered`, got)
	}
}

func TestStructLevelIssuesLandUnderRootKey(t *testing.T) {
	m := &scripted{Name: "x", rules: RuleSet{ScenarioInsert: z.Struct(z.Shape{
		"name": z.String().Required(z.Message("name cannot be blank")),
	}).TestFunc(func(val any, ctx z.Ctx) bool { return false }, z.Message("record as a whole is off"))}}
	errs := Validate(context.Background(), m, ScenarioInsert)
	if errs == nil {
		t.Fatal("Validate = nil, want the root-level message")
	}
	if got := errs.Fields["$root"]; !equalMessages(got, []string{"record as a whole is off"}) {
		t.Errorf(`Fields["$root"] = %q, want the struct-level message`, got)
	}
}

func TestErrorsErrorString(t *testing.T) {
	errs := &Errors{
		Scenario: ScenarioInsert,
		Fields:   map[string][]string{"email": {"bad"}, "name": {"blank"}},
	}
	got := errs.Error()
	for _, want := range []string{"insert", "email: bad", "name: blank"} {
		if !strings.Contains(got, want) {
			t.Errorf("Error() = %q, want it to contain %q", got, want)
		}
	}
	// Fields render in sorted order: email precedes name regardless of map
	// iteration order.
	if strings.Index(got, "email: bad") > strings.Index(got, "name: blank") {
		t.Errorf("Error() = %q, want email before name", got)
	}
}

func equalMessages(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
