package i18n_test

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	z "github.com/Oudwins/zog"

	"github.com/goyii2/framework/base"
	"github.com/goyii2/framework/config"
	"github.com/goyii2/framework/i18n"
	"github.com/goyii2/framework/validators"
	"github.com/goyii2/framework/web"
)

// The message tree travels inside the test binary exactly the way an
// application ships it: compiled in via go:embed, no files on disk.
//
//go:embed testdata/messages
var embedded embed.FS

func embeddedSource(t *testing.T) *i18n.MessageSource {
	t.Helper()
	tree, err := fs.Sub(embedded, "testdata/messages")
	if err != nil {
		t.Fatalf("sub-FS of the embedded tree: %v", err)
	}
	return i18n.NewMessageSource(tree)
}

func TestEmbeddedSourceTranslates(t *testing.T) {
	src := embeddedSource(t)
	if got := src.Languages(); !slices.Equal(got, []string{"de-DE", "zh", "zh-CN"}) {
		t.Fatalf("Languages() = %v, want the three embedded languages", got)
	}
	ctx := i18n.WithLanguage(i18n.WithSource(context.Background(), src), "zh-CN")
	if got := i18n.T(ctx, "app", "Hello, {name}!", map[string]any{"name": "Ada"}); got != "你好，Ada！" {
		t.Errorf("embedded T = %q, want the zh-CN translation", got)
	}
}

// form is the validated request model: its rule messages are the ids the
// app category translates.
type form struct {
	Name string
}

func (f *form) Rules() validators.RuleSet {
	return validators.RuleSet{
		validators.ScenarioInsert: z.Struct(z.Shape{
			"name": z.String().Required(z.Message("name cannot be blank")),
		}),
	}
}

// TestEmbeddedSourceServesTheRequestPipeline wires the embedded source into
// a real application and handler: actions translate through the request
// context, the language follows the preference chain (query parameter ->
// session -> Accept-Language -> default), a parameter-driven switch sticks
// to the session, and validation errors arrive translated in the
// field -> []messages shape.
func TestEmbeddedSourceServesTheRequestPipeline(t *testing.T) {
	app := base.New(config.Config{
		App:     config.App{Name: "i18n-test"},
		Web:     config.Web{DisableCsrf: true},
		Session: config.Session{Path: t.TempDir()},
	})
	app.SetI18n(embeddedSource(t))
	handler := web.NewHandler(app)

	handler.Get("/hello", func(c *web.Context) error {
		msg := i18n.T(c.Context(), "app", "Hello, {name}!", map[string]any{"name": "Ada"})
		price := i18n.F(c.Context()).Currency("USD", 1234.5)
		return c.JSON(map[string]string{"msg": msg, "price": price})
	})
	handler.Post("/form", func(c *web.Context) error {
		verrs := validators.Validate(c.Context(), &form{}, validators.ScenarioInsert)
		return c.JSONStatus(http.StatusUnprocessableEntity, verrs.Fields)
	})

	get := func(t *testing.T, target, accept, cookie string) (body map[string]string, fields map[string][]string, setCookie string) {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, target, nil)
		if strings.Contains(target, "/form") {
			req = httptest.NewRequest(http.MethodPost, target, nil)
		}
		if accept != "" {
			req.Header.Set("Accept-Language", accept)
		}
		if cookie != "" {
			req.Header.Set("Cookie", cookie)
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if cs := rec.Header().Values("Set-Cookie"); len(cs) > 0 {
			setCookie = strings.Join(cs, "; ")
		}
		data := rec.Body.Bytes()
		if strings.Contains(target, "/form") {
			if err := json.Unmarshal(data, &fields); err != nil {
				t.Fatalf("decode %q: %v", data, err)
			}
			return
		}
		if err := json.Unmarshal(data, &body); err != nil {
			t.Fatalf("decode %q: %v", data, err)
		}
		return
	}

	// No signals: the application default (source language) renders.
	body, _, _ := get(t, "/hello", "", "")
	if body["msg"] != "Hello, Ada!" || body["price"] != "$ 1,234.50" {
		t.Errorf("default-language reply = %v, want the en-US message and price", body)
	}

	// Accept-Language alone steers the language; zh-CN is listed first so it
	// resolves to the zh-CN directory (a bare "zh" would resolve to the zh
	// parent directory, which carries no hello entry and falls back).
	body, _, _ = get(t, "/hello", "zh-CN,zh;q=0.8,en;q=0.5", "")
	if body["msg"] != "你好，Ada！" || body["price"] != "US$ 1,234.50" {
		t.Errorf("zh reply = %v, want the zh-CN translation and price", body)
	}

	// The query parameter wins over the header and persists in the session.
	body, _, cookie := get(t, "/hello?lang=de-DE", "zh", "")
	if !strings.Contains(cookie, "goyii-session=") {
		t.Fatalf("switching via the parameter issued no session cookie: %q", cookie)
	}
	if body["msg"] != "Hallo, Ada！" {
		t.Errorf("de-DE reply = %v, want the German translation", body)
	}
	session := sessionCookieValue(t, cookie)

	// Later requests without the parameter keep the session's choice, the
	// browser header notwithstanding.
	body, _, _ = get(t, "/hello", "zh", session)
	if body["msg"] != "Hallo, Ada！" {
		t.Errorf("sticky session reply = %v, want the session's de-DE to outlast the zh header", body)
	}

	// Validation errors translate through the same request context and keep
	// the field -> []messages shape.
	_, fields, _ := get(t, "/form", "zh-CN,zh;q=0.9", "")
	want := map[string][]string{"name": {"名称不能为空"}}
	if !sameFields(fields, want) {
		t.Errorf("translated 422 body = %v, want %v", fields, want)
	}
	_, fields, _ = get(t, "/form", "", "")
	want = map[string][]string{"name": {"name cannot be blank"}}
	if !sameFields(fields, want) {
		t.Errorf("untranslated 422 body = %v, want %v", fields, want)
	}

	// The rejected record check: validation with a foreign-language context
	// still reports failure (non-nil Fields) — shape over language.
	if verrs := validators.Validate(i18n.WithLanguage(
		i18n.WithSource(context.Background(), embeddedSource(t)), "zh-CN"),
		&form{}, validators.ScenarioInsert); len(verrs.Fields) == 0 {
		t.Error("translated validation unexpectedly passed a blank record")
	}

	// Errors.As compatibility of the translated result stays intact.
	err := error(validators.Validate(i18n.WithLanguage(
		i18n.WithSource(context.Background(), embeddedSource(t)), "zh-CN"),
		&form{}, validators.ScenarioInsert))
	var verrs *validators.Errors
	if !errors.As(err, &verrs) {
		t.Fatal("translated failure does not unwrap to *validators.Errors")
	}
}

func sessionCookieValue(t *testing.T, setCookie string) string {
	t.Helper()
	for _, part := range strings.Split(setCookie, ";") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(part), "goyii-session="); ok {
			return "goyii-session=" + v
		}
	}
	t.Fatalf("no session cookie in %q", setCookie)
	return ""
}

func sameFields(got, want map[string][]string) bool {
	if len(got) != len(want) {
		return false
	}
	for field, messages := range want {
		if !slices.Equal(got[field], messages) {
			return false
		}
	}
	return true
}
