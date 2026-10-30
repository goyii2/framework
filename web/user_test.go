package web_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/goyii2/framework/base"
	"github.com/goyii2/framework/session"
	"github.com/goyii2/framework/web"
)

// testIdentity is the shared Identity double: a username, an id and a set of
// roles.
type testIdentity struct {
	id    string
	name  string
	roles []string
}

func (i testIdentity) GetID() string      { return i.id }
func (i testIdentity) GetName() string    { return i.name }
func (i testIdentity) GetRoles() []string { return i.roles }

var (
	alice = testIdentity{id: "107", name: "alice", roles: []string{"editor"}}
	bob   = testIdentity{id: "208", name: "bob", roles: []string{"admin"}}
)

// userApp assembles a login/logout application over the given session
// manager. The identity finder resolves from a mutable table so tests can
// also simulate identities disappearing between requests.
func userApp(t *testing.T, m *session.Manager) (*web.Handler, *map[string]web.Identity) {
	t.Helper()
	users := map[string]web.Identity{alice.GetID(): alice, bob.GetID(): bob}
	app := base.New()
	app.SetSession(m)
	h := web.NewHandler(app, web.WithoutCsrf())
	h.SessionManager().IdentityFinder = func(id string) (web.Identity, bool) {
		u, ok := users[id]
		return u, ok
	}

	h.Get("/whoami", func(c *web.Context) error {
		name := ""
		if id := c.User().Identity(); id != nil {
			name = id.GetName()
		}
		return c.JSON(map[string]any{"name": name, "guest": c.User().IsGuest()})
	})
	h.Post("/login", func(c *web.Context) error {
		if err := c.User().Login(alice); err != nil {
			return err
		}
		return c.JSON(map[string]string{"ok": "yes"})
	})
	h.Post("/logout", func(c *web.Context) error {
		c.User().Logout()
		return c.JSON(map[string]string{"ok": "yes"})
	})
	h.Get("/touch", func(c *web.Context) error {
		c.Session().Set("t", "1")
		return c.JSON(map[string]string{"ok": "yes"})
	})
	h.Get("/read", func(c *web.Context) error {
		return c.JSON(map[string]string{"t": c.Session().GetString("t")})
	})
	return h, &users
}

// whoami performs GET /whoami and returns name and guest flag.
func whoami(t *testing.T, h http.Handler, cookies ...*http.Cookie) (string, bool) {
	t.Helper()
	res := sessionReq(t, h, http.MethodGet, "/whoami", cookies...)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d on /whoami", res.StatusCode)
	}
	var body struct {
		Name  string `json:"name"`
		Guest bool   `json:"guest"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("decoding body: %v", err)
	}
	return body.Name, body.Guest
}

func TestUserStartsAsGuest(t *testing.T) {
	h, _ := userApp(t, fileSessionManager(t))

	if name, guest := whoami(t, h); name != "" || !guest {
		t.Fatalf("fresh request: name=%q guest=%v, want a guest", name, guest)
	}
}

func TestLoginBindsIdentityAcrossRequests(t *testing.T) {
	h, _ := userApp(t, fileSessionManager(t))

	res := sessionReq(t, h, http.MethodPost, "/login")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d on /login", res.StatusCode)
	}
	ck := sessionCookieOf(t, res, "goyii-session")
	if ck == nil || ck.Value == "" {
		t.Fatal("login issued no session cookie")
	}

	// The very request that logs in already sees the identity, and so does
	// any follow-up request presenting the session cookie.
	if name, guest := whoami(t, h, ck); name != "alice" || guest {
		t.Fatalf("after login: name=%q guest=%v, want alice authenticated", name, guest)
	}
}

// TestLoginRegeneratesSessionID is the session-fixation defense: an id the
// client held before login is worthless afterwards — the login state only
// exists under a fresh id, the old record is destroyed, and pre-login data
// still travels along.
func TestLoginRegeneratesSessionID(t *testing.T) {
	for _, backend := range []struct {
		name string
		mgr  func(t *testing.T) *session.Manager
	}{
		{"file", fileSessionManager},
		{"redis", redisSessionManager},
	} {
		t.Run(backend.name, func(t *testing.T) {
			m := backend.mgr(t)
			h, _ := userApp(t, m)

			// Establish a pre-login session the attacker's id might be
			// riding on, with data the legitimate user expects to keep.
			ck := sessionCookieOf(t, sessionReq(t, h, http.MethodGet, "/touch"), "goyii-session")
			if ck == nil {
				t.Fatal("pre-login session issued no cookie")
			}

			res := sessionReq(t, h, http.MethodPost, "/login", ck)
			if res.StatusCode != http.StatusOK {
				t.Fatalf("status = %d on /login", res.StatusCode)
			}
			ck2 := sessionCookieOf(t, res, "goyii-session")
			if ck2 == nil || ck2.Value == "" || ck2.Value == ck.Value {
				t.Fatalf("login cookie = %+v, want a rotated id distinct from %q", ck2, ck.Value)
			}

			// Old id dead, new id authenticated, data preserved.
			if name, guest := whoami(t, h, ck); !guest || name != "" {
				t.Fatalf("old id after login: name=%q guest=%v, want the planted id dead", name, guest)
			}
			if name, guest := whoami(t, h, ck2); name != "alice" || guest {
				t.Fatalf("new id after login: name=%q guest=%v, want alice", name, guest)
			}
			if _, ok := loadSessionRecord(t, m, ck2.Value); !ok {
				t.Fatal("new session record missing from backend")
			}
			if _, ok := loadSessionRecord(t, m, ck.Value); ok {
				t.Fatal("old session record still alive in backend")
			}
			res = sessionReq(t, h, http.MethodGet, "/read", ck2)
			var body map[string]string
			if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
				t.Fatalf("decoding body: %v", err)
			}
			if body["t"] != "1" {
				t.Fatalf("pre-login data t = %q under new id, want 1", body["t"])
			}
		})
	}
}

// TestLogoutInvalidatesSession: after logout the session record is gone, the
// cookie is cleared and follow-up requests with the old id are guests.
func TestLogoutInvalidatesSession(t *testing.T) {
	m := fileSessionManager(t)
	h, _ := userApp(t, m)

	ck := sessionCookieOf(t, sessionReq(t, h, http.MethodPost, "/login"), "goyii-session")
	if name, _ := whoami(t, h, ck); name != "alice" {
		t.Fatalf("login did not stick: name=%q", name)
	}

	res := sessionReq(t, h, http.MethodPost, "/logout", ck)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d on /logout", res.StatusCode)
	}
	if ck2 := sessionCookieOf(t, res, "goyii-session"); ck2 == nil || ck2.MaxAge >= 0 {
		t.Fatalf("logout must clear the cookie, got %+v", ck2)
	}
	if _, ok := loadSessionRecord(t, m, ck.Value); ok {
		t.Fatal("session record still alive in backend after logout")
	}
	if name, guest := whoami(t, h, ck); name != "" || !guest {
		t.Fatalf("after logout: name=%q guest=%v, want guest", name, guest)
	}
}

// TestIdentityDisappearingBetweenRequests: a stored id that no longer
// resolves (deleted account) degrades to a guest instead of failing the
// request, and the stale key is dropped.
func TestIdentityDisappearingBetweenRequests(t *testing.T) {
	m := fileSessionManager(t)
	h, users := userApp(t, m)

	ck := sessionCookieOf(t, sessionReq(t, h, http.MethodPost, "/login"), "goyii-session")
	delete(*users, alice.GetID())

	if name, guest := whoami(t, h, ck); name != "" || !guest {
		t.Fatalf("unresolvable identity: name=%q guest=%v, want guest", name, guest)
	}
	// The stale key left the record, so the session is a plain empty one.
	if _, ok := loadSessionRecord(t, m, ck.Value); ok {
		record, _ := loadSessionRecord(t, m, ck.Value)
		if _, has := record["__id"]; has {
			t.Fatalf("stale identity key survived: %#v", record)
		}
	}
}

// TestUserZeroValueRejectsLogin: a User without a session (the stateless
// shape authentication middlewares install) refuses to log in rather than
// pretending the login stuck.
func TestUserZeroValueRejectsLogin(t *testing.T) {
	u := &web.User{}
	if err := u.Login(alice); err == nil {
		t.Fatal("login without a session must fail")
	}
	if id := u.Identity(); id != nil {
		t.Fatalf("identity = %v, want nil", id)
	}
	if !u.IsGuest() {
		t.Fatal("zero-value user must be a guest")
	}
}

// TestIdentityFromContext covers the resolution seam the auditing
// behaviors ride on: an explicitly bound identity wins, the request's own
// user resolves otherwise, and a context without either reports nil.
func TestIdentityFromContext(t *testing.T) {
	if id := web.IdentityFromContext(context.Background()); id != nil {
		t.Fatalf("plain context: identity = %v, want nil", id)
	}
	ctx := web.WithIdentity(context.Background(), alice)
	if id := web.IdentityFromContext(ctx); id.GetID() != alice.GetID() {
		t.Fatalf("explicit identity = %v, want %v", id, alice)
	}
	if id := web.IdentityFromContext(web.WithIdentity(ctx, nil)); id != nil {
		t.Fatalf("nil explicit identity = %v, want nil", id)
	}

	// Through a request: a guest resolves nil, the session identity
	// resolves by name, and an explicitly bound identity outranks the
	// session one.
	m := fileSessionManager(t)
	h, _ := userApp(t, m)
	h.Get("/resolve", func(c *web.Context) error {
		name := ""
		if id := web.IdentityFromContext(c.Context()); id != nil {
			name = id.GetName()
		}
		return c.JSON(map[string]string{"name": name})
	})

	res := sessionReq(t, h, http.MethodGet, "/resolve")
	var body struct{ Name string }
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("decode guest body: %v", err)
	}
	if body.Name != "" {
		t.Fatalf("guest resolve: name = %q, want empty", body.Name)
	}

	ck := sessionCookieOf(t, sessionReq(t, h, http.MethodPost, "/login"), "goyii-session")
	res = sessionReq(t, h, http.MethodGet, "/resolve", ck)
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("decode logged-in body: %v", err)
	}
	if body.Name != alice.GetName() {
		t.Fatalf("logged-in resolve: name = %q, want %q", body.Name, alice.GetName())
	}

	h.Get("/resolve-explicit", func(c *web.Context) error {
		name := ""
		if id := web.IdentityFromContext(web.WithIdentity(c.Context(), bob)); id != nil {
			name = id.GetName()
		}
		return c.JSON(map[string]string{"name": name})
	})
	res = sessionReq(t, h, http.MethodGet, "/resolve-explicit", ck)
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("decode explicit body: %v", err)
	}
	if body.Name != bob.GetName() {
		t.Fatalf("explicit resolve: name = %q, want %q (explicit outranks the session)", body.Name, bob.GetName())
	}
}
