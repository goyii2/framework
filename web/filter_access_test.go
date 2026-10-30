package web_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/goyii2/framework/base"
	"github.com/goyii2/framework/web"
)

// adminController requires the "admin" role: basic authentication resolves
// the identity, access control checks its roles. alice holds editor, bob
// holds admin+editor.
type adminController struct {
	web.Controller
}

func (adminController) Middleware() []web.Middleware {
	return []web.Middleware{
		web.HttpBasic{Authenticate: staticBasicAuth()}.Middleware(),
		web.AccessControl{Rules: []web.AccessRule{
			{Allow: true, Roles: []string{"admin"}},
		}}.Middleware(),
	}
}

func (adminController) ActionPurge(c *web.Context) error {
	return c.JSON(map[string]string{"purged": "yes"})
}

// editorController requires the "editor" role, with a deny rule for editors
// listed first: the first matching rule wins, so an editor is denied even
// though a later rule would allow them.
type editorController struct {
	web.Controller
}

func (editorController) Middleware() []web.Middleware {
	return []web.Middleware{
		web.HttpBasic{Authenticate: staticBasicAuth()}.Middleware(),
		web.AccessControl{Rules: []web.AccessRule{
			{Allow: false, Roles: []string{"editor"}},
			{Allow: true, Roles: []string{"admin"}},
		}}.Middleware(),
	}
}

func (editorController) ActionDraft(c *web.Context) error {
	return c.JSON(map[string]string{"draft": "yes"})
}

// signoffController scopes its rule to GET: editors may read, only admins
// may write (the POST matches no rule and falls to the default deny).
type signoffController struct {
	web.Controller
}

func (signoffController) Middleware() []web.Middleware {
	return []web.Middleware{
		web.HttpBasic{Authenticate: staticBasicAuth()}.Middleware(),
		web.AccessControl{Rules: []web.AccessRule{
			{Allow: true, Verbs: []string{http.MethodGet}, Roles: []string{"editor"}},
			{Allow: true, Roles: []string{"admin"}},
		}}.Middleware(),
	}
}

func (signoffController) ActionView(c *web.Context) error {
	return c.JSON(map[string]string{"view": "yes"})
}

func (signoffController) ActionSign(c *web.Context) error {
	return c.JSON(map[string]string{"signed": "yes"})
}

// lobbyController uses the special guest/authenticated roles without any
// credential check: guests are welcomed in, authenticated requests are not.
type lobbyController struct {
	web.Controller
}

func (lobbyController) Middleware() []web.Middleware {
	return []web.Middleware{
		web.AccessControl{Rules: []web.AccessRule{
			{Allow: true, Roles: []string{"?"}},
			{Allow: false, Roles: []string{"@"}},
		}}.Middleware(),
	}
}

func (lobbyController) ActionIndex(c *web.Context) error {
	return c.JSON(map[string]string{"lobby": "yes"})
}

// memberController inverts the special roles: only authenticated requests
// pass (the guest rule denies them first).
type memberController struct {
	web.Controller
}

func (memberController) Middleware() []web.Middleware {
	return []web.Middleware{
		web.AccessControl{Rules: []web.AccessRule{
			{Allow: false, Roles: []string{"?"}},
			{Allow: true, Roles: []string{"@"}},
		}}.Middleware(),
	}
}

func (memberController) ActionDesk(c *web.Context) error {
	return c.JSON(map[string]string{"desk": "yes"})
}

// customRoleController replaces the role resolver entirely: roles come from
// the injected callback, not from the identity's own roles.
type customRoleController struct {
	web.Controller
}

func (customRoleController) Middleware() []web.Middleware {
	return []web.Middleware{
		web.HttpBearer{Authenticate: web.StaticBearerTokens(map[string]web.Identity{
			"token-alice": alice,
		})}.Middleware(),
		web.AccessControl{
			RoleResolver: func(c *web.Context) []string { return []string{"ticketing"} },
			Rules: []web.AccessRule{
				{Allow: true, Roles: []string{"ticketing"}},
			},
		}.Middleware(),
	}
}

func (customRoleController) ActionBook(c *web.Context) error {
	return c.JSON(map[string]string{"booked": "yes"})
}

// accessApp mounts every access-control shape under its own prefix.
func accessApp(t *testing.T) *web.Handler {
	t.Helper()
	app := base.New()
	h := web.NewHandler(app, web.WithoutCsrf())
	h.Ctrl(adminController{}).Post("/admin/purge", adminController{}.ActionPurge)
	h.Ctrl(editorController{}).Get("/editor/draft", editorController{}.ActionDraft)
	h.Ctrl(signoffController{}).Get("/report/view", signoffController{}.ActionView)
	h.Ctrl(signoffController{}).Post("/report/sign", signoffController{}.ActionSign)
	h.Ctrl(lobbyController{}).Get("/lobby", lobbyController{}.ActionIndex)
	h.Ctrl(memberController{}).Get("/member/desk", memberController{}.ActionDesk)
	h.Ctrl(customRoleController{}).Get("/custom/book", customRoleController{}.ActionBook)
	return h
}

// accessReq performs a request through the access-control app.
func accessReq(t *testing.T, h http.Handler, method, target, authorization string) int {
	t.Helper()
	r := httptest.NewRequest(method, target, nil)
	if authorization != "" {
		r.Header.Set("Authorization", authorization)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec.Code
}

const (
	asAlice = "Basic YWxpY2U6c2VjcmV0LTE=" // alice:secret-1
	asBob   = "Basic Ym9iOnNlY3JldC0x"     // bob:secret-1
)

func TestAccessControlAllowsHolderOfRequiredRole(t *testing.T) {
	h := accessApp(t)

	if code := accessReq(t, h, http.MethodPost, "/admin/purge", asBob); code != http.StatusOK {
		t.Fatalf("admin purge: status = %d, want 200 for the admin role", code)
	}
}

func TestAccessControlDeniesMissingRoleWith403(t *testing.T) {
	h := accessApp(t)

	// alice authenticates fine (no 401) but holds only editor.
	if code := accessReq(t, h, http.MethodPost, "/admin/purge", asAlice); code != http.StatusForbidden {
		t.Fatalf("admin purge as editor: status = %d, want 403", code)
	}
}

func TestAccessControlFirstMatchingRuleWins(t *testing.T) {
	h := accessApp(t)

	if code := accessReq(t, h, http.MethodGet, "/editor/draft", asAlice); code != http.StatusForbidden {
		t.Fatalf("editor draft: status = %d, want 403 (the deny rule matches first)", code)
	}
	if code := accessReq(t, h, http.MethodGet, "/editor/draft", asBob); code != http.StatusOK {
		t.Fatalf("editor draft as admin: status = %d, want 200 (the deny rule does not match)", code)
	}
}

func TestAccessControlVerbScopedRules(t *testing.T) {
	h := accessApp(t)

	if code := accessReq(t, h, http.MethodGet, "/report/view", asAlice); code != http.StatusOK {
		t.Fatalf("report view as editor: status = %d, want 200 (GET rule matches)", code)
	}
	if code := accessReq(t, h, http.MethodPost, "/report/sign", asAlice); code != http.StatusForbidden {
		t.Fatalf("report sign as editor: status = %d, want 403 (no rule matches, default deny)", code)
	}
	if code := accessReq(t, h, http.MethodPost, "/report/sign", asBob); code != http.StatusOK {
		t.Fatalf("report sign as admin: status = %d, want 200 (admin rule matches)", code)
	}
}

func TestAccessControlGuestRequests(t *testing.T) {
	h := accessApp(t)

	// "?" matches only guests: the open lobby welcomes them...
	if code := accessReq(t, h, http.MethodGet, "/lobby", ""); code != http.StatusOK {
		t.Fatalf("lobby as guest: status = %d, want 200", code)
	}
	// ...while "@" (any authenticated requester) is out of reach for them.
	if code := accessReq(t, h, http.MethodGet, "/member/desk", ""); code != http.StatusForbidden {
		t.Fatalf("member desk as guest: status = %d, want 403", code)
	}
}

func TestAccessControlSessionLoginSatisfiesSpecialRoles(t *testing.T) {
	// Authentication arrives through the session, not a credential header:
	// the special roles must see session logins exactly like transient ones.
	app := base.New()
	app.SetSession(fileSessionManager(t))
	h := web.NewHandler(app, web.WithoutCsrf())
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

	// Authenticated by session: "@" applies (member desk opens) while the
	// guest-only lobby closes.
	r := httptest.NewRequest(http.MethodGet, "/member/desk", nil)
	r.AddCookie(ck)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	if rec.Code != http.StatusOK {
		t.Fatalf("member desk with session login: status = %d, want 200", rec.Code)
	}
}

func TestAccessControlCustomRoleResolver(t *testing.T) {
	h := accessApp(t)

	// alice's identity roles are editor, but the resolver says ticketing:
	// the resolver is the authorization seam and its answer wins.
	if code := accessReq(t, h, http.MethodGet, "/custom/book", "Bearer token-alice"); code != http.StatusOK {
		t.Fatalf("custom book: status = %d, want 200 with the resolver-provided role", code)
	}
	// The token table only knows alice's token: the bearer rejection (401)
	// fires before access control could weigh in.
	if code := accessReq(t, h, http.MethodGet, "/custom/book", "Bearer token-bob"); code != http.StatusUnauthorized {
		t.Fatalf("unknown token: status = %d, want 401 from the authenticator", code)
	}
}
