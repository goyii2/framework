// Tests of the post CRUD plumbing: the listing-search escaper, and the write
// actions end to end now that they bind request values straight into the
// record — the model's form tags as the whitelist. The JSON API and the
// browser form share one form model: both shapes answer a rejected submission
// with 422 and the entered values, an update always fetches the row first so
// fields a submission left out keep their stored values, and no request key
// outside the whitelist (an id, an author reference, a timestamp) assigns
// anything.
package controllers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
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
	"github.com/goyii2/framework/rbac"
	"github.com/goyii2/framework/session"
	goyiiurl "github.com/goyii2/framework/url"
	"github.com/goyii2/framework/web"
)

// newPostHandler assembles the application slice the post write actions
// need: an in-memory sqlite database the real migrations shaped (the RBAC
// tables among them), a file-backed session store in a per-test directory,
// the project's message files, the site controller's login endpoints, the
// post controller's write routes and one admin account holding the editor
// role — the RBAC hierarchy grants post/create and post/update through it.
// CSRF stays on, the production shape, so every test rides the token round
// trip. The application is returned alongside for direct database
// assertions.
func newPostHandler(t *testing.T) (*web.Handler, *base.Application) {
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

	auth, err := rbac.NewDbManager(conn.DB)
	if err != nil {
		t.Fatalf("rbac.NewDbManager: %v", err)
	}

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
	h.SetAuthManager(auth)

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
	sc.Get("/site/csrf", site.ActionCsrf).Name("site/csrf")

	post := &PostController{Auth: auth}
	pcr := h.Ctrl(post)
	// The listing is the real action — the redirect and cancel targets of
	// the form pages resolve by name too — and its filtering has its own
	// tests.
	pcr.Get("/posts", post.ActionList).Name("post/list")
	// Browser pages.
	pcr.Get("/posts/new", post.ActionNew).Name("post/new")
	pcr.Post("/posts", post.ActionStore).Name("post/store")
	pcr.Get("/posts/{id:[0-9]+}", post.ActionShow).Name("post/show")
	pcr.Get("/posts/{id:[0-9]+}/edit", post.ActionEdit).Name("post/edit")
	pcr.Post("/posts/{id:[0-9]+}", post.ActionSave).Name("post/save")
	// JSON API.
	pcr.Get("/post/{id:[0-9]+}", post.ActionView).Name("post/view")
	pcr.Post("/post", post.ActionCreate).Name("post/create")
	pcr.Put("/post/{id:[0-9]+}", post.ActionUpdate).Name("post/update")
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
	for _, item := range []rbac.Item{
		{Name: "editor", Type: rbac.TypeRole, Description: "may write posts"},
		{Name: "post/create", Type: rbac.TypePermission, Description: "may create posts"},
		{Name: "post/update", Type: rbac.TypePermission, Description: "may edit any post"},
	} {
		if err := auth.AddItem(item); err != nil {
			t.Fatalf("adding the rbac item %s: %v", item.Name, err)
		}
	}
	for _, edge := range [][2]string{{"editor", "post/create"}, {"editor", "post/update"}} {
		if err := auth.AddChild(edge[0], edge[1]); err != nil {
			t.Fatalf("adding the rbac edge %s -> %s: %v", edge[0], edge[1], err)
		}
	}
	if err := auth.Assign("editor", admin.GetID()); err != nil {
		t.Fatalf("assigning the editor role: %v", err)
	}
	return h, app
}

// getPage performs a GET and returns the body and status.
func getPage(t *testing.T, client *http.Client, target string) (string, int) {
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
	body, status, location := loginPage(t, client, target, http.Header{
		"Accept":       {"text/html"},
		"Content-Type": {"application/x-www-form-urlencoded"},
	}, vals.Encode())
	return body, status, location
}

// browserLogin signs client in through the login form, the browser way: one
// GET fetches the CSRF token, one POST binds the session cookie.
func browserLogin(t *testing.T, client *http.Client, srvURL string) {
	t.Helper()
	body, _ := getPage(t, client, srvURL+"/login")
	match := csrfField.FindStringSubmatch(body)
	if match == nil {
		t.Fatal("login form carries no CSRF field")
	}
	_, status, _ := loginPage(t, client, srvURL+"/login", http.Header{
		"Accept":       {"text/html"},
		"Content-Type": {"application/x-www-form-urlencoded"},
	}, url.Values{
		"_csrf": {match[1]}, "username": {"admin"}, "password": {"admin123"},
	}.Encode())
	if status != http.StatusFound {
		t.Fatalf("login status = %d, want 302", status)
	}
}

// callJSON performs one JSON API request carrying the session cookie and the
// CSRF header, and returns the reply with its body.
func callJSON(t *testing.T, client *http.Client, method, target, token, body string) (*http.Response, string) {
	t.Helper()
	req, err := http.NewRequest(method, target, strings.NewReader(body))
	if err != nil {
		t.Fatalf("building the request: %v", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Csrf-Token", token)
	res, err := client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, target, err)
	}
	raw, err := io.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		t.Fatalf("reading the body: %v", err)
	}
	return res, string(raw)
}

// loadPost fetches one row straight from the database for assertions.
func loadPost(t *testing.T, app *base.Application, id int64) models.Post {
	t.Helper()
	post, err := ar.FindByID[models.Post](app.WithContext(context.Background()), id)
	if err != nil {
		t.Fatalf("loading post %d: %v", id, err)
	}
	return *post
}

// TestLikePatternEscapesWildcards pins the escaping contract of the listing
// search: the wrapping wildcards provide the contains match while every
// wildcard the user typed is neutralized to its literal meaning, so input
// can never reshape the match semantics.
func TestLikePatternEscapesWildcards(t *testing.T) {
	bs := string(rune(92)) // one backslash, the LIKE escape character
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"plain text stays literal", "hello", "%hello%"},
		{"percent is escaped", "50%", "%50" + bs + "%%"},
		{"underscore is escaped", "sample_post", "%sample" + bs + "_post%"},
		{"escape character is escaped", "a" + bs + "b", "%a" + bs + bs + "b%"},
		{"all wildcards together", "%_", "%" + bs + "%" + bs + "_" + "%"},
		{"empty input still wraps", "", "%%"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := likePattern(tc.raw); got != tc.want {
				t.Fatalf("likePattern(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

// seedListPost stores one listing row straight through the database.
func seedListPost(t *testing.T, app *base.Application, author int64, title, status string) {
	t.Helper()
	post := ar.New[models.Post](app.WithContext(context.Background())).New()
	post.Title, post.Body, post.Status = title, "body of "+title, status
	post.AuthorID = author
	if err := post.Save(); err != nil {
		t.Fatalf("seeding the post %q: %v", title, err)
	}
}

// TestPostListFilterBehavior pins the listing's filter semantics end to end
// now that the inputs bind through the form whitelist: the title narrows by
// a contains-match and the status exactly, a user-typed LIKE wildcard (% or
// _) matches only its literal self, a title wrapped in spaces filters as its
// trimmed text, and an empty query lists everything.
func TestPostListFilterBehavior(t *testing.T) {
	h, app := newPostHandler(t)
	client, srv := newBrowser(t, h)
	adminID := loadAdminID(t, app)

	seedListPost(t, app, adminID, "Alpha report", models.PostStatusPublished)
	seedListPost(t, app, adminID, "Beta report", models.PostStatusDraft)
	seedListPost(t, app, adminID, "50% off sale", models.PostStatusPublished)
	seedListPost(t, app, adminID, "under_score note", models.PostStatusPublished)
	// Would match the title pattern if "_" still acted as a single-character
	// wildcard; the escaper must keep it literal.
	seedListPost(t, app, adminID, "underXscore bait", models.PostStatusPublished)

	// Title and status together: only the row matching both is listed, and
	// the filter row echoes the submitted title.
	body, status := getPage(t, client, srv.URL+"/posts?title=report&status=published")
	if status != http.StatusOK {
		t.Fatalf("filtered list status = %d, want 200", status)
	}
	if !strings.Contains(body, "Alpha report") {
		t.Error("the filtered list lacks the row matching both filters")
	}
	for _, absent := range []string{"Beta report", "50% off sale", "under_score note", "underXscore bait"} {
		if strings.Contains(body, absent) {
			t.Errorf("the filtered list shows %q, which the filters must exclude", absent)
		}
	}
	if !strings.Contains(body, `value="report"`) {
		t.Error("the filter row does not echo the submitted title")
	}

	// A bare percent matches only titles containing a literal percent —
	// user input can never reshape the match into a catch-all.
	body, status = getPage(t, client, srv.URL+"/posts?title=%25")
	if status != http.StatusOK {
		t.Fatalf("percent-filtered list status = %d, want 200", status)
	}
	if !strings.Contains(body, "50% off sale") || strings.Contains(body, "Alpha report") {
		t.Error("a percent filter must match only titles carrying a literal percent")
	}

	// An underscore stays literal the same way: the bait row whose title
	// differs exactly at that character stays out.
	body, status = getPage(t, client, srv.URL+"/posts?title=under_score")
	if status != http.StatusOK {
		t.Fatalf("underscore-filtered list status = %d, want 200", status)
	}
	if !strings.Contains(body, "under_score note") || strings.Contains(body, "underXscore bait") {
		t.Error("an underscore filter must match only its literal self")
	}

	// A title wrapped in spaces filters as its trimmed text.
	body, status = getPage(t, client, srv.URL+"/posts?title=%20report%20&status=published")
	if status != http.StatusOK {
		t.Fatalf("padded-filter list status = %d, want 200", status)
	}
	if !strings.Contains(body, "Alpha report") || strings.Contains(body, "Beta report") {
		t.Error("a padded title must filter as its trimmed text")
	}

	// No filters: everything is listed.
	body, status = getPage(t, client, srv.URL+"/posts")
	if status != http.StatusOK {
		t.Fatalf("unfiltered list status = %d, want 200", status)
	}
	for _, want := range []string{"Alpha report", "Beta report", "50% off sale", "under_score note", "underXscore bait"} {
		if !strings.Contains(body, want) {
			t.Errorf("the unfiltered list lacks %q", want)
		}
	}
}

// TestPostListKeepsFiltersInLinks pins the keep closure: sort toggles and
// pager buttons derive the query parameters they preserve from the filter
// model itself, so a filtered listing keeps its filters across sorting and
// paging, an unfiltered one adds no filter parameters, and a padded title
// survives as its trimmed text.
func TestPostListKeepsFiltersInLinks(t *testing.T) {
	h, app := newPostHandler(t)
	client, srv := newBrowser(t, h)
	adminID := loadAdminID(t, app)

	// Enough matching rows to push the listing past one page of ten.
	for i := 1; i <= 11; i++ {
		seedListPost(t, app, adminID, "Pager report "+strconv.Itoa(i), models.PostStatusPublished)
	}
	seedListPost(t, app, adminID, "Pager report draft", models.PostStatusDraft)

	body, status := getPage(t, client, srv.URL+"/posts?title=Pager+report&status=published")
	if status != http.StatusOK {
		t.Fatalf("filtered list status = %d, want 200", status)
	}
	if strings.Contains(body, "Pager report draft") {
		t.Error("the filtered list shows the draft row the status filter excludes")
	}
	// The sort toggle of a column keeps both filter values.
	if !strings.Contains(body, `href="/posts?sort=title&amp;status=published&amp;title=Pager+report"`) {
		t.Error("the sort link does not keep the filter values")
	}
	// And so does the pager's page-two button.
	if !strings.Contains(body, `href="/posts?page=2&amp;status=published&amp;title=Pager+report"`) {
		t.Error("the pager link does not keep the filter values")
	}

	// A padded title survives into the kept links as its trimmed text.
	body, _ = getPage(t, client, srv.URL+"/posts?title=%20Pager%20report%20&status=published")
	if !strings.Contains(body, `title=Pager+report"`) {
		t.Error("the kept title is not the trimmed text")
	}
	if strings.Contains(body, "title=%20Pager%20report%20") {
		t.Error("the kept title carries the raw padded text")
	}

	// An unfiltered listing adds no filter parameters to its links.
	body, _ = getPage(t, client, srv.URL+"/posts")
	if strings.Contains(body, "status=published") || strings.Contains(body, "title=Pager+report") {
		t.Error("the unfiltered listing keeps filter parameters it never had")
	}
	if !strings.Contains(body, `href="/posts?sort=id"`) {
		t.Error("the unfiltered sort link lost its plain shape")
	}
}

// TestPostShowRendersPermalink pins the absolute-URL touchpoint: with host
// info wired on the route table (the test's stand-in for main.go's
// SetHostInfo line), the detail page's permalink renders the scheme, host
// and the named route's path while everything else on the page stays
// relative — the permalink is the one absolute link of the browser face.
func TestPostShowRendersPermalink(t *testing.T) {
	h, app := newPostHandler(t)
	h.URLs().SetHostInfo("https://basic.example.com")
	client, srv := newBrowser(t, h)

	seedListPost(t, app, loadAdminID(t, app), "Permalink story", models.PostStatusPublished)
	post, err := ar.New[models.Post](app.WithContext(context.Background())).Where("title = ?", "Permalink story").One()
	if err != nil {
		t.Fatalf("loading the seeded post: %v", err)
	}
	permalink := "https://basic.example.com/posts/" + strconv.FormatInt(post.ID, 10)

	body, status := getPage(t, client, srv.URL+"/posts/"+strconv.FormatInt(post.ID, 10))
	if status != http.StatusOK {
		t.Fatalf("detail status = %d, want 200", status)
	}
	if !strings.Contains(body, `href="`+permalink+`"`) {
		t.Errorf("the detail page lacks the permalink link %q", permalink)
	}
	// The permalink URL rides the page twice only — its href and its visible
	// text — so no other link silently turned absolute.
	if n := strings.Count(body, permalink); n != 2 {
		t.Errorf("the permalink appears %d times, want 2 (href and text)", n)
	}
	// The rest of the page keeps its relative shape: the back link and the
	// breadcrumb's posts entry address the site itself, never an origin.
	if !strings.Contains(body, `href="/posts"`) {
		t.Error("the page lacks the relative back link href=\"/posts\"")
	}
}

// TestPostJSONCreateFlow pins the create side of the JSON API: a valid body
// stores the row with the signed-in author, a body the rules reject answers
// the flat 422 map without writing, a body that is not one JSON object is
// the record-level $root failure of the same map, and keys outside the
// model's form whitelist — case variants, the primary key, the author
// reference, timestamps — assign nothing.
func TestPostJSONCreateFlow(t *testing.T) {
	h, app := newPostHandler(t)
	client, srv := newBrowser(t, h)
	browserLogin(t, client, srv.URL)
	token := jsonToken(t, client, srv.URL+"/site/csrf")
	adminID := loadAdminID(t, app)

	// A valid body: 201, the named-route Location header, the row stored
	// with the submitted values and the signed-in author.
	res, raw := callJSON(t, client, http.MethodPost, srv.URL+"/post", token,
		`{"title": "Direct binding", "body": "Stored body", "status": "published"}`)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d, want 201", res.StatusCode)
	}
	var created struct {
		ID       int64  `json:"id"`
		Title    string `json:"title"`
		Body     string `json:"body"`
		Status   string `json:"status"`
		AuthorID int64  `json:"author_id"`
	}
	if err := json.Unmarshal([]byte(raw), &created); err != nil {
		t.Fatalf("decoding the create reply: %v", err)
	}
	if want := "/post/" + strconv.FormatInt(created.ID, 10); res.Header.Get("Location") != want {
		t.Errorf("Location = %q, want %q", res.Header.Get("Location"), want)
	}
	stored := loadPost(t, app, created.ID)
	if stored.Title != "Direct binding" || stored.Body != "Stored body" || stored.Status != "published" {
		t.Errorf("stored post = %+v, want the submitted values", stored)
	}
	if stored.AuthorID != adminID {
		t.Errorf("stored author id = %d, want the signed-in account %d", stored.AuthorID, adminID)
	}

	// A body the insert scenario rejects: the flat 422 map, nothing written.
	countBefore := postCount(t, app)
	res, raw = callJSON(t, client, http.MethodPost, srv.URL+"/post", token,
		`{"title": "ab", "body": "x", "status": "bogus"}`)
	if res.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("rejected create status = %d, want 422", res.StatusCode)
	}
	var fields map[string][]string
	if err := json.Unmarshal([]byte(raw), &fields); err != nil {
		t.Fatalf("decoding the error map: %v", err)
	}
	if got := fields["title"]; len(got) != 1 || got[0] != "title must be at least 3 characters" {
		t.Errorf(`fields["title"] = %v, want the length note`, got)
	}
	if got := fields["status"]; len(got) != 1 || got[0] != "status must be one of: draft, published" {
		t.Errorf(`fields["status"] = %v, want the one-of note`, got)
	}
	if n := postCount(t, app); n != countBefore {
		t.Errorf("post count after a rejected create = %d, want %d", n, countBefore)
	}

	// A body that is not one JSON object: the record-level $root failure of
	// the same flat map.
	res, raw = callJSON(t, client, http.MethodPost, srv.URL+"/post", token, `{"title": `)
	if res.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("malformed create status = %d, want 422", res.StatusCode)
	}
	fields = nil
	if err := json.Unmarshal([]byte(raw), &fields); err != nil {
		t.Fatalf("decoding the root error map: %v", err)
	}
	if got := fields["$root"]; len(got) != 1 || got[0] != "malformed JSON body" {
		t.Errorf(`fields["$root"] = %v, want the malformed-body note`, got)
	}

	// Keys outside the whitelist assign nothing: only the exact tag names
	// bind, so case variants, the primary key, the author reference and the
	// timestamp columns stay at their server-side values.
	res, raw = callJSON(t, client, http.MethodPost, srv.URL+"/post", token, `{
		"Title": "IGNORED", "title": "Whitelisted",
		"body": "b", "status": "draft",
		"ID": 999, "Id": 998,
		"AuthorID": 42, "author_id": 41,
		"created_at": "2030-01-01T00:00:00Z", "CreatedAt": "2031-01-01T00:00:00Z"
	}`)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("over-posting create status = %d, want 201 (reply %s)", res.StatusCode, raw)
	}
	var pushed struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal([]byte(raw), &pushed); err != nil {
		t.Fatalf("decoding the over-posting reply: %v", err)
	}
	if pushed.ID == 999 || pushed.ID == 998 {
		t.Errorf("created id = %d, the submitted id must not assign", pushed.ID)
	}
	stored = loadPost(t, app, pushed.ID)
	if stored.Title != "Whitelisted" {
		t.Errorf("stored title = %q, the case variant must not assign", stored.Title)
	}
	if stored.AuthorID != adminID {
		t.Errorf("stored author id = %d, the submitted author must not assign", stored.AuthorID)
	}
	if stored.CreatedAt.Year() == 2030 || stored.CreatedAt.Year() == 2031 {
		t.Errorf("stored created_at = %v, the submitted timestamps must not assign", stored.CreatedAt)
	}
}

// TestPostJSONUpdateFlow pins the update side of the JSON API: the row is
// fetched before the submitted values overwrite it, so a key the body left
// out keeps the stored value and no key outside the whitelist rewrites the
// row's server-managed columns. A missing row answers the plain 404, a body
// the update scenario rejects the flat 422 map.
func TestPostJSONUpdateFlow(t *testing.T) {
	h, app := newPostHandler(t)
	client, srv := newBrowser(t, h)
	browserLogin(t, client, srv.URL)
	token := jsonToken(t, client, srv.URL+"/site/csrf")
	adminID := loadAdminID(t, app)

	seed := ar.New[models.Post](app.WithContext(context.Background())).New()
	seed.Title, seed.Body, seed.Status = "Original title", "Original body", models.PostStatusPublished
	seed.AuthorID = adminID
	if err := seed.Save(); err != nil {
		t.Fatalf("seeding the post: %v", err)
	}
	target := srv.URL + "/post/" + strconv.FormatInt(seed.ID, 10)

	// An update that leaves body and author out: the stored values survive,
	// the submitted ones overwrite. This is the fetch-first contract — an
	// absent key is not a zero value.
	res, raw := callJSON(t, client, http.MethodPut, target, token,
		`{"title": "Renamed", "status": "draft", "author_id": 7, "ID": 999, "created_at": "2030-01-01T00:00:00Z"}`)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("update status = %d (reply %s), want 200", res.StatusCode, raw)
	}
	stored := loadPost(t, app, seed.ID)
	if stored.Title != "Renamed" || stored.Status != models.PostStatusDraft {
		t.Errorf("stored post = %+v, want the submitted values", stored)
	}
	if stored.Body != "Original body" {
		t.Errorf("stored body = %q, an omitted key must keep the stored value", stored.Body)
	}
	if stored.AuthorID != adminID {
		t.Errorf("stored author id = %d, the submitted author must not assign", stored.AuthorID)
	}
	if stored.ID == 999 || stored.CreatedAt.Year() == 2030 {
		t.Errorf("stored row = %+v, the submitted id and timestamps must not assign", stored)
	}

	// A missing row: the plain 404.
	res, _ = callJSON(t, client, http.MethodPut, srv.URL+"/post/424242", token, `{"title": "Nowhere"}`)
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("missing row update status = %d, want 404", res.StatusCode)
	}

	// A body the update scenario rejects: the flat 422 map, the row intact.
	res, raw = callJSON(t, client, http.MethodPut, target, token, `{"title": "", "status": "draft"}`)
	if res.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("rejected update status = %d, want 422", res.StatusCode)
	}
	var fields map[string][]string
	if err := json.Unmarshal([]byte(raw), &fields); err != nil {
		t.Fatalf("decoding the error map: %v", err)
	}
	if got := fields["title"]; len(got) != 1 || got[0] != "title cannot be blank" {
		t.Errorf(`fields["title"] = %v, want the blank note`, got)
	}
	stored = loadPost(t, app, seed.ID)
	if stored.Title != "Renamed" {
		t.Errorf("stored title after a rejected update = %q, want %q", stored.Title, "Renamed")
	}
}

// TestPostBrowserWriteFlow walks the browser shape: the compose form
// (draft selected by default), the create submission redirecting to the
// row's page, the rejected submission re-rendered under 422 with the
// messages and entered values echoed and nothing persisted, the edit form
// prefilled from the row, and the update form leaving the body field out —
// the stored body must survive the save.
func TestPostBrowserWriteFlow(t *testing.T) {
	h, app := newPostHandler(t)
	client, srv := newBrowser(t, h)
	browserLogin(t, client, srv.URL)
	adminID := loadAdminID(t, app)

	// The compose form: a fresh record exports the refill values, so the
	// status select defaults to draft.
	body, status := getPage(t, client, srv.URL+"/posts/new")
	if status != http.StatusOK {
		t.Fatalf("compose form status = %d, want 200", status)
	}
	match := csrfField.FindStringSubmatch(body)
	if match == nil {
		t.Fatal("compose form carries no CSRF field")
	}
	if !strings.Contains(body, `value="draft" selected`) {
		t.Error("the fresh form does not default the status select to draft")
	}

	// Create: the redirect points at the row's page, the row carries the
	// submitted values and the signed-in author.
	body, status, location := postForm(t, client, srv.URL+"/posts", url.Values{
		"_csrf": {match[1]}, "title": {"Browser composed"}, "status": {"published"}, "body": {"Composed in the browser."},
	})
	if status != http.StatusFound {
		t.Fatalf("compose status = %d, want 302 (body %s)", status, body)
	}
	created := seedPostID(t, location)
	stored := loadPost(t, app, created)
	if stored.Title != "Browser composed" || stored.Body != "Composed in the browser." || stored.Status != "published" {
		t.Errorf("stored post = %+v, want the submitted values", stored)
	}
	if stored.AuthorID != adminID {
		t.Errorf("stored author id = %d, want the signed-in account", stored.AuthorID)
	}

	// A rejected submission: 422, the messages under their fields, the
	// entered values refilled, nothing persisted.
	countBefore := postCount(t, app)
	body, status, _ = postForm(t, client, srv.URL+"/posts", url.Values{
		"_csrf": {match[1]}, "title": {"ab"}, "status": {"bogus"}, "body": {"This must never be persisted."},
	})
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("rejected compose status = %d, want 422", status)
	}
	for _, want := range []string{
		"title must be at least 3 characters",
		"status must be one of: draft, published",
		"This must never be persisted.",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the rejected form lacks %q", want)
		}
	}
	if n := postCount(t, app); n != countBefore {
		t.Errorf("post count after a rejected compose = %d, want %d", n, countBefore)
	}

	// The edit form comes prefilled from the row.
	body, status = getPage(t, client, srv.URL+"/posts/"+strconv.FormatInt(created, 10)+"/edit")
	if status != http.StatusOK || !strings.Contains(body, `value="Browser composed"`) {
		t.Fatalf("edit form status = %d, prefilled = %v", status, strings.Contains(body, `value="Browser composed"`))
	}
	if !strings.Contains(body, "Composed in the browser.") {
		t.Error("the edit form does not prefill the body")
	}

	// Save without the body field: the update binds over the fetched row,
	// so the stored body survives the form that never carried it.
	_, status, location = postForm(t, client, srv.URL+"/posts/"+strconv.FormatInt(created, 10), url.Values{
		"_csrf": {match[1]}, "title": {"Browser renamed"}, "status": {"draft"},
	})
	if status != http.StatusFound || location != "/posts/"+strconv.FormatInt(created, 10) {
		t.Fatalf("save status = %d location = %q, want 302 the row's page", status, location)
	}
	stored = loadPost(t, app, created)
	if stored.Title != "Browser renamed" || stored.Status != "draft" {
		t.Errorf("stored post = %+v, want the submitted values", stored)
	}
	if stored.Body != "Composed in the browser." {
		t.Errorf("stored body = %q, a form field left out must keep the stored value", stored.Body)
	}
	if stored.AuthorID != adminID {
		t.Errorf("stored author id = %d, the author is server-side state", stored.AuthorID)
	}

	// A rejected save re-renders the edit form with its values refilled.
	body, status, _ = postForm(t, client, srv.URL+"/posts/"+strconv.FormatInt(created, 10), url.Values{
		"_csrf": {match[1]}, "title": {"x"}, "status": {"draft"},
	})
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("rejected save status = %d, want 422", status)
	}
	if !strings.Contains(body, "title must be at least 3 characters") || !strings.Contains(body, `value="x"`) {
		t.Error("the rejected save renders neither the message nor the refilled title")
	}
}

// loadAdminID reports the seeded admin account's row id.
func loadAdminID(t *testing.T, app *base.Application) int64 {
	t.Helper()
	user, err := ar.New[models.User](app.WithContext(context.Background())).Where("username = ?", "admin").One()
	if err != nil {
		t.Fatalf("loading the admin account: %v", err)
	}
	return user.ID
}

// postCount counts the post rows.
func postCount(t *testing.T, app *base.Application) int64 {
	t.Helper()
	n, err := ar.New[models.Post](app.WithContext(context.Background())).Count()
	if err != nil {
		t.Fatalf("counting posts: %v", err)
	}
	return n
}

// seedPostID extracts the row id a redirect Location points at.
func seedPostID(t *testing.T, location string) int64 {
	t.Helper()
	id, err := strconv.ParseInt(strings.TrimPrefix(location, "/posts/"), 10, 64)
	if err != nil {
		t.Fatalf("parsing the redirect location %q: %v", location, err)
	}
	return id
}
