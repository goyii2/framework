// Tests of the frontend partition's login and logout flow: the assembly
// main.go composes (session store, shared identity finder, the partition's
// own Register) driven through real HTTP requests with a cookie-carrying
// client — wrong credentials replay the form under 401 without storing a
// login, correct credentials bind the session and flip the navigation, and
// logout returns the requester to the anonymous state.
package frontend

import (
	"context"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/goyii2/framework/base"
	"github.com/goyii2/framework/db"
	"github.com/goyii2/framework/db/ar"
	"github.com/goyii2/framework/examples/advanced/common/migrations"
	"github.com/goyii2/framework/examples/advanced/common/models"
	"github.com/goyii2/framework/migrate"
	"github.com/goyii2/framework/session"
	"github.com/goyii2/framework/web"
)

// newFrontendHandler assembles the application behind the frontend
// partition: an in-memory sqlite database the real migrations shaped, a
// file-backed session store in a per-test directory, the shared identity
// finder, the partition's own Register — and one demo account. The pool is
// capped at one connection because an in-memory database exists per
// connection.
func newFrontendHandler(t *testing.T) *web.Handler {
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

	h := web.NewHandler(app) // CSRF on, like the production assembly
	h.SessionManager().IdentityFinder = models.FindIdentity(app)
	if err := Register(h, app); err != nil {
		t.Fatalf("frontend.Register: %v", err)
	}

	user := ar.New[models.User](app.WithContext(ctx)).New()
	user.Username, user.Role = "demo", "user"
	if err := user.SetPassword("demo123"); err != nil {
		t.Fatalf("hashing the demo password: %v", err)
	}
	if err := user.Save(); err != nil {
		t.Fatalf("seeding the demo account: %v", err)
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

// get performs a GET and returns the body.
func get(t *testing.T, client *http.Client, target string) (string, int) {
	t.Helper()
	res, err := client.Get(target)
	if err != nil {
		t.Fatalf("GET %s: %v", target, err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("reading the body: %v", err)
	}
	return string(body), res.StatusCode
}

// postForm submits form values and returns the body, status and redirect
// location.
func postForm(t *testing.T, client *http.Client, target string, vals url.Values) (string, int, string) {
	t.Helper()
	res, err := client.PostForm(target, vals)
	if err != nil {
		t.Fatalf("POST %s: %v", target, err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("reading the body: %v", err)
	}
	return string(body), res.StatusCode, res.Header.Get("Location")
}

func TestLoginLogoutFlow(t *testing.T) {
	h := newFrontendHandler(t)
	client, srv := newBrowser(t, h)

	// Anonymous navigation: the login link, no logout form.
	body, status := get(t, client, srv.URL+"/")
	if status != http.StatusOK {
		t.Fatalf("home status = %d", status)
	}
	if !strings.Contains(body, ">Login</a>") {
		t.Fatal("guest navigation shows no login link")
	}
	if strings.Contains(body, "Logout") {
		t.Fatal("guest navigation shows a logout form")
	}

	// The form page issues the CSRF token the submission must echo back.
	body, status = get(t, client, srv.URL+"/login")
	if status != http.StatusOK || !strings.Contains(body, `name="username"`) {
		t.Fatalf("login form status = %d", status)
	}
	match := csrfField.FindStringSubmatch(body)
	if match == nil {
		t.Fatal("login form carries no CSRF field")
	}
	token := match[1]

	// Wrong credentials: 401, the error re-displayed, the username refilled
	// — and no login stored.
	body, status, _ = postForm(t, client, srv.URL+"/login", url.Values{
		"_csrf":    {token},
		"username": {"demo"},
		"password": {"wrong"},
	})
	if status != http.StatusUnauthorized {
		t.Fatalf("wrong credentials status = %d, want 401", status)
	}
	if !strings.Contains(body, "Wrong username or password.") {
		t.Fatal("wrong credentials render no error")
	}
	if !strings.Contains(body, `value="demo"`) {
		t.Fatal("the submitted username is not refilled")
	}
	if body, _ = get(t, client, srv.URL+"/"); strings.Contains(body, "Logout") {
		t.Fatal("a failed login must leave the requester a guest")
	}

	// Correct credentials: redirect home, navigation signed in.
	body, status, location := postForm(t, client, srv.URL+"/login", url.Values{
		"_csrf":    {token},
		"username": {"demo"},
		"password": {"demo123"},
	})
	if status != http.StatusFound || location != "/" {
		t.Fatalf("login status = %d location = %q, want 302 /", status, location)
	}
	body, _ = get(t, client, srv.URL+"/")
	if !strings.Contains(body, "<span>demo</span>") || !strings.Contains(body, "Logout") {
		t.Fatal("signed-in navigation shows neither the account name nor the logout form")
	}
	if !strings.Contains(body, "Signed in as") {
		t.Fatal("the home page shows no signed-in greeting")
	}

	// Logout: redirect home, back to the anonymous navigation.
	body, status, location = postForm(t, client, srv.URL+"/logout", url.Values{"_csrf": {token}})
	if status != http.StatusFound || location != "/" {
		t.Fatalf("logout status = %d location = %q, want 302 /", status, location)
	}
	body, _ = get(t, client, srv.URL+"/")
	if !strings.Contains(body, ">Login</a>") || strings.Contains(body, "Logout") {
		t.Fatal("logout did not restore the anonymous navigation")
	}
}

func TestLoginBlankFieldsAnswerPerFieldErrors(t *testing.T) {
	h := newFrontendHandler(t)
	client, srv := newBrowser(t, h)

	body, _ := get(t, client, srv.URL+"/login")
	match := csrfField.FindStringSubmatch(body)
	if match == nil {
		t.Fatal("login form carries no CSRF field")
	}

	body, status, _ := postForm(t, client, srv.URL+"/login", url.Values{
		"_csrf":    {match[1]},
		"username": {""},
		"password": {""},
	})
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("blank submission status = %d, want 422", status)
	}
	if !strings.Contains(body, "username cannot be blank") || !strings.Contains(body, "password cannot be blank") {
		t.Fatal("blank fields render no per-field errors")
	}
}
