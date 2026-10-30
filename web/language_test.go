package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/goyii2/framework/base"
	"github.com/goyii2/framework/i18n"
	"github.com/goyii2/framework/session"
)

// languageApp assembles a handler whose one route reports the language the
// i18n helpers resolve for the request, backed by a two-language source.
func languageApp(t *testing.T) *Handler {
	t.Helper()
	app := base.New()
	sess, err := session.New(session.Config{Path: t.TempDir()})
	if err != nil {
		t.Fatalf("session.New: %v", err)
	}
	app.SetSession(sess)
	app.SetI18n(i18n.NewMessageSource(fstest.MapFS{
		"zh-CN/app.json": &fstest.MapFile{Data: []byte(`{"Hello": "你好"}`)},
		"de-DE/app.json": &fstest.MapFile{Data: []byte(`{"Hello": "Hallo"}`)},
	}))
	h := NewHandler(app, WithoutCsrf())
	h.Get("/lang", func(c *Context) error {
		return c.JSON(map[string]string{"lang": i18n.T(c.Context(), "app", "Hello", nil)})
	})
	return h
}

func do(h *Handler, target, accept, cookie string) (body string, setCookie string) {
	req := httptest.NewRequest(http.MethodGet, target, nil)
	if accept != "" {
		req.Header.Set("Accept-Language", accept)
	}
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if cs := rec.Header().Values("Set-Cookie"); len(cs) > 0 {
		setCookie = strings.Join(cs, "; ")
	}
	return rec.Body.String(), setCookie
}

func TestLanguageMiddlewareResolvesFromHeader(t *testing.T) {
	h := languageApp(t)
	if body, _ := do(h, "/lang", "zh-CN", ""); !strings.Contains(body, "你好") {
		t.Errorf("header-resolved reply = %s, want the zh-CN translation", body)
	}
	if body, _ := do(h, "/lang", "de-DE", ""); !strings.Contains(body, "Hallo") {
		t.Errorf("header-resolved reply = %s, want the de-DE translation", body)
	}
	if body, _ := do(h, "/lang", "fr", ""); !strings.Contains(body, `"Hello"`) {
		t.Errorf("unknown-language reply = %s, want the untranslated id", body)
	}
}

func TestLanguageMiddlewareQueryParamOutranksHeader(t *testing.T) {
	h := languageApp(t)
	if body, _ := do(h, "/lang?lang=de-DE", "zh-CN", ""); !strings.Contains(body, "Hallo") {
		t.Errorf("param vs header reply = %s, want the parameter's de-DE to win", body)
	}
	// Unparseable parameters are ignored rather than switching anything.
	if body, _ := do(h, "/lang?lang=nonsense", "zh-CN", ""); !strings.Contains(body, "你好") {
		t.Errorf("nonsense param reply = %s, want the header language to still apply", body)
	}
}

func TestLanguageMiddlewareSessionStickiness(t *testing.T) {
	h := languageApp(t)

	// The switch request issues a session cookie...
	_, cookie := do(h, "/lang?lang=zh-CN", "de-DE", "")
	if !strings.Contains(cookie, "goyii-session=") {
		t.Fatalf("language switch issued no session cookie: %q", cookie)
	}
	session := ""
	for _, part := range strings.Split(cookie, ";") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(part), "goyii-session="); ok {
			session = "goyii-session=" + v
		}
	}
	if session == "" {
		t.Fatal("no session value found")
	}

	// ...and follow-up requests keep the stored choice over the header.
	if body, _ := do(h, "/lang", "de-DE", session); !strings.Contains(body, "你好") {
		t.Errorf("sticky reply = %s, want the session's zh-CN to beat the de-DE header", body)
	}
	// A later parameter can switch again.
	if body, _ := do(h, "/lang?lang=de-DE", "", session); !strings.Contains(body, "Hallo") {
		t.Errorf("re-switch reply = %s, want the new parameter language", body)
	}
}

func TestLanguageMiddlewareWithoutSessionStillResolves(t *testing.T) {
	// The middleware standalone — no session middleware in front of it: the
	// language still resolves from the parameter and nothing persists.
	app := base.New()
	app.SetI18n(i18n.NewMessageSource(fstest.MapFS{
		"zh-CN/app.json": &fstest.MapFile{Data: []byte(`{"Hello": "你好"}`)},
	}))
	translated := ""
	pipe := Language()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Downstream holders of the wrapped request resolve the language;
		// the request pipeline re-binds web.Context to each wrapped request
		// the same way.
		translated = i18n.T(r.Context(), "app", "Hello", nil)
	}))

	req := httptest.NewRequest(http.MethodGet, "/x?lang=zh-CN", nil)
	req = req.WithContext(app.WithContext(req.Context()))
	rec := httptest.NewRecorder()
	c := newContext(app, rec, req)
	pipe.ServeHTTP(rec, c.r)

	if translated != "你好" {
		t.Errorf("standalone middleware translated = %q, want the parameter language", translated)
	}
	if got := rec.Header().Values("Set-Cookie"); len(got) != 0 {
		t.Errorf("standalone middleware issued cookies: %v", got)
	}
}
