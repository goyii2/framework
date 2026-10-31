// Tests of the backend partition's guard: the assembly main.go composes
// (shared identity finder, the frontend partition's login, the backend
// module's own Register) driven through real HTTP requests with
// cookie-carrying clients. The three states of the admin guard are pinned
// here: a guest is redirected to the login page, an authenticated account
// without the admin role is refused with 403, and an admin sees the
// dashboard inside the partition's dark frame.
package backend

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
	"github.com/goyii2/framework/examples/advanced/frontend"
	"github.com/goyii2/framework/migrate"
	"github.com/goyii2/framework/rbac"
	"github.com/goyii2/framework/session"
	goyiiurl "github.com/goyii2/framework/url"
	"github.com/goyii2/framework/web"
)

// newBackendHandler assembles the application behind the backend partition:
// an in-memory sqlite database the real migrations shaped (user table and
// the RBAC tables alike), a file-backed session store in a per-test
// directory, the frontend partition (the login page the guard redirects
// to), the auth manager over the RBAC tables, and the partition's own
// Register — plus two accounts: admin carries the admin role through an
// RBAC assignment, demo holds none. The handler's route table becomes the
// url.To default, like the production assembly, and Register receives the
// partition's own backend host — the test's stand-in for main.go handing
// in the configured value. The pool is capped at one connection because an
// in-memory database exists per connection. The application is returned
// alongside for direct database assertions.
func newBackendHandler(t *testing.T) (*web.Handler, *base.Application) {
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
	goyiiurl.SetDefault(h.URLs())
	h.SessionManager().IdentityFinder = models.FindIdentity(app)
	if err := frontend.Register(h, app); err != nil {
		t.Fatalf("frontend.Register: %v", err)
	}
	auth, err := rbac.NewDbManager(conn.DB)
	if err != nil {
		t.Fatalf("rbac.NewDbManager: %v", err)
	}
	h.SetAuthManager(auth)
	if err := Register(h, app, auth, "https://backend.example.com"); err != nil {
		t.Fatalf("backend.Register: %v", err)
	}

	for _, acc := range []struct{ username, password string }{
		{"admin", "admin123"},
		{"demo", "demo123"},
	} {
		user := ar.New[models.User](app.WithContext(ctx)).New()
		user.Username = acc.username
		if err := user.SetPassword(acc.password); err != nil {
			t.Fatalf("hashing %s's password: %v", acc.username, err)
		}
		if err := user.Save(); err != nil {
			t.Fatalf("seeding %s's account: %v", acc.username, err)
		}
	}
	if err := auth.AddItem(rbac.Item{Name: "admin", Type: rbac.TypeRole, Description: "may enter the backend"}); err != nil {
		t.Fatalf("adding the admin role: %v", err)
	}
	admin, err := ar.New[models.User](app.WithContext(ctx)).Where("username = ?", "admin").One()
	if err != nil {
		t.Fatalf("loading the admin account: %v", err)
	}
	if err := auth.Assign("admin", admin.GetID()); err != nil {
		t.Fatalf("assigning the admin role: %v", err)
	}
	return h, app
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

// get performs a GET and returns the body and status.
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

// login signs client in through the frontend's form (CSRF token fetched
// per session), mirroring what a browser does.
func login(t *testing.T, client *http.Client, srvURL, username, password string) {
	t.Helper()
	body, status := get(t, client, srvURL+"/login")
	if status != http.StatusOK {
		t.Fatalf("login form status = %d", status)
	}
	match := csrfField.FindStringSubmatch(body)
	if match == nil {
		t.Fatal("login form carries no CSRF field")
	}
	res, err := client.PostForm(srvURL+"/login", url.Values{
		"_csrf":    {match[1]},
		"username": {username},
		"password": {password},
	})
	if err != nil {
		t.Fatalf("posting the login form: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusFound {
		t.Fatalf("login as %s status = %d, want 302", username, res.StatusCode)
	}
}

func TestBackendGuardThreeStates(t *testing.T) {
	h, _ := newBackendHandler(t)

	// Guest: every path below /backend points at the login page instead of
	// answering content — the explicit module route and the convention
	// route alike.
	guest, srv := newBrowser(t, h)
	for _, target := range []string{srv.URL + "/backend/", srv.URL + "/backend/dashboard/index"} {
		res, err := guest.Get(target)
		if err != nil {
			t.Fatalf("GET %s: %v", target, err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusFound {
			t.Errorf("guest %s status = %d, want 302", target, res.StatusCode)
		}
		if got := res.Header.Get("Location"); got != "/login" {
			t.Errorf("guest %s Location = %q, want /login", target, got)
		}
	}

	// Authenticated without the admin role: refused with 403, no redirect.
	demo, srv := newBrowser(t, h)
	login(t, demo, srv.URL, "demo", "demo123")
	body, status := get(t, demo, srv.URL+"/backend/")
	if status != http.StatusForbidden {
		t.Fatalf("demo /backend/ status = %d, want 403", status)
	}
	if strings.Contains(body, "Dashboard") {
		t.Error("the 403 reply must not render the dashboard")
	}

	// Admin: the dashboard answers inside the partition's dark frame —
	// the frontend's light shell is nowhere on the page.
	admin, srv := newBrowser(t, h)
	login(t, admin, srv.URL, "admin", "admin123")
	body, status = get(t, admin, srv.URL+"/backend/")
	if status != http.StatusOK {
		t.Fatalf("admin /backend/ status = %d, want 200", status)
	}
	if !strings.Contains(body, "Dashboard") || !strings.Contains(body, "admin") {
		t.Error("the dashboard shows neither its heading nor the account name")
	}
	if !strings.Contains(body, "backend partition") {
		t.Error("the dashboard does not render in the backend's frame")
	}
	if strings.Contains(body, "frontend partition") {
		t.Error("the dashboard renders in the frontend's frame")
	}
}

func TestRegisterRequiresFrontendLoginRoute(t *testing.T) {
	// Same assembly minus the frontend partition: the guard's redirect
	// target is resolved from the named site/login route, so a missing
	// registration must fail Register loudly instead of redirecting
	// browsers to a convention-assembled ghost path.
	conn, err := db.New(db.Config{Driver: "sqlite", DSN: ":memory:"})
	if err != nil {
		t.Fatalf("connect in-memory sqlite: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	app := base.New()
	app.SetDB(conn)
	h := web.NewHandler(app)
	auth, err := rbac.NewDbManager(conn.DB)
	if err != nil {
		t.Fatalf("rbac.NewDbManager: %v", err)
	}
	if err := Register(h, app, auth, ""); err == nil {
		t.Fatal("Register without the frontend's site/login route must error")
	}
}
