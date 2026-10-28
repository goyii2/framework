package web_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/goyii2/framework/base"
	"github.com/goyii2/framework/config"
	"github.com/goyii2/framework/web"
)

// corsApp mounts the Cors filter globally over an action that records it
// ran; CSRF is off because CORS-focused tests do not carry form tokens.
func corsApp(t *testing.T, cors web.Cors, actionRan *bool) *web.Handler {
	t.Helper()
	app := base.New(config.Config{Web: config.Web{DisableCsrf: true}})
	h := web.NewHandler(app)
	h.Use(cors.Middleware())
	h.Get("/data", func(c *web.Context) error {
		*actionRan = true
		return c.JSON(map[string]string{"m": "data"})
	})
	return h
}

func TestCorsPreflightFromAllowedOriginAnswered204(t *testing.T) {
	var ran bool
	h := corsApp(t, web.Cors{
		Origins: []string{"https://app.example.com"},
		Methods: []string{"GET", "POST"},
		Headers: []string{"Content-Type", "X-Custom"},
		MaxAge:  600,
	}, &ran)

	r := httptest.NewRequest(http.MethodOptions, "/data", nil)
	r.Header.Set("Origin", "https://app.example.com")
	r.Header.Set("Access-Control-Request-Method", "POST")
	r.Header.Set("Access-Control-Request-Headers", "Content-Type")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	res := rec.Result()

	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", res.StatusCode)
	}
	if ran {
		t.Error("action ran for a preflight request")
	}
	for header, want := range map[string]string{
		"Access-Control-Allow-Origin":  "https://app.example.com",
		"Access-Control-Allow-Methods": "GET, POST",
		"Access-Control-Allow-Headers": "Content-Type, X-Custom",
		"Access-Control-Max-Age":       "600",
	} {
		if got := res.Header.Get(header); got != want {
			t.Errorf("%s = %q, want %q", header, got, want)
		}
	}
}

func TestCorsPreflightDisallowedMethodRejected403(t *testing.T) {
	var ran bool
	h := corsApp(t, web.Cors{
		Origins: []string{"*"},
		Methods: []string{"GET"},
	}, &ran)

	r := httptest.NewRequest(http.MethodOptions, "/data", nil)
	r.Header.Set("Origin", "https://evil.example.org")
	r.Header.Set("Access-Control-Request-Method", "DELETE")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	res := rec.Result()

	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", res.StatusCode)
	}
	if ran {
		t.Error("action ran for a rejected preflight")
	}
	if got := res.Header.Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Access-Control-Allow-Origin = %q, want none on a rejected preflight", got)
	}
}

func TestCorsPreflightDisallowedHeaderRejected403(t *testing.T) {
	var ran bool
	h := corsApp(t, web.Cors{
		Origins: []string{"*"},
		Headers: []string{"Content-Type"},
	}, &ran)

	r := httptest.NewRequest(http.MethodOptions, "/data", nil)
	r.Header.Set("Origin", "https://app.example.com")
	r.Header.Set("Access-Control-Request-Method", "GET")
	r.Header.Set("Access-Control-Request-Headers", "Content-Type, X-Bootleg")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestCorsPreflightDisallowedOriginRejected403(t *testing.T) {
	var ran bool
	h := corsApp(t, web.Cors{Origins: []string{"https://app.example.com"}}, &ran)

	r := httptest.NewRequest(http.MethodOptions, "/data", nil)
	r.Header.Set("Origin", "https://evil.example.org")
	r.Header.Set("Access-Control-Request-Method", "GET")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	res := rec.Result()

	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", res.StatusCode)
	}
	if got := res.Header.Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Access-Control-Allow-Origin = %q, want none for a disallowed origin", got)
	}
}

func TestCorsSimpleRequestFromAllowedOriginCarriesHeaders(t *testing.T) {
	var ran bool
	h := corsApp(t, web.Cors{
		Origins:       []string{"https://app.example.com"},
		ExposeHeaders: []string{"X-Request-Id"},
	}, &ran)

	r := httptest.NewRequest(http.MethodGet, "/data", nil)
	r.Header.Set("Origin", "https://app.example.com")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	res := rec.Result()

	if res.StatusCode != http.StatusOK || !ran {
		t.Fatalf("status = %d, ran = %v, want the action served", res.StatusCode, ran)
	}
	if got := res.Header.Get("Access-Control-Allow-Origin"); got != "https://app.example.com" {
		t.Errorf("Access-Control-Allow-Origin = %q, want the origin echoed", got)
	}
	if got := res.Header.Get("Access-Control-Expose-Headers"); got != "X-Request-Id" {
		t.Errorf("Access-Control-Expose-Headers = %q, want X-Request-Id", got)
	}
}

func TestCorsWildcardPolicyAnswersStarOrigin(t *testing.T) {
	var ran bool
	h := corsApp(t, web.Cors{}, &ran)

	r := httptest.NewRequest(http.MethodGet, "/data", nil)
	r.Header.Set("Origin", "https://anywhere.example.net")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	res := rec.Result()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	if got := res.Header.Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("Access-Control-Allow-Origin = %q, want *", got)
	}
	if got := res.Header.Get("Access-Control-Allow-Credentials"); got != "" {
		t.Errorf("Access-Control-Allow-Credentials = %q, want unset", got)
	}
}

func TestCorsCredentialsEchoOriginInsteadOfStar(t *testing.T) {
	var ran bool
	h := corsApp(t, web.Cors{Credentials: true}, &ran)

	r := httptest.NewRequest(http.MethodGet, "/data", nil)
	r.Header.Set("Origin", "https://app.example.com")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	res := rec.Result()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	if got := res.Header.Get("Access-Control-Allow-Origin"); got != "https://app.example.com" {
		t.Errorf("Access-Control-Allow-Origin = %q, want the origin (star is invalid with credentials)", got)
	}
	if got := res.Header.Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Errorf("Access-Control-Allow-Credentials = %q, want true", got)
	}
}

func TestCorsDisallowedOriginSimpleRequestServedWithoutCorsHeaders(t *testing.T) {
	var ran bool
	h := corsApp(t, web.Cors{Origins: []string{"https://app.example.com"}}, &ran)

	r := httptest.NewRequest(http.MethodGet, "/data", nil)
	r.Header.Set("Origin", "https://evil.example.org")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	res := rec.Result()

	if res.StatusCode != http.StatusOK || !ran {
		t.Fatalf("status = %d, ran = %v, want the action served", res.StatusCode, ran)
	}
	for _, header := range []string{"Access-Control-Allow-Origin", "Access-Control-Allow-Credentials"} {
		if got := res.Header.Get(header); got != "" {
			t.Errorf("%s = %q, want unset for a disallowed origin", header, got)
		}
	}
}

func TestCorsRequestWithoutOriginPassesUntouched(t *testing.T) {
	var ran bool
	h := corsApp(t, web.Cors{Origins: []string{"https://app.example.com"}}, &ran)

	res := req(t, h, http.MethodGet, "/data")
	if res.StatusCode != http.StatusOK || !ran {
		t.Fatalf("status = %d, ran = %v, want a normal same-origin request", res.StatusCode, ran)
	}
	if got := res.Header.Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Access-Control-Allow-Origin = %q, want unset without an Origin header", got)
	}
}

func TestCorsEmptyMethodsAndHeadersEchoPreflightRequest(t *testing.T) {
	var ran bool
	h := corsApp(t, web.Cors{}, &ran)

	r := httptest.NewRequest(http.MethodOptions, "/data", nil)
	r.Header.Set("Origin", "https://app.example.com")
	r.Header.Set("Access-Control-Request-Method", "PATCH")
	r.Header.Set("Access-Control-Request-Headers", "X-Whatever")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	res := rec.Result()

	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", res.StatusCode)
	}
	if got := res.Header.Get("Access-Control-Allow-Methods"); got != "PATCH" {
		t.Errorf("Access-Control-Allow-Methods = %q, want the requested PATCH echoed", got)
	}
	if got := res.Header.Get("Access-Control-Allow-Headers"); got != "X-Whatever" {
		t.Errorf("Access-Control-Allow-Headers = %q, want the requested header echoed", got)
	}
}
