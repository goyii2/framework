// Tests of the login form model end to end: the action binds and validates
// the submission in one LoadForm step, and both caller shapes are served —
// JSON clients get the flat 422 error map or the signed-in reply, browser
// submissions get the re-rendered form with per-field errors, the refilled
// username and, on success, the redirect to the post list. The validation
// messages ride the validators' translation path, so the translated run
// asserts the localization did not regress with the migration.
package controllers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/goyii2/framework/base"
	"github.com/goyii2/framework/db"
	"github.com/goyii2/framework/db/ar"
	"github.com/goyii2/framework/examples/basic/migrations"
	"github.com/goyii2/framework/examples/basic/models"
	"github.com/goyii2/framework/examples/basic/views"
	"github.com/goyii2/framework/i18n"
	"github.com/goyii2/framework/migrate"
	"github.com/goyii2/framework/session"
	goyiiurl "github.com/goyii2/framework/url"
	"github.com/goyii2/framework/web"
)

// newLoginHandler assembles the application slice the login flow needs: an
// in-memory sqlite database the real migrations shaped, a file-backed
// session store in a per-test directory, the project's own message files,
// the site controller's routes and one admin account. CSRF stays on, the
// production shape, so the tests exercise the token round trip the form's
// hidden _csrf field carries.
func newLoginHandler(t *testing.T) *web.Handler {
	t.Helper()
	conn, err := db.New(db.Config{Driver: "sqlite", DSN: ":memory:"})
	if err != nil {
		t.Fatalf("connect in-memory sqlite: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	sqlDB, err := conn.DB.DB()
	if err != nil {
		t.Fatalf("raw connection: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	mig := migrate.NewMigrator(sqlDB, migrate.SQLiteDialect{}, migrations.All())
	mig.UseLock = false
	ctx := context.Background()
	if err := mig.EnsureMigrationTable(ctx); err != nil {
		t.Fatalf("ensure migration table: %v", err)
	}
	if _, err := mig.Up(ctx, 0); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	app := base.New()
	app.SetDB(conn)
	sess, err := session.New(session.Config{Backend: "file", Path: t.TempDir()})
	if err != nil {
		t.Fatalf("session.New: %v", err)
	}
	app.SetSession(sess)
	// The production assembly embeds the message tree; the test reads it
	// from disk, rooted one level up from this package.
	app.SetI18n(i18n.NewMessageSource(os.DirFS("../messages")))

	h := web.NewHandler(app)
	h.Use(web.ContentNegotiator{
		ResponseFormats: []web.Format{
			{Type: "application/json", Format: "json"},
			{Type: "text/html", Format: "html"},
		},
		RequestFormats: []web.Format{
			{Type: "application/json", Format: "json"},
			{Type: "application/x-www-form-urlencoded", Format: "form"},
		},
	}.Middleware())
	h.Use(views.Layout())

	h.SessionManager().IdentityFinder = func(id string) (web.Identity, bool) {
		uid, err := strconv.ParseInt(id, 10, 64)
		if err != nil {
			return nil, false
		}
		user, err := ar.FindByID[models.User](app.WithContext(context.Background()), uid)
		if err != nil {
			return nil, false
		}
		return user, true
	}

	site := &SiteController{}
	sc := h.Ctrl(site)
	sc.Get("/login", site.ActionLoginForm).Name("site/login")
	sc.Post("/login", site.ActionLogin)
	sc.Post("/logout", site.ActionLogout).Name("site/logout")
	sc.Get("/me", site.ActionMe).Name("site/me")
	sc.Get("/site/csrf", site.ActionCsrf).Name("site/csrf")
	// The redirect target of a successful browser login; its handler never
	// runs in these tests.
	h.Get("/posts", func(c *web.Context) error { return c.JSON(nil) }).Name("post/list")
	views.Register(h.Views())
	goyiiurl.SetDefault(h.URLs())

	admin := ar.New[models.User](app.WithContext(ctx)).New()
	admin.Username = "admin"
	admin.Role = "admin"
	if err := admin.SetPassword("admin123"); err != nil {
		t.Fatalf("hashing the admin password: %v", err)
	}
	if err := admin.Save(); err != nil {
		t.Fatalf("seeding the admin account: %v", err)
	}
	return h
}

// newBrowser serves h and returns a client carrying a cookie jar — the
// session and CSRF cookies flow between requests the browser way — that
// never follows redirects, so the tests observe them.
func newBrowser(t *testing.T, h http.Handler) (*http.Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookie jar: %v", err)
	}
	return &http.Client{
		Jar: jar,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}, srv
}

// csrfField matches the hidden CSRF input the form renders.
var csrfField = regexp.MustCompile(`name="_csrf" value="([^"]+)"`)

// loginPage performs one request against the login endpoint and returns the
// body, status and redirect location.
func loginPage(t *testing.T, client *http.Client, target string, header http.Header, body string) (string, int, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, target, strings.NewReader(body))
	if err != nil {
		t.Fatalf("building the request: %v", err)
	}
	for k, vs := range header {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	res, err := client.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", target, err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("reading the body: %v", err)
	}
	return string(raw), res.StatusCode, res.Header.Get("Location")
}

// jsonToken fetches a CSRF token the JSON path accepts: the safe csrf
// endpoint answers it as JSON and issues the matching cookie.
func jsonToken(t *testing.T, client *http.Client, target string) string {
	t.Helper()
	res, err := client.Get(target)
	if err != nil {
		t.Fatalf("GET %s: %v", target, err)
	}
	defer res.Body.Close()
	var reply struct {
		CSRF string `json:"csrf"`
	}
	if err := json.NewDecoder(res.Body).Decode(&reply); err != nil {
		t.Fatalf("decoding the csrf reply: %v", err)
	}
	return reply.CSRF
}

// TestLoginBrowserFlow walks the browser shape: the form page, the 401
// re-render with the submitted username refilled, the 422 re-render with
// per-field errors, and the successful submission redirecting to the post
// list with the session bound.
func TestLoginBrowserFlow(t *testing.T) {
	h := newLoginHandler(t)
	client, srv := newBrowser(t, h)

	res, err := client.Get(srv.URL + "/login")
	if err != nil {
		t.Fatalf("GET /login: %v", err)
	}
	form, _ := io.ReadAll(res.Body)
	res.Body.Close()
	match := csrfField.FindStringSubmatch(string(form))
	if match == nil {
		t.Fatal("login form carries no CSRF field")
	}
	html := http.Header{
		"Accept":       {"text/html"},
		"Content-Type": {"application/x-www-form-urlencoded"},
	}

	// Wrong credentials: 401, the general error, the username refilled.
	body, status, _ := loginPage(t, client, srv.URL+"/login", html, url.Values{
		"_csrf": {match[1]}, "username": {"admin"}, "password": {"wrong"},
	}.Encode())
	if status != http.StatusUnauthorized {
		t.Fatalf("wrong credentials status = %d, want 401", status)
	}
	if !strings.Contains(body, "Wrong username or password.") {
		t.Fatal("wrong credentials render no error")
	}
	if !strings.Contains(body, `value="admin"`) {
		t.Fatal("the submitted username is not refilled")
	}

	// Half-blank submission: 422 with the blank field's note alone and the
	// filled half refilled.
	body, status, _ = loginPage(t, client, srv.URL+"/login", html, url.Values{
		"_csrf": {match[1]}, "username": {"admin"}, "password": {""},
	}.Encode())
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("half-blank submission status = %d, want 422", status)
	}
	if !strings.Contains(body, "password cannot be blank") || strings.Contains(body, "username cannot be blank") {
		t.Fatal("half-blank submission renders the wrong field notes")
	}
	if !strings.Contains(body, `value="admin"`) {
		t.Fatal("the submitted username is not refilled on the rejected form")
	}

	// Correct credentials: redirect to the post list, the session bound.
	_, status, location := loginPage(t, client, srv.URL+"/login", html, url.Values{
		"_csrf": {match[1]}, "username": {"admin"}, "password": {"admin123"},
	}.Encode())
	if status != http.StatusFound || location != "/posts" {
		t.Fatalf("login status = %d location = %q, want 302 /posts", status, location)
	}
	res, err = client.Get(srv.URL + "/me")
	if err != nil {
		t.Fatalf("GET /me: %v", err)
	}
	var me struct {
		Guest bool   `json:"guest"`
		Name  string `json:"name"`
	}
	if err := json.NewDecoder(res.Body).Decode(&me); err != nil {
		t.Fatalf("decoding /me: %v", err)
	}
	res.Body.Close()
	if me.Guest || me.Name != "admin" {
		t.Fatalf("/me after login = %+v, want the bound admin identity", me)
	}
}

// TestLoginJSONFlow walks the API shape: blank and malformed bodies answer
// the flat 422 error map, wrong credentials stay the uniform 401, correct
// credentials return the account.
func TestLoginJSONFlow(t *testing.T) {
	h := newLoginHandler(t)
	client, srv := newBrowser(t, h)

	token := jsonToken(t, client, srv.URL+"/site/csrf")
	postJSON := func(body string) (string, int) {
		header := http.Header{
			"Content-Type": {"application/json"},
			"X-Csrf-Token": {token},
		}
		body, status, _ := loginPage(t, client, srv.URL+"/login", header, body)
		return body, status
	}

	// Blank submission: 422 with each required field's own message.
	body, status := postJSON(`{"username": "", "password": ""}`)
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("blank JSON status = %d, want 422", status)
	}
	var fields map[string][]string
	if err := json.Unmarshal([]byte(body), &fields); err != nil {
		t.Fatalf("decoding the error map: %v", err)
	}
	if got := fields["username"]; len(got) != 1 || got[0] != "username cannot be blank" {
		t.Errorf(`fields["username"] = %v, want the blank note`, got)
	}
	if got := fields["password"]; len(got) != 1 || got[0] != "password cannot be blank" {
		t.Errorf(`fields["password"] = %v, want the blank note`, got)
	}

	// A body that is not one JSON object is a record-level failure under
	// the $root key of the same flat map.
	body, status = postJSON(`{"username": `)
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("malformed JSON status = %d, want 422", status)
	}
	fields = nil
	if err := json.Unmarshal([]byte(body), &fields); err != nil {
		t.Fatalf("decoding the root error map: %v", err)
	}
	if got := fields["$root"]; len(got) != 1 || got[0] != "malformed JSON body" {
		t.Errorf(`fields["$root"] = %v, want the malformed-body note`, got)
	}

	// Wrong credentials: the uniform 401 that never reveals which half.
	_, status = postJSON(`{"username": "admin", "password": "wrong"}`)
	if status != http.StatusUnauthorized {
		t.Fatalf("wrong credentials status = %d, want 401", status)
	}

	// Correct credentials: the account reply.
	body, status = postJSON(`{"username": "admin", "password": "admin123"}`)
	if status != http.StatusOK {
		t.Fatalf("login status = %d, want 200", status)
	}
	var account struct {
		Name string `json:"name"`
		Role string `json:"role"`
	}
	if err := json.Unmarshal([]byte(body), &account); err != nil {
		t.Fatalf("decoding the account reply: %v", err)
	}
	if account.Name != "admin" || account.Role != "admin" {
		t.Fatalf("account reply = %+v, want the admin account", account)
	}
}

// TestLoginTranslatesValidationMessages pins the migration's i18n contract:
// the rule messages ride the validators' translation path, so a zh-CN
// requester sees the translated notes exactly as the inline schema produced
// them.
func TestLoginTranslatesValidationMessages(t *testing.T) {
	h := newLoginHandler(t)
	client, srv := newBrowser(t, h)

	res, err := client.Get(srv.URL + "/login")
	if err != nil {
		t.Fatalf("GET /login: %v", err)
	}
	form, _ := io.ReadAll(res.Body)
	res.Body.Close()
	match := csrfField.FindStringSubmatch(string(form))
	if match == nil {
		t.Fatal("login form carries no CSRF field")
	}

	body, status, _ := loginPage(t, client, srv.URL+"/login", http.Header{
		"Accept":          {"text/html"},
		"Accept-Language": {"zh-CN"},
		"Content-Type":    {"application/x-www-form-urlencoded"},
	}, url.Values{
		"_csrf": {match[1]}, "username": {""}, "password": {""},
	}.Encode())
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("blank submission status = %d, want 422", status)
	}
	if !strings.Contains(body, "用户名不能为空") || !strings.Contains(body, "密码不能为空") {
		t.Fatal("the validation notes are not translated")
	}
}
