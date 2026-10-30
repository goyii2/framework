package web_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/goyii2/framework/base"
	"github.com/goyii2/framework/web"
)

// checkerStub doubles an AccessChecker: a fixed per-user holdings table, a
// record of what it was asked, and an optional failure to inject.
type checkerStub struct {
	held   map[string][]string
	asked  []string
	params []map[string]any
	err    error
}

func (s *checkerStub) CheckAccess(userID string, itemName string, params map[string]any) (bool, error) {
	if s.err != nil {
		return false, s.err
	}
	s.asked = append(s.asked, userID+"/"+itemName)
	s.params = append(s.params, params)
	return slices.Contains(s.held[userID], itemName), nil
}

// authzApp assembles an application whose /gate route authenticates through
// basic auth and answers what User.Can reports; checker is installed as the
// authorization backend.
func authzApp(t *testing.T, checker web.AccessChecker) *web.Handler {
	t.Helper()
	app := base.New()
	h := web.NewHandler(app, web.WithoutCsrf())
	h.SetAuthManager(checker)
	h.Use(web.HttpBasic{Authenticate: staticBasicAuth()}.Middleware())

	h.Get("/gate", func(c *web.Context) error {
		return c.JSON(map[string]any{
			"can":  c.User().Can("post/delete"),
			"can2": c.User().CanWithParams("post/delete", map[string]any{"post": 42}),
		})
	})
	return h
}

func TestUserCanAsksTheAuthManager(t *testing.T) {
	checker := &checkerStub{held: map[string][]string{
		alice.GetID(): {"post/delete"}, // alice's identity roles say editor only
	}}
	h := authzApp(t, checker)

	var body struct {
		Can  bool `json:"can"`
		Can2 bool `json:"can2"`
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/gate", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated request: status = %d, want 401 (basic auth rejects before Can)", rec.Code)
	}
	if len(checker.asked) != 0 {
		t.Errorf("the checker must not be consulted for guests, it saw %v", checker.asked)
	}

	rec = httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/gate", nil)
	r.Header.Set("Authorization", asAlice)
	h.ServeHTTP(rec, r)
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !body.Can || !body.Can2 {
		t.Errorf("alice holds post/delete through the checker, got can=%v can2=%v", body.Can, body.Can2)
	}
	if len(checker.params) != 2 || checker.params[1]["post"] != 42 {
		t.Errorf("the checker must receive the check params, saw %v", checker.params)
	}

	// bob authenticates but holds nothing per the checker.
	rec = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodGet, "/gate", nil)
	r.Header.Set("Authorization", asBob)
	h.ServeHTTP(rec, r)
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Can {
		t.Error("bob holds nothing per the checker, Can must deny")
	}
}

func TestUserCanWithoutCheckerDenies(t *testing.T) {
	app := base.New()
	h := web.NewHandler(app, web.WithoutCsrf()) // no SetAuthManager
	h.Get("/gate", func(c *web.Context) error {
		return c.JSON(map[string]bool{"can": c.User().Can("admin")})
	})

	var body struct {
		Can bool `json:"can"`
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/gate", nil))
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Can {
		t.Error("without an authorization backend Can must deny (not panic)")
	}
}

// gatekeeper is the AuthManager-backed access control shape: the rule names
// a permission the identity's own roles know nothing about.
type gatekeeper struct {
	web.Controller
	Auth web.AccessChecker
}

func (g gatekeeper) Middleware() []web.Middleware {
	return []web.Middleware{
		web.HttpBasic{Authenticate: staticBasicAuth()}.Middleware(),
		web.AccessControl{
			Auth: g.Auth,
			Rules: []web.AccessRule{
				{Allow: true, Roles: []string{"post/delete"}},
			},
		}.Middleware(),
	}
}

func (gatekeeper) ActionPurge(c *web.Context) error {
	return c.JSON(map[string]string{"purged": "yes"})
}

func TestAccessControlChecksRolesThroughAuthManager(t *testing.T) {
	checker := &checkerStub{held: map[string][]string{
		alice.GetID(): {"post/delete"},
		// bob's identity role is admin, but the backend says he holds
		// nothing: with a checker installed, the backend decides.
	}}
	app := base.New()
	h := web.NewHandler(app, web.WithoutCsrf())
	g := gatekeeper{Auth: checker}
	h.Ctrl(g).Post("/gate/purge", g.ActionPurge)

	req := func(auth string) int {
		r := httptest.NewRequest(http.MethodPost, "/gate/purge", nil)
		if auth != "" {
			r.Header.Set("Authorization", auth)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		return rec.Code
	}
	if code := req(asAlice); code != http.StatusOK {
		t.Errorf("alice holds post/delete per the backend: status = %d, want 200", code)
	}
	if code := req(asBob); code != http.StatusForbidden {
		t.Errorf("bob holds nothing per the backend: status = %d, want 403", code)
	}
	if code := req(""); code != http.StatusUnauthorized {
		t.Errorf("guest: status = %d, want 401 from the authenticator", code)
	}
}

func TestAccessControlAuthBackendKeepsSpecialRoles(t *testing.T) {
	// "?" and "@" keep their guest/authenticated meaning with a checker
	// installed: sessions authenticate, the backend plays no part in it.
	checker := &checkerStub{held: map[string][]string{}}
	app := base.New()
	app.SetSession(fileSessionManager(t))
	h := web.NewHandler(app, web.WithoutCsrf())
	h.SetAuthManager(checker)
	h.SessionManager().IdentityFinder = func(id string) (web.Identity, bool) {
		if id == alice.GetID() {
			return alice, true
		}
		return nil, false
	}
	h.Post("/login", func(c *web.Context) error {
		return c.User().Login(alice)
	})
	h.Ctrl(lobbyController{}).Get("/lobby", lobbyController{}.ActionIndex)
	h.Ctrl(memberController{}).Get("/member/desk", memberController{}.ActionDesk)

	ck := sessionCookieOf(t, sessionReq(t, h, http.MethodPost, "/login"), "goyii-session")
	if ck == nil {
		t.Fatal("login issued no session cookie")
	}
	authed := func(target string) int {
		r := httptest.NewRequest(http.MethodGet, target, nil)
		r.AddCookie(ck)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		return rec.Code
	}

	if code := accessReq(t, h, http.MethodGet, "/lobby", ""); code != http.StatusOK {
		t.Errorf("guest lobby: status = %d, want 200 (\"?\" still matches guests)", code)
	}
	if code := authed("/lobby"); code != http.StatusForbidden {
		t.Errorf("authenticated lobby: status = %d, want 403 (\"@\" rule denies)", code)
	}
	if code := authed("/member/desk"); code != http.StatusOK {
		t.Errorf("authenticated desk: status = %d, want 200 (\"@\" rule allows)", code)
	}
	if len(checker.asked) != 0 {
		t.Errorf("special roles never consult the backend, it saw %v", checker.asked)
	}
}

func TestAccessControlAuthBackendFailureIsServerError(t *testing.T) {
	checker := &checkerStub{err: errors.New("rule registry misaligned")}
	app := base.New()
	h := web.NewHandler(app, web.WithoutCsrf())
	g := gatekeeper{Auth: checker}
	h.Ctrl(g).Post("/gate/purge", g.ActionPurge)

	r := httptest.NewRequest(http.MethodPost, "/gate/purge", nil)
	r.Header.Set("Authorization", asAlice)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("backend failure: status = %d, want 500 (a fault, not a denial)", rec.Code)
	}
}
