package validators

import (
	"context"
	"testing"
	"testing/fstest"

	z "github.com/Oudwins/zog"

	"github.com/goyii2/framework/i18n"
)

// translatedFixture translates the fixture model's messages into Chinese.
func translatedFixture() *i18n.MessageSource {
	return i18n.NewMessageSource(fstest.MapFS{
		"zh-CN/app.json": &fstest.MapFile{Data: []byte(`{
			"name cannot be blank": "名称不能为空",
			"email is not a valid address": "邮箱不是有效地址"
		}`)},
	})
}

func translatedCtx(lang string) context.Context {
	return i18n.WithLanguage(i18n.WithSource(context.Background(), translatedFixture()), lang)
}

// TestValidateTranslatesMessages proves the i18n linkage: messages produced
// by the rules are ids, translated per the request language, with the
// field -> []messages shape intact.
func TestValidateTranslatesMessages(t *testing.T) {
	m := &member{Name: "", Email: "not-an-email"}
	errs := Validate(translatedCtx("zh-CN"), m, ScenarioInsert)
	if errs == nil {
		t.Fatal("Validate = nil, want field errors")
	}
	want := map[string][]string{
		"name":  {"名称不能为空"},
		"email": {"邮箱不是有效地址"},
	}
	if len(errs.Fields) != len(want) {
		t.Fatalf("Fields = %v, want exactly name and email", errs.Fields)
	}
	for field, messages := range want {
		if got := errs.Fields[field]; len(got) != 1 || got[0] != messages[0] {
			t.Errorf("Fields[%s] = %q, want %q", field, got, messages)
		}
	}
}

// TestValidateKeepsUntranslatedMessages: ids without a translation entry
// stay verbatim, so partial dictionaries degrade gracefully.
func TestValidateKeepsUntranslatedMessages(t *testing.T) {
	schema := &scripted{Age: 5, rules: RuleSet{ScenarioInsert: z.Struct(z.Shape{
		"age": z.Int().GT(10, z.Message("age must exceed 10")),
	})}}
	errs := Validate(translatedCtx("zh-CN"), schema, ScenarioInsert)
	if errs == nil {
		t.Fatal("Validate = nil, want the age message")
	}
	if got := errs.Fields["age"]; len(got) != 1 || got[0] != "age must exceed 10" {
		t.Errorf(`Fields["age"] = %q, want the untranslated message`, got)
	}
}

// TestValidateWithoutSourceKeepsMessages: contexts without an i18n source
// (CLI, standalone tools) return the source-language text untouched.
func TestValidateWithoutSourceKeepsMessages(t *testing.T) {
	m := &member{Name: "", Email: "not-an-email"}
	for _, ctx := range []context.Context{nil, context.Background()} {
		errs := Validate(ctx, m, ScenarioInsert)
		if errs == nil {
			t.Fatalf("Validate(ctx=%v) = nil, want field errors", ctx)
		}
		if got := errs.Fields["name"]; len(got) != 1 || got[0] != "name cannot be blank" {
			t.Errorf(`Fields["name"] = %q, want the original message`, got)
		}
	}
}

// TestValidateOtherLanguageFallsBack: a request language with no
// translations renders the ids, exactly like Yii2 falling back to the source
// language.
func TestValidateOtherLanguageFallsBack(t *testing.T) {
	m := &member{Name: "", Email: "not-an-email"}
	errs := Validate(translatedCtx("fr-FR"), m, ScenarioInsert)
	if got := errs.Fields["name"]; len(got) != 1 || got[0] != "name cannot be blank" {
		t.Errorf(`Fields["name"] = %q, want the untranslated fallback`, got)
	}
}
