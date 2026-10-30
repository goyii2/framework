// Tests of the backend's post management: the module guard owns the CRUD
// routes like every /backend path, an admin runs the whole create/edit/
// delete cycle through the real forms (every link and redirect resolved by
// route name), and a payload the model rejects re-renders under 422 with
// the messages echoed and nothing persisted.
package backend

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/goyii2/framework/db/ar"
	"github.com/goyii2/framework/examples/advanced/common/models"
)

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

func TestBackendPostManagement(t *testing.T) {
	h, app := newBackendHandler(t)
	ctx := app.WithContext(context.Background())

	// The module guard owns the new routes too: a guest is pointed at the
	// login page, not the listing.
	guest, srv := newBrowser(t, h)
	res, err := guest.Get(srv.URL + "/backend/posts")
	if err != nil {
		t.Fatalf("GET /backend/posts: %v", err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusFound || res.Header.Get("Location") != "/login" {
		t.Fatalf("guest listing status = %d location = %q, want 302 /login", res.StatusCode, res.Header.Get("Location"))
	}

	admin, srv := newBrowser(t, h)
	login(t, admin, srv.URL, "admin", "admin123")

	// The empty listing renders inside the dark frame, with the sidebar's
	// Posts entry pointing at the page itself.
	body, status := get(t, admin, srv.URL+"/backend/posts")
	if status != http.StatusOK {
		t.Fatalf("listing status = %d", status)
	}
	if !strings.Contains(body, "backend partition") {
		t.Fatal("the listing does not render in the backend's frame")
	}
	if !strings.Contains(body, `href="/backend/posts"`) {
		t.Fatal("the sidebar shows no posts entry")
	}
	if !strings.Contains(body, "No posts yet") {
		t.Fatal("the empty listing shows no empty note")
	}

	// Compose: the form page, then the submission.
	body, status = get(t, admin, srv.URL+"/backend/posts/new")
	if status != http.StatusOK || !strings.Contains(body, `name="title"`) {
		t.Fatalf("compose form status = %d", status)
	}
	match := csrfField.FindStringSubmatch(body)
	if match == nil {
		t.Fatal("compose form carries no CSRF field")
	}
	token := match[1]

	body, status, location := postForm(t, admin, srv.URL+"/backend/posts", url.Values{
		"_csrf":  {token},
		"title":  {"First managed post"},
		"status": {"published"},
		"body":   {"Written from the backend."},
	})
	if status != http.StatusFound || location != "/backend/posts" {
		t.Fatalf("compose status = %d location = %q, want 302 /backend/posts", status, location)
	}

	// The listing shows the row, and its View action points into the
	// frontend's public page — the cross-partition address the named route
	// built.
	created, err := ar.New[models.Post](ctx).Where("title = ?", "First managed post").One()
	if err != nil {
		t.Fatalf("loading the created post: %v", err)
	}
	viewURL := "/posts/" + strconv.FormatInt(created.ID, 10)
	body, status = get(t, admin, srv.URL+"/backend/posts")
	if status != http.StatusOK {
		t.Fatalf("listing status = %d", status)
	}
	for _, want := range []string{
		"First managed post", ">admin</td>", "published",
		`href="` + viewURL + `"`,
		`href="/backend/posts/` + strconv.FormatInt(created.ID, 10) + `/edit"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the listing lacks %q", want)
		}
	}

	// Edit: the form comes prefilled, the save renames the row.
	body, status = get(t, admin, srv.URL+"/backend/posts/"+strconv.FormatInt(created.ID, 10)+"/edit")
	if status != http.StatusOK || !strings.Contains(body, `value="First managed post"`) {
		t.Fatalf("edit form status = %d, prefilled = %v", status, strings.Contains(body, `value="First managed post"`))
	}
	body, status, location = postForm(t, admin, srv.URL+"/backend/posts/"+strconv.FormatInt(created.ID, 10), url.Values{
		"_csrf":  {token},
		"title":  {"Renamed post"},
		"status": {"draft"},
		"body":   {"Edited from the backend."},
	})
	if status != http.StatusFound || location != "/backend/posts" {
		t.Fatalf("save status = %d location = %q, want 302 /backend/posts", status, location)
	}
	body, _ = get(t, admin, srv.URL+"/backend/posts")
	if !strings.Contains(body, "Renamed post") || strings.Contains(body, "First managed post") {
		t.Error("the listing does not reflect the rename")
	}
	if strings.Contains(body, `href="`+viewURL+`"`) {
		t.Error("a draft row keeps the view-on-site link")
	}

	// Delete: the row disappears from the listing and the database.
	body, status, location = postForm(t, admin, srv.URL+"/backend/posts/"+strconv.FormatInt(created.ID, 10)+"/delete", url.Values{
		"_csrf": {token},
	})
	if status != http.StatusFound || location != "/backend/posts" {
		t.Fatalf("delete status = %d location = %q, want 302 /backend/posts", status, location)
	}
	body, _ = get(t, admin, srv.URL+"/backend/posts")
	if strings.Contains(body, "Renamed post") {
		t.Error("the deleted post still renders in the listing")
	}
	if n, err := ar.New[models.Post](ctx).Count(); err != nil || n != 0 {
		t.Errorf("post count after delete = %d (err %v), want 0", n, err)
	}
}

func TestBackendPostValidationEchoesWithoutPersisting(t *testing.T) {
	h, app := newBackendHandler(t)
	admin, srv := newBrowser(t, h)
	login(t, admin, srv.URL, "admin", "admin123")

	body, _ := get(t, admin, srv.URL+"/backend/posts/new")
	match := csrfField.FindStringSubmatch(body)
	if match == nil {
		t.Fatal("compose form carries no CSRF field")
	}

	// A blank title fails the insert scenario: the form re-renders under 422
	// with the message below the field and the entered values refilled.
	body, status, _ := postForm(t, admin, srv.URL+"/backend/posts", url.Values{
		"_csrf":  {match[1]},
		"title":  {"ab"}, // under the 3-character minimum, a length rejection
		"status": {"bogus"},
		"body":   {"This must never be persisted."},
	})
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("rejected submission status = %d, want 422", status)
	}
	if !strings.Contains(body, "title must be at least 3 characters") {
		t.Error("the rejected form renders no title message")
	}
	if !strings.Contains(body, "status must be one of: draft, published") {
		t.Error("the rejected form renders no status message")
	}
	if !strings.Contains(body, "This must never be persisted.") {
		t.Error("the submitted body is not refilled")
	}

	// The rejected save reached no database.
	n, err := ar.New[models.Post](app.WithContext(context.Background())).Count()
	if err != nil {
		t.Fatalf("counting posts: %v", err)
	}
	if n != 0 {
		t.Errorf("post count after a rejected save = %d, want 0", n)
	}
}
