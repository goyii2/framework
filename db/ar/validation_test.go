package ar

import (
	"context"
	"errors"
	"testing"
	"testing/fstest"

	z "github.com/Oudwins/zog"

	"github.com/goyii2/framework/i18n"
	"github.com/goyii2/framework/validators"
)

// regUser exercises scenario validation on the save chain: name is required
// only when inserting; email must be unique among OTHER rows on both
// scenarios, the record's own row excluded so updates keeping their own
// email pass.
type regUser struct {
	Model
	ID    int64  `gorm:"column:id;primaryKey"`
	Name  string `gorm:"column:name"`
	Email string `gorm:"column:email"`
}

func (regUser) TableName() string { return "reg_users" }

func (u *regUser) Rules() validators.RuleSet {
	uniqueEmail := func() *z.StringSchema[string] {
		return z.String().TestFunc(func(val *string, ctx z.Ctx) bool {
			query := New[regUser](u.Context()).Where("email = ?", *val)
			if u.ID != 0 {
				// Exclude the row being updated: keeping an email already
				// stored on this very record must stay valid.
				query = query.Where("id <> ?", u.ID)
			}
			n, err := query.Count()
			return err == nil && n == 0
		}, z.Message("email is already taken"))
	}
	return validators.RuleSet{
		validators.ScenarioInsert: z.Struct(z.Shape{
			"name":  z.String().Required(z.Message("name cannot be blank")),
			"email": uniqueEmail(),
		}),
		validators.ScenarioUpdate: z.Struct(z.Shape{
			"email": uniqueEmail(),
		}),
	}
}

// guardedUser has rules for insert only: its updates fall outside every rule
// set and therefore pass, the Yii2 semantic of an empty scenario rule set.
type guardedUser struct {
	Model
	ID   int64  `gorm:"column:id;primaryKey"`
	Name string `gorm:"column:name"`
}

func (guardedUser) TableName() string { return "guarded_users" }

func (u *guardedUser) Rules() validators.RuleSet {
	return validators.RuleSet{
		validators.ScenarioInsert: z.Struct(z.Shape{
			"name": z.String().Required(z.Message("name cannot be blank")),
		}),
	}
}

// newValidationContext prepares the validation models' tables on top of the
// shared in-memory database context.
func newValidationContext(t *testing.T) context.Context {
	t.Helper()
	ctx := newTestContext(t)
	for _, ddl := range []string{
		"CREATE TABLE reg_users (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL, email TEXT NOT NULL DEFAULT '')",
		"CREATE TABLE guarded_users (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL)",
	} {
		if err := exec(ctx, ddl); err != nil {
			t.Fatalf("create table: %v", err)
		}
	}
	return ctx
}

func TestSaveRejectsInvalidInsertBeforeWriting(t *testing.T) {
	ctx := newValidationContext(t)

	u := New[regUser](ctx).New()
	u.Name = "" // required on insert
	u.Email = "ada@example.com"
	err := u.Save()

	var verrs *validators.Errors
	if !errors.As(err, &verrs) {
		t.Fatalf("Save error = %v, want a *validators.Errors", err)
	}
	if verrs.Scenario != validators.ScenarioInsert {
		t.Errorf("Scenario = %q, want %q", verrs.Scenario, validators.ScenarioInsert)
	}
	if got := verrs.Fields["name"]; len(got) != 1 || got[0] != "name cannot be blank" {
		t.Errorf(`Fields["name"] = %q, want the required message`, got)
	}
	// The rejection must happen before any statement: no row exists.
	if n, err := New[regUser](ctx).Count(); err != nil || n != 0 {
		t.Errorf("count after rejected insert = %d (err %v), want 0: an invalid record must not be written", n, err)
	}
}

func TestSaveValidatesInsertUniqueness(t *testing.T) {
	ctx := newValidationContext(t)

	first := New[regUser](ctx).New()
	first.Name, first.Email = "ada", "ada@example.com"
	if err := first.Save(); err != nil {
		t.Fatalf("insert first: %v", err)
	}

	second := New[regUser](ctx).New()
	second.Name, second.Email = "bob", "ada@example.com"
	err := second.Save()
	var verrs *validators.Errors
	if !errors.As(err, &verrs) {
		t.Fatalf("Save error = %v, want a *validators.Errors", err)
	}
	if got := verrs.Fields["email"]; len(got) != 1 || got[0] != "email is already taken" {
		t.Errorf(`Fields["email"] = %q, want the uniqueness message`, got)
	}
	if n, err := New[regUser](ctx).Count(); err != nil || n != 1 {
		t.Errorf("count = %d (err %v), want 1: only the first row may exist", n, err)
	}
}

func TestSaveValidatesUpdateScenarioWithSelfExclusion(t *testing.T) {
	ctx := newValidationContext(t)
	for _, row := range []struct {
		id          int64
		name, email string
	}{
		{1, "ada", "ada@example.com"},
		{2, "bob", "bob@example.com"},
	} {
		if err := exec(ctx, "INSERT INTO reg_users (id, name, email) VALUES (?, ?, ?)", row.id, row.name, row.email); err != nil {
			t.Fatalf("seed reg_users: %v", err)
		}
	}

	ada, err := FindByID[regUser](ctx, 1)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	// Keeping the own email is valid: the uniqueness rule excludes the row
	// being updated. Name may be blank here — required is insert-only.
	ada.Name, ada.Email = "", "ada@example.com"
	if err := ada.Save(); err != nil {
		t.Fatalf("update keeping own email: %v", err)
	}

	// Taking another row's email is rejected, and the stored row is left
	// untouched: the refusal happens before any SQL write.
	ada, err = FindByID[regUser](ctx, 1)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	ada.Email = "bob@example.com"
	err = ada.Save()
	var verrs *validators.Errors
	if !errors.As(err, &verrs) {
		t.Fatalf("Save error = %v, want a *validators.Errors", err)
	}
	if verrs.Scenario != validators.ScenarioUpdate {
		t.Errorf("Scenario = %q, want %q", verrs.Scenario, validators.ScenarioUpdate)
	}
	if got := verrs.Fields["email"]; len(got) != 1 || got[0] != "email is already taken" {
		t.Errorf(`Fields["email"] = %q, want the uniqueness message`, got)
	}
	stored, err := FindByID[regUser](ctx, 1)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if stored.Email != "ada@example.com" {
		t.Errorf("stored email = %q, want the original: a rejected update must not write", stored.Email)
	}
}

func TestSavePassesModelWithoutRules(t *testing.T) {
	ctx := newValidationContext(t)

	// user implements no Rules: the save chain skips validation entirely and
	// a blank name — invalid everywhere a rule would look — stores fine.
	u := New[user](ctx).New()
	u.Name = ""
	u.Email = ""
	if err := u.Save(); err != nil {
		t.Fatalf("Save(model without Rules) = %v, want nil", err)
	}
	if n, err := New[user](ctx).Count(); err != nil || n != 1 {
		t.Errorf("count = %d (err %v), want 1", n, err)
	}
}

func TestSavePassesScenarioWithoutSchema(t *testing.T) {
	ctx := newValidationContext(t)

	if err := exec(ctx, "INSERT INTO guarded_users (id, name) VALUES (1, 'seed')"); err != nil {
		t.Fatalf("seed guarded_users: %v", err)
	}
	loaded, err := FindByID[guardedUser](ctx, 1)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	// No update schema exists, so the update scenario validates nothing.
	loaded.Name = ""
	if err := loaded.Save(); err != nil {
		t.Fatalf("Save(scenario without schema) = %v, want nil", err)
	}
}

func TestModelValidateReturnsFieldMap(t *testing.T) {
	ctx := newValidationContext(t)

	u := New[regUser](ctx).New()
	u.Name = ""
	u.Email = "ada@example.com"
	got := u.Validate(validators.ScenarioInsert)
	if len(got) != 1 {
		t.Fatalf("Validate map = %v, want exactly the name field", got)
	}
	if msgs := got["name"]; len(msgs) != 1 || msgs[0] != "name cannot be blank" {
		t.Errorf(`Validate["name"] = %q, want the required message`, msgs)
	}

	valid := New[regUser](ctx).New()
	valid.Name, valid.Email = "ada", "ada@example.com"
	if m := valid.Validate(validators.ScenarioInsert); m != nil {
		t.Errorf("Validate(valid record) = %v, want nil", m)
	}
}

// TestSaveTranslatesValidationMessagesForRequestLanguage proves the save
// chain translation: the record's bound context carries the request's
// language, and refused writes report their messages translated while
// keeping the field -> []messages shape.
func TestSaveTranslatesValidationMessagesForRequestLanguage(t *testing.T) {
	base := newValidationContext(t)
	messages := fstest.MapFS{
		"zh-CN/app.json": &fstest.MapFile{Data: []byte(`{"name cannot be blank": "名称不能为空"}`)},
	}
	ctx := i18n.WithLanguage(i18n.WithSource(base, i18n.NewMessageSource(messages)), "zh-CN")

	u := New[regUser](ctx).New()
	u.Name = "" // required on insert
	u.Email = "ada@example.com"
	err := u.Save()

	var verrs *validators.Errors
	if !errors.As(err, &verrs) {
		t.Fatalf("Save error = %v, want a *validators.Errors", err)
	}
	if got := verrs.Fields["name"]; len(got) != 1 || got[0] != "名称不能为空" {
		t.Errorf(`Fields["name"] = %q, want the translated message`, got)
	}
	// The shape survives translation: one field, one message, nothing else.
	if len(verrs.Fields) != 1 {
		t.Errorf("Fields = %v, want only the name field after translation", verrs.Fields)
	}
	// The refusal still precedes any write.
	if n, err := New[regUser](ctx).Count(); err != nil || n != 0 {
		t.Errorf("count after rejected insert = %d (err %v), want 0", n, err)
	}
}
