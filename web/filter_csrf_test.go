package web_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/goyii2/framework/base"
	"github.com/goyii2/framework/web"
)

// csrfApp builds a default-configuration application (CSRF on, the
// documented default) exposing GET /form (token echo) and POST /submit.
func csrfApp() *web.Handler {
	app := base.New()
	h := web.NewHandler(app)
	h.Get("/form", func(c *web.Context) error {
		return c.JSON(map[string]string{"token": c.CsrfToken()})
	})
	h.Post("/submit", func(c *web.Context) error {
		return c.JSONStatus(http.StatusCreated, map[string]string{"ok": "yes"})
	})
	return h
}

// fetchCsrfCookie requests /form and returns the issued cookie value.
func fetchCsrfCookie(t *testing.T, h *web.Handler) string {
	t.Helper()
	res := req(t, h, http.MethodGet, "/form")
	for _, ck := range res.Cookies() {
		if ck.Name == "_csrf" {
			return ck.Value
		}
	}
	t.Fatal("GET /form issued no _csrf cookie")
	return ""
}

// csrfPost performs POST /submit carrying a cookie and optionally a token
// in the header or in the form field.
func csrfPost(t *testing.T, h *web.Handler, cookieToken, headerToken, formToken string) *http.Response {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "/submit", nil)
	if formToken != "" {
		r = httptest.NewRequest(http.MethodPost, "/submit", strings.NewReader("_csrf="+formToken))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if cookieToken != "" {
		r.AddCookie(&http.Cookie{Name: "_csrf", Value: cookieToken})
	}
	if headerToken != "" {
		r.Header.Set("X-Csrf-Token", headerToken)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec.Result()
}

func TestCsrfSafeRequestIssuesCookieAndExposesToken(t *testing.T) {
	h := csrfApp()

	// One request carries both the issued cookie and the action's token:
	// they must match for the double-submit pattern to close.
	res := req(t, h, http.MethodGet, "/form")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	var body map[string]string
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("decoding body: %v", err)
	}
	var token string
	for _, ck := range res.Cookies() {
		if ck.Name == "_csrf" {
			token = ck.Value
		}
	}
	if token == "" {
		t.Fatal("GET /form issued no _csrf cookie")
	}
	if body["token"] != token {
		t.Errorf("action token = %q, cookie token = %q, want identical", body["token"], token)
	}

	// A follow-up safe request with the cookie keeps the same token.
	r := httptest.NewRequest(http.MethodGet, "/form", nil)
	r.AddCookie(&http.Cookie{Name: "_csrf", Value: token})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	body = nil
	if err := json.NewDecoder(rec.Result().Body).Decode(&body); err != nil {
		t.Fatalf("decoding body: %v", err)
	}
	if body["token"] != token {
		t.Errorf("token changed between safe requests: %q then %q", token, body["token"])
	}
}

func TestCsrfRejectsUnsafeRequestWithoutToken(t *testing.T) {
	h := csrfApp()

	res := csrfPost(t, h, "", "", "")
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for a POST without any token", res.StatusCode)
	}
}

func TestCsrfRejectsUnsafeRequestWithCookieOnly(t *testing.T) {
	h := csrfApp()
	token := fetchCsrfCookie(t, h)

	res := csrfPost(t, h, token, "", "")
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (a valid cookie without a submitted token)", res.StatusCode)
	}
}

func TestCsrfRejectsMismatchedToken(t *testing.T) {
	h := csrfApp()
	token := fetchCsrfCookie(t, h)

	res := csrfPost(t, h, token, "forged-token-attempt", "")
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for a mismatched token", res.StatusCode)
	}
}

func TestCsrfAcceptsMatchingHeaderToken(t *testing.T) {
	h := csrfApp()
	token := fetchCsrfCookie(t, h)

	res := csrfPost(t, h, token, token, "")
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201 for a matching double submit", res.StatusCode)
	}
}

func TestCsrfAcceptsMatchingFormFieldToken(t *testing.T) {
	h := csrfApp()
	token := fetchCsrfCookie(t, h)

	res := csrfPost(t, h, token, "", token)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201 for a form-field double submit", res.StatusCode)
	}
}

func TestCsrfQueryTokenIsNotAccepted(t *testing.T) {
	h := csrfApp()
	token := fetchCsrfCookie(t, h)

	r := httptest.NewRequest(http.MethodPost, "/submit?_csrf="+token, nil)
	r.AddCookie(&http.Cookie{Name: "_csrf", Value: token})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (query-string tokens must not count)", rec.Code)
	}
}

func TestCsrfDisabledGloballyByConfiguration(t *testing.T) {
	app := base.New()
	h := web.NewHandler(app, web.WithoutCsrf())
	h.Post("/submit", func(c *web.Context) error {
		return c.JSONStatus(http.StatusCreated, map[string]string{"ok": "yes"})
	})

	r := httptest.NewRequest(http.MethodPost, "/submit", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 with CSRF validation disabled", rec.Code)
	}
}

func TestCsrfDisabledFlagSkipsHandMountedFilter(t *testing.T) {
	// The Disabled flag is for applications that mount the filter by hand
	// (global default off) and want one chain branch without validation.
	app := base.New()
	h := web.NewHandler(app, web.WithoutCsrf())
	h.Use(web.Csrf{Disabled: true}.Middleware())
	h.Post("/submit", func(c *web.Context) error {
		return c.JSONStatus(http.StatusCreated, map[string]string{"ok": "yes"})
	})

	r := httptest.NewRequest(http.MethodPost, "/submit", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (Disabled flag skips validation)", rec.Code)
	}
}
