// Tests of the nested system submodule: /backend/system assembles below the
// backend module (the parent registration the nesting seam requires), its
// user listing serves through the convention fallback — no route is
// registered anywhere for it — and the ancestor module's guard wraps the
// submodule's paths exactly like the backend's own. The listing's DataProvider
// pages the accounts: the page parameter changes the result set, and the
// seeded demo accounts are too few to page, so the test grows the table.
package backend

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/goyii2/framework/db/ar"
	"github.com/goyii2/framework/examples/advanced/common/models"
)

func TestSystemSubmoduleGuardInheritsBackendChain(t *testing.T) {
	h, _ := newBackendHandler(t)

	// Guest: the submodule's path points at the login page like every
	// /backend path — the ancestor module's login redirect wraps nested
	// modules' routes outermost.
	guest, srv := newBrowser(t, h)
	for _, target := range []string{srv.URL + "/backend/", srv.URL + "/backend/system/user/list"} {
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

	// Authenticated without the admin role: refused with 403 below the
	// submodule prefix exactly like the parent's own dashboard.
	demo, srv := newBrowser(t, h)
	login(t, demo, srv.URL, "demo", "demo123")
	body, status := get(t, demo, srv.URL+"/backend/system/user/list")
	if status != http.StatusForbidden {
		t.Fatalf("demo /backend/system/user/list status = %d, want 403", status)
	}
	if strings.Contains(body, "Users") {
		t.Error("the 403 reply must not render the user listing")
	}

	// Admin: the listing answers inside the backend partition's dark frame,
	// with the sidebar's System entry pointing at the conventional route and
	// carrying the active marker.
	admin, srv := newBrowser(t, h)
	login(t, admin, srv.URL, "admin", "admin123")
	body, status = get(t, admin, srv.URL+"/backend/system/user/list")
	if status != http.StatusOK {
		t.Fatalf("admin /backend/system/user/list status = %d, want 200", status)
	}
	for _, want := range []string{
		"<h1>Users</h1>",
		"backend partition",
		`href="/backend/system/user/list"`,
		`class="active"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the listing page lacks %q", want)
		}
	}
	if strings.Contains(body, "frontend partition") {
		t.Error("the listing renders in the frontend's frame")
	}
}

func TestSystemUserListingPagesThroughConventionRoute(t *testing.T) {
	h, app := newBackendHandler(t)

	// The route is conventional: no named route exists for it, so the
	// sidebar's and the pager's addresses were assembled by the convention
	// fallback of reverse URL generation, not looked up in a route table.
	if h.URLs().Has("backend/system/user/list") {
		t.Fatal("backend/system/user/list must not be a registered route — the submodule serves by convention")
	}

	// The demo accounts alone cannot fill a page (the listing fixes ten per
	// page), so the table grows past one page: nine more accounts make
	// eleven rows — two pages, the second holding the oldest row alone.
	ctx := app.WithContext(context.Background())
	for i := 1; i <= 9; i++ {
		user := ar.New[models.User](ctx).New()
		user.Username = fmt.Sprintf("bulk%02d", i)
		if err := user.SetPassword("bulk-pass-123"); err != nil {
			t.Fatalf("hashing %s's password: %v", user.Username, err)
		}
		if err := user.Save(); err != nil {
			t.Fatalf("seeding %s's account: %v", user.Username, err)
		}
	}

	admin, srv := newBrowser(t, h)
	login(t, admin, srv.URL, "admin", "admin123")

	// Page one (the default, newest first): the nine bulk accounts and demo
	// fill it; the oldest row, admin, waits on page two. The pager offers
	// the convention-built address of that next page.
	body, status := get(t, admin, srv.URL+"/backend/system/user/list")
	if status != http.StatusOK {
		t.Fatalf("page 1 status = %d, want 200", status)
	}
	if !strings.Contains(body, "11 accounts · page 1 of 2") {
		t.Error("page 1 does not report the totals the provider computed")
	}
	for _, want := range []string{">bulk01</td>", ">bulk09</td>", ">demo</td>"} {
		if !strings.Contains(body, want) {
			t.Errorf("page 1 lacks the row %q", want)
		}
	}
	if strings.Contains(body, ">admin</td>") {
		t.Error("page 1 renders a row that belongs to page 2")
	}
	if !strings.Contains(body, `href="/backend/system/user/list?page=2"`) {
		t.Error("page 1 offers no link to page 2")
	}

	// Page two: the page parameter changed the result set — the oldest row
	// alone, none of page one's rows repeated.
	body, status = get(t, admin, srv.URL+"/backend/system/user/list?page=2")
	if status != http.StatusOK {
		t.Fatalf("page 2 status = %d, want 200", status)
	}
	if !strings.Contains(body, "11 accounts · page 2 of 2") {
		t.Error("page 2 does not report the totals the provider computed")
	}
	if !strings.Contains(body, ">admin</td>") {
		t.Error("page 2 lacks the oldest row")
	}
	for _, gone := range []string{">demo</td>", ">bulk01</td>", ">bulk09</td>"} {
		if strings.Contains(body, gone) {
			t.Errorf("page 2 repeats page 1's row %q", gone)
		}
	}
}
