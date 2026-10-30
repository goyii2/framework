package web_test

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/goyii2/framework/base"
	"github.com/goyii2/framework/web"
)

// basicController declares the HTTP basic authenticator as its middleware:
// every action mounted through it requires credentials.
type basicController struct {
	web.Controller
}

func (basicController) Middleware() []web.Middleware {
	return []web.Middleware{
		web.HttpBasic{Realm: "api", Authenticate: staticBasicAuth()}.Middleware(),
	}
}

func (basicController) ActionProfile(c *web.Context) error {
	return c.JSON(map[string]any{"user": c.User().Identity().GetName(), "guest": c.User().IsGuest()})
}

// staticBasicAuth is the application-side credential check shared by the
// auth and access-control tests: two known pairs, compared in constant time.
func staticBasicAuth() func(username, password string) (web.Identity, bool) {
	users := map[string]string{"alice": "secret-1", "bob": "secret-1"}
	identities := map[string]web.Identity{"alice": alice, "bob": bob}
	return func(username, password string) (web.Identity, bool) {
		want, known := users[username]
		if !known || !web.ConstantTimeEqual(password, want) {
			return nil, false
		}
		return identities[username], true
	}
}

// bearerController declares the bearer authenticator (backed by a static
// token table) as its middleware.
type bearerController struct {
	web.Controller
}

func (bearerController) Middleware() []web.Middleware {
	table := web.StaticBearerTokens(map[string]web.Identity{
		"token-alice": alice,
		"token-bob":   bob,
	})
	return []web.Middleware{web.HttpBearer{Realm: "api", Authenticate: table}.Middleware()}
}

func (bearerController) ActionOrders(c *web.Context) error {
	return c.JSON(map[string]any{"orders": []string{"o1"}, "user": c.User().Identity().GetName()})
}

// authApp mounts the basic controller at /basic, the bearer controller at
// /api and an unprotected action: the authenticators guard their own
// controllers' actions only.
func authApp(t *testing.T) *web.Handler {
	t.Helper()
	app := base.New()
	h := web.NewHandler(app, web.WithoutCsrf())
	h.Ctrl(basicController{}).Get("/basic/profile", basicController{}.ActionProfile)
	h.Ctrl(bearerController{}).Get("/api/orders", bearerController{}.ActionOrders)
	h.Get("/public/ping", func(c *web.Context) error {
		return c.JSON(map[string]string{"pong": "yes"})
	})
	return h
}

// authGet performs GET against h with an optional raw Authorization header
// value (left out when empty).
func authGet(t *testing.T, h http.Handler, target, authorization string) *http.Response {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, target, nil)
	if authorization != "" {
		r.Header.Set("Authorization", authorization)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec.Result()
}

// basicAuthHeader builds a Basic Authorization header value.
func basicAuthHeader(username, password string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(username+":"+password))
}

// bodyOf decodes a JSON object body into map[string]any.
func bodyOf(t *testing.T, res *http.Response) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("decoding body: %v", err)
	}
	return body
}

func TestHttpBasicRejectsMissingCredentials(t *testing.T) {
	h := authApp(t)

	res := authGet(t, h, "/basic/profile", "")
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 without credentials", res.StatusCode)
	}
	if got := res.Header.Get("WWW-Authenticate"); got != `Basic realm="api", charset="UTF-8"` {
		t.Fatalf("WWW-Authenticate = %q, want a basic challenge for realm api", got)
	}
	if body := bodyOf(t, res); body["status"] != float64(http.StatusUnauthorized) {
		t.Fatalf("unified error body missing: %v", body)
	}
}

func TestHttpBasicRejectsWrongCredentials(t *testing.T) {
	h := authApp(t)

	res := authGet(t, h, "/basic/profile", basicAuthHeader("alice", "wrong"))
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 for a wrong password", res.StatusCode)
	}
	res = authGet(t, h, "/basic/profile", basicAuthHeader("mallory", "secret-1"))
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 for an unknown user", res.StatusCode)
	}
	if got := res.Header.Get("WWW-Authenticate"); got == "" {
		t.Fatal("rejection must carry a WWW-Authenticate challenge")
	}
}

func TestHttpBasicRejectsMalformedHeader(t *testing.T) {
	h := authApp(t)

	for _, header := range []string{
		"Basic !!!not-base64!!!",
		"Bearer token-alice", // right token, wrong scheme
		"Basic",
	} {
		res := authGet(t, h, "/basic/profile", header)
		if res.StatusCode != http.StatusUnauthorized {
			t.Fatalf("Authorization %q: status = %d, want 401", header, res.StatusCode)
		}
	}
}

func TestHttpBasicAcceptsAndBindsIdentity(t *testing.T) {
	h := authApp(t)

	res := authGet(t, h, "/basic/profile", basicAuthHeader("alice", "secret-1"))
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 with valid credentials", res.StatusCode)
	}
	body := bodyOf(t, res)
	if body["user"] != "alice" || body["guest"] != false {
		t.Fatalf("identity not bound: %v", body)
	}

	// The authenticator guards only its own controller: the unprotected
	// action stays open.
	if res := authGet(t, h, "/public/ping", ""); res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d on the unprotected action, want 200", res.StatusCode)
	}
}

func TestHttpBearerRejectsMissingToken(t *testing.T) {
	h := authApp(t)

	res := authGet(t, h, "/api/orders", "")
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 without a token", res.StatusCode)
	}
	if got := res.Header.Get("WWW-Authenticate"); got != `Bearer realm="api"` {
		t.Fatalf("WWW-Authenticate = %q, want a bearer challenge for realm api", got)
	}
}

func TestHttpBearerRejectsUnknownToken(t *testing.T) {
	h := authApp(t)

	for _, header := range []string{
		"Bearer token-mallory",
		"Bearer ",        // scheme without a token
		"Basic dXNlcjpw", // wrong scheme entirely
	} {
		res := authGet(t, h, "/api/orders", header)
		if res.StatusCode != http.StatusUnauthorized {
			t.Fatalf("Authorization %q: status = %d, want 401", header, res.StatusCode)
		}
	}
}

func TestHttpBearerStaticTableAcceptsKnownTokens(t *testing.T) {
	h := authApp(t)

	for token, want := range map[string]string{
		"token-alice": "alice",
		"token-bob":   "bob",
	} {
		res := authGet(t, h, "/api/orders", "Bearer "+token)
		if res.StatusCode != http.StatusOK {
			t.Fatalf("token %q: status = %d, want 200", token, res.StatusCode)
		}
		if body := bodyOf(t, res); body["user"] != want {
			t.Fatalf("token %q resolved to %v, want %s", token, body["user"], want)
		}
	}

	// The bearer scheme keyword is case-insensitive per the Authorization
	// scheme grammar.
	if res := authGet(t, h, "/api/orders", "bearer token-alice"); res.StatusCode != http.StatusOK {
		t.Fatalf("lowercase scheme: status = %d, want 200", res.StatusCode)
	}
}

func TestHttpBearerCustomValidatorSeam(t *testing.T) {
	// Applications resolve tokens their own way: a custom validator replaces
	// the static table without the middleware changing.
	var seen string
	app := base.New()
	h := web.NewHandler(app, web.WithoutCsrf())
	h.Use(web.HttpBearer{
		Authenticate: func(token string) (web.Identity, bool) {
			seen = token
			if token == "custom-token" {
				return bob, true
			}
			return nil, false
		},
	}.Middleware())
	h.Get("/seam", func(c *web.Context) error {
		return c.JSON(map[string]string{"user": c.User().Identity().GetName()})
	})

	res := authGet(t, h, "/seam", "Bearer custom-token")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 through the custom validator", res.StatusCode)
	}
	if seen != "custom-token" {
		t.Fatalf("validator saw %q, want custom-token", seen)
	}
	if body := bodyOf(t, res); body["user"] != "bob" {
		t.Fatalf("user = %v, want bob", body["user"])
	}
}

func TestConstantTimeEqual(t *testing.T) {
	if !web.ConstantTimeEqual("secret", "secret") {
		t.Fatal("equal strings must compare equal")
	}
	if web.ConstantTimeEqual("secret", "secreT") || web.ConstantTimeEqual("secret", "secret!") {
		t.Fatal("different strings must compare unequal")
	}
	if web.ConstantTimeEqual("", "") {
		t.Fatal("empty strings carry no secret; comparing them must not claim equality")
	}
}
