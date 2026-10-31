// Tests of the frontend's public content pages: an anonymous visitor sees
// the published posts — the listing and one post's detail — drafts stay
// invisible, and the pages carry no guard of their own.
package frontend

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/goyii2/framework/db/ar"
	"github.com/goyii2/framework/examples/advanced/common/models"
)

func TestFrontendShowsPublishedPostsToGuests(t *testing.T) {
	h, app := newFrontendHandler(t)
	ctx := app.WithContext(context.Background())

	// Two published posts and one draft, written by the demo account.
	demo, err := ar.New[models.User](ctx).Where("username = ?", "demo").One()
	if err != nil {
		t.Fatalf("loading the demo account: %v", err)
	}
	for _, p := range []struct{ title, status string }{
		{"Public announcement", models.PostStatusPublished},
		{"Second story", models.PostStatusPublished},
		{"Secret draft", models.PostStatusDraft},
	} {
		post := ar.New[models.Post](ctx).New()
		post.Title = p.title
		post.Body = "Body of " + p.title + "."
		post.Status = p.status
		post.AuthorID = demo.ID
		if err := post.Save(); err != nil {
			t.Fatalf("seeding %q: %v", p.title, err)
		}
	}
	published, err := ar.New[models.Post](ctx).Where("title = ?", "Public announcement").One()
	if err != nil {
		t.Fatalf("loading the published post: %v", err)
	}
	draft, err := ar.New[models.Post](ctx).Where("title = ?", "Secret draft").One()
	if err != nil {
		t.Fatalf("loading the draft: %v", err)
	}
	publishedURL := "/posts/" + strconv.FormatInt(published.ID, 10)
	draftURL := "/posts/" + strconv.FormatInt(draft.ID, 10)

	guest, srv := newBrowser(t, h)

	// The listing: the navigation's posts entry, both published cards, no
	// draft — inside the light frontend frame.
	body, status := get(t, guest, srv.URL+"/posts")
	if status != http.StatusOK {
		t.Fatalf("/posts status = %d", status)
	}
	if !strings.Contains(body, "frontend partition") {
		t.Fatal("the listing does not render in the frontend's frame")
	}
	if !strings.Contains(body, `href="/posts"`) {
		t.Fatal("the navigation shows no posts entry")
	}
	if !strings.Contains(body, "Public announcement") || !strings.Contains(body, "Second story") {
		t.Fatal("the listing misses a published post")
	}
	if strings.Contains(body, "Secret draft") {
		t.Fatal("the listing leaks a draft")
	}
	if !strings.Contains(body, `href="`+publishedURL+`"`) {
		t.Fatal("the published card links to no detail page")
	}

	// The detail page: title, byline, body and the way back — reached at
	// the address the named route built.
	body, status = get(t, guest, srv.URL+publishedURL)
	if status != http.StatusOK {
		t.Fatalf("%s status = %d", publishedURL, status)
	}
	for _, want := range []string{
		"Public announcement", "Body of Public announcement.",
		"by demo", `href="/posts"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the detail page lacks %q", want)
		}
	}

	// The draft and an unknown id answer 404 — the public site resolves
	// published rows only.
	for _, target := range []string{draftURL, "/posts/999999"} {
		body, status = get(t, guest, srv.URL+target)
		if status != http.StatusNotFound {
			t.Fatalf("%s status = %d, want 404", target, status)
		}
		if strings.Contains(body, "Secret draft") {
			t.Errorf("%s leaks the draft's content", target)
		}
	}
}

// TestFrontendPostViewRendersPermalink pins the absolute-URL touchpoint:
// with host info wired on the route table (the test's stand-in for main.go's
// SetHostInfo line), the detail page's permalink renders the scheme, host
// and the named route's path while the rest of the page — back link,
// navigation — stays relative.
func TestFrontendPostViewRendersPermalink(t *testing.T) {
	h, app := newFrontendHandler(t)
	h.URLs().SetHostInfo("https://frontend.example.com")
	ctx := app.WithContext(context.Background())

	demo, err := ar.New[models.User](ctx).Where("username = ?", "demo").One()
	if err != nil {
		t.Fatalf("loading the demo account: %v", err)
	}
	post := ar.New[models.Post](ctx).New()
	post.Title, post.Body, post.Status = "Permalink story", "Body of the permalink story.", models.PostStatusPublished
	post.AuthorID = demo.ID
	if err := post.Save(); err != nil {
		t.Fatalf("seeding the post: %v", err)
	}
	permalink := "https://frontend.example.com/posts/" + strconv.FormatInt(post.ID, 10)

	guest, srv := newBrowser(t, h)
	body, status := get(t, guest, srv.URL+"/posts/"+strconv.FormatInt(post.ID, 10))
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
	// navigation's posts entry address the site itself.
	for _, rel := range []string{`href="/posts"`, `href="/"`} {
		if !strings.Contains(body, rel) {
			t.Errorf("the page lacks the relative link %q", rel)
		}
	}
}
