package view_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/goyii2/framework/view"
)

func TestLinkPagerDefaultWindow(t *testing.T) {
	p := view.LinkPager{
		Pagination: view.Pagination{TotalCount: 100, PerPage: 10, Page: 5},
	}
	got := render(t, p.Render())

	// 10 pages fit the default button budget: all numbers, current active.
	for _, want := range []string{
		"<ul class=\"pagination\">",
		`<li class="prev"><a href="?page=4">«</a></li>`,
		`<li class="active"><a href="?page=5">5</a></li>`,
		`<li class="next"><a href="?page=6">»</a></li>`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("pager missing %q:\n%s", want, got)
		}
	}
	if n := strings.Count(got, "<li"); n != 12 {
		t.Errorf("rendered %d buttons, want 10 numbers + prev + next", n)
	}
}

func TestLinkPagerBoundariesDisableSteps(t *testing.T) {
	first := render(t, view.LinkPager{Pagination: view.Pagination{TotalCount: 30, PerPage: 10, Page: 1}}.Render())
	if !strings.Contains(first, `<li class="prev disabled"><span>«</span></li>`) {
		t.Errorf("first page prev not disabled:\n%s", first)
	}
	if !strings.Contains(first, `<li class="next"><a href="?page=2">»</a></li>`) {
		t.Errorf("first page next not linked:\n%s", first)
	}

	last := render(t, view.LinkPager{Pagination: view.Pagination{TotalCount: 30, PerPage: 10, Page: 3}}.Render())
	if !strings.Contains(last, `<li class="next disabled"><span>»</span></li>`) {
		t.Errorf("last page next not disabled:\n%s", last)
	}
}

func TestLinkPagerSlidingWindow(t *testing.T) {
	got := render(t, view.LinkPager{
		Pagination:     view.Pagination{TotalCount: 1000, PerPage: 10, Page: 50},
		MaxButtonCount: 5,
	}.Render())
	for _, want := range []string{
		`<a href="?page=48">48</a>`,
		`<a href="?page=49">49</a>`,
		`<li class="active"><a href="?page=50">50</a></li>`,
		`<a href="?page=51">51</a>`,
		`<a href="?page=52">52</a>`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("window missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, ">47<") || strings.Contains(got, ">53<") {
		t.Errorf("window exceeds MaxButtonCount 5:\n%s", got)
	}
}

func TestLinkPagerCustomURLAndLabels(t *testing.T) {
	got := render(t, view.LinkPager{
		Pagination:     view.Pagination{TotalCount: 40, PerPage: 10, Page: 2},
		URL:            func(page int) string { return "/post?page=" + strconv.Itoa(page) },
		PrevPageLabel:  "Previous",
		NextPageLabel:  "Next",
		FirstPageLabel: "First",
		LastPageLabel:  "Last",
	}.Render())
	for _, want := range []string{
		`<li class="first"><a href="/post?page=1">First</a></li>`,
		`<li class="prev"><a href="/post?page=1">Previous</a></li>`,
		`<li class="next"><a href="/post?page=3">Next</a></li>`,
		`<li class="last"><a href="/post?page=4">Last</a></li>`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("pager missing %q:\n%s", want, got)
		}
	}
}

func TestLinkPagerHideOnSinglePage(t *testing.T) {
	p := view.LinkPager{
		Pagination:       view.Pagination{TotalCount: 5, PerPage: 10, Page: 1},
		HideOnSinglePage: true,
	}
	if got := render(t, p.Render()); got != "" {
		t.Errorf("hidden pager rendered %q, want empty", got)
	}
	visible := view.LinkPager{Pagination: view.Pagination{TotalCount: 5, PerPage: 10, Page: 1}}
	if got := render(t, visible.Render()); !strings.Contains(got, "<ul class=\"pagination\">") {
		t.Errorf("single-page pager should still render by default:\n%s", got)
	}
}

func TestBreadcrumbs(t *testing.T) {
	got := render(t, view.Breadcrumbs{Links: []view.Crumb{
		{Label: "Home", URL: "/"},
		{Label: "Posts", URL: "/post"},
		{Label: "Hello <goyii>"},
	}}.Render())

	for _, want := range []string{
		"<ul class=\"breadcrumb\">",
		`<a href="/">Home</a>`,
		`<a href="/post">Posts</a>`,
		`<span class="divider">/</span>`,
		// the unlinked current stop is highlighted and escaped
		`<li class="active">Hello &lt;goyii&gt;</li>`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("breadcrumbs missing %q:\n%s", want, got)
		}
	}
	if n := strings.Count(got, "divider"); n != 2 {
		t.Errorf("divider count = %d, want 2 (after each linked stop)", n)
	}
}

func TestBreadcrumbsEscapeLabels(t *testing.T) {
	got := render(t, view.Breadcrumbs{Links: []view.Crumb{
		{Label: "<b>bold</b>", URL: "/x?a=1&b=2"},
	}}.Render())
	if !strings.Contains(got, "&lt;b&gt;bold&lt;/b&gt;") {
		t.Errorf("label not escaped:\n%s", got)
	}
}

func TestMenuFlatAndNested(t *testing.T) {
	got := render(t, view.Menu{Items: []view.MenuItem{
		{Label: "Home", URL: "/", Active: true},
		{Label: "Admin", Items: []view.MenuItem{
			{Label: "Users", URL: "/admin/user"},
			{Label: "Settings", URL: "/admin/settings", Active: true},
		}},
	}}.Render())

	// templ may place whitespace inside the li; assert open-element
	// prefixes rather than exact full elements.
	for _, want := range []string{
		`<li class="active"><a href="/">Home</a>`,
		`<span>Admin</span>`, // linkless parent renders as heading
		`<a href="/admin/user">Users</a>`,
		`<li class="active"><a href="/admin/settings">Settings</a>`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("menu missing %q:\n%s", want, got)
		}
	}
	if n := strings.Count(got, "<ul>"); n != 2 {
		t.Errorf("nested menu renders %d lists, want 2", n)
	}
}

func TestMenuSanitizesURLSchemes(t *testing.T) {
	got := render(t, view.Menu{Items: []view.MenuItem{
		{Label: "evil", URL: "javascript:alert(1)"},
	}}.Render())
	if strings.Contains(got, "javascript:") {
		t.Errorf("javascript: URL passed through:\n%s", got)
	}
	if !strings.Contains(got, "TemplFailedSanitizationURL") {
		t.Errorf("unsafe URL not replaced by the sanitization marker:\n%s", got)
	}
}

func TestDetailView(t *testing.T) {
	got := render(t, view.DetailView{Rows: []view.DetailRow{
		{Label: "ID", Value: 42},
		{Label: "Title", Value: "Hello & <welcome>"},
		{Label: "Notes", Value: nil},
		{Label: "Badge", Value: text(`<span class="badge">new</span>`)},
	}}.Render())

	for _, want := range []string{
		`<table class="table table-striped table-bordered detail-view">`,
		"<th>ID</th>", "<td>42</td>",
		// plain values are escaped
		"<td>Hello &amp; &lt;welcome&gt;</td>",
		// nil renders an empty cell
		"<th>Notes</th><td></td>",
		// component values render inline untouched
		`<span class="badge">new</span>`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("detail view missing %q:\n%s", want, got)
		}
	}
	if n := strings.Count(got, "<tr>"); n != 4 {
		t.Errorf("rendered %d rows, want 4", n)
	}
}

func TestWidgetsComposeInsideShell(t *testing.T) {
	page := view.Page{
		Title:   "Posts",
		Nav:     view.Breadcrumbs{Links: []view.Crumb{{Label: "Home", URL: "/"}, {Label: "Posts"}}}.Render(),
		Content: view.DetailView{Rows: []view.DetailRow{{Label: "Total", Value: 100}}}.Render(),
		Footer:  view.LinkPager{Pagination: view.Pagination{TotalCount: 100, PerPage: 10}}.Render(),
	}
	got := render(t, view.Shell(page))
	for _, want := range []string{"<title>Posts</title>", "breadcrumb", "detail-view", "pagination"} {
		if !strings.Contains(got, want) {
			t.Errorf("composed page missing %q", want)
		}
	}
}
