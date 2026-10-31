package form

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"testing/fstest"

	"github.com/goyii2/framework/i18n"
)

// translatedFixture translates the form package's parse messages into Chinese.
func translatedFixture() *i18n.MessageSource {
	return i18n.NewMessageSource(fstest.MapFS{
		"zh-CN/app.json": &fstest.MapFile{Data: []byte(`{
			"not a valid integer": "不是有效整数",
			"not a valid date-time": "不是有效日期时间",
			"malformed JSON body": "JSON 请求体格式错误"
		}`)},
	})
}

func translatedCtx(lang string) context.Context {
	return i18n.WithLanguage(i18n.WithSource(context.Background(), translatedFixture()), lang)
}

// TestTypeErrorsAreTranslated proves the i18n linkage: parse messages are ids,
// translated per the request's language like rule validation messages.
func TestTypeErrorsAreTranslated(t *testing.T) {
	req := formRequest(http.MethodPost, "/x", url.Values{
		"age":     {"abc"},
		"created": {"not-a-time"},
	}).WithContext(translatedCtx("zh-CN"))
	f := allTypes{}
	errs := Bind(req, &f)
	if errs == nil {
		t.Fatal("Bind = nil, want field errors")
	}
	if got := errs.Fields["age"]; len(got) != 1 || got[0] != "不是有效整数" {
		t.Errorf(`Fields["age"] = %q, want the translated message`, got)
	}
	if got := errs.Fields["created"]; len(got) != 1 || got[0] != "不是有效日期时间" {
		t.Errorf(`Fields["created"] = %q, want the translated message`, got)
	}
}

// TestTypeErrorsWithoutSourceStayVerbatim: requests without an i18n source
// keep the source-language text.
func TestTypeErrorsWithoutSourceStayVerbatim(t *testing.T) {
	req := formRequest(http.MethodPost, "/x", url.Values{"age": {"abc"}})
	f := allTypes{}
	errs := Bind(req, &f)
	if errs == nil {
		t.Fatal("Bind = nil, want field errors")
	}
	if got := errs.Fields["age"]; len(got) != 1 || got[0] != "not a valid integer" {
		t.Errorf(`Fields["age"] = %q, want the untranslated message`, got)
	}
}

// TestTypeErrorsOtherLanguageFallsBack: languages with no dictionary render
// the ids, like validators falling back to the source language.
func TestTypeErrorsOtherLanguageFallsBack(t *testing.T) {
	req := formRequest(http.MethodPost, "/x", url.Values{"age": {"abc"}}).WithContext(translatedCtx("fr-FR"))
	f := allTypes{}
	errs := Bind(req, &f)
	if errs == nil {
		t.Fatal("Bind = nil, want field errors")
	}
	if got := errs.Fields["age"]; len(got) != 1 || got[0] != "not a valid integer" {
		t.Errorf(`Fields["age"] = %q, want the untranslated fallback`, got)
	}
}

// TestQueryTypeErrorsAreTranslated: the query source runs the same
// translation path as the body source.
func TestQueryTypeErrorsAreTranslated(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/x?age=abc", nil).WithContext(translatedCtx("zh-CN"))
	f := allTypes{}
	errs := BindQuery(req, &f)
	if errs == nil {
		t.Fatal("BindQuery = nil, want field errors")
	}
	if got := errs.Fields["age"]; len(got) != 1 || got[0] != "不是有效整数" {
		t.Errorf(`Fields["age"] = %q, want the translated message`, got)
	}
}

// TestJSONTypeErrorsAreTranslated: the JSON source runs the same translation
// path as the form source.
func TestJSONTypeErrorsAreTranslated(t *testing.T) {
	req := jsonRequest(`{"age": "abc"}`).WithContext(translatedCtx("zh-CN"))
	f := allTypes{}
	errs := Bind(req, &f)
	if errs == nil {
		t.Fatal("Bind = nil, want field errors")
	}
	if got := errs.Fields["age"]; len(got) != 1 || got[0] != "不是有效整数" {
		t.Errorf(`Fields["age"] = %q, want the translated message`, got)
	}
}

// TestMalformedJSONBodyIsTranslated: the record-level body message is an id on
// the same translation path as field messages.
func TestMalformedJSONBodyIsTranslated(t *testing.T) {
	req := jsonRequest(`{"age": `).WithContext(translatedCtx("zh-CN"))
	f := allTypes{}
	errs := Bind(req, &f)
	if errs == nil {
		t.Fatal("Bind = nil, want a root error")
	}
	if got := errs.Fields["$root"]; len(got) != 1 || got[0] != "JSON 请求体格式错误" {
		t.Errorf(`Fields["$root"] = %q, want the translated message`, got)
	}
}
