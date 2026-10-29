package view_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/a-h/templ"
	"github.com/goyii2/framework/view"
)

// postItemView renders one post item, standing in for a .templ component.
func postItemView(item any) templ.Component {
	return text("<p>" + item.(gridPost).Title + "</p>")
}

func TestListViewRendersItemsSummaryAndPager(t *testing.T) {
	posts := gridPosts()
	got := render(t, view.ListView{
		Pagination: view.Pagination{TotalCount: 42, PerPage: 10, Page: 2},
		ItemCount:  len(posts),
		Item:       func(i int) any { return posts[i] },
		ItemView:   postItemView,
		// the pager's Pagination comes from the list; only the rest is
		// configured here
		Pager: view.LinkPager{URL: func(page int) string { return "/post?page=" + strconv.Itoa(page) }},
	}.Render())

	for _, want := range []string{
		`<div class="list-view">`,
		// summary carries the current-page range and the total
		`<div class="summary">Showing <b>11-12</b> of <b>42</b> items.</div>`,
		// items render in order through Item + ItemView
		`<div class="item"><p>Hello</p></div>`,
		`<div class="item"><p>World</p></div>`,
		// the reused LinkPager renders under the items with the list's own
		// pagination
		`<li class="prev"><a href="/post?page=1">«</a></li>`,
		`<li class="active"><a href="/post?page=2">2</a></li>`,
		`<li class="next"><a href="/post?page=3">»</a></li>`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("list missing %q:\n%s", want, got)
		}
	}
}

func TestListViewEmptyState(t *testing.T) {
	got := render(t, view.ListView{
		Pagination: view.Pagination{TotalCount: 0, PerPage: 10, Page: 1},
		ItemCount:  0,
		ItemView:   postItemView,
	}.Render())

	for _, want := range []string{
		`<div class="summary">Total <b>0</b> items.</div>`,
		`<div class="empty">No results found.</div>`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("empty list missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, `class="item"`) {
		t.Errorf("empty list renders item wrappers:\n%s", got)
	}

	custom := view.ListView{
		Pagination: view.Pagination{TotalCount: 0},
		EmptyText:  "没有文章。",
	}
	if got := render(t, custom.Render()); !strings.Contains(got, `<div class="empty">没有文章。</div>`) {
		t.Errorf("custom empty text missing:\n%s", got)
	}
}

func TestListViewPagerHidesOnSinglePage(t *testing.T) {
	l := view.ListView{
		Pagination: view.Pagination{TotalCount: 5, PerPage: 10, Page: 1},
		ItemCount:  5,
		Item:       func(int) any { return nil },
		ItemView:   func(any) templ.Component { return text("<p>x</p>") },
		Pager:      view.LinkPager{HideOnSinglePage: true},
	}
	got := render(t, l.Render())
	if strings.Contains(got, "pagination") {
		t.Errorf("hidden pager still renders:\n%s", got)
	}
	if !strings.Contains(got, `<div class="summary">Showing <b>1-5</b> of <b>5</b> items.</div>`) {
		t.Errorf("summary missing on the single page:\n%s", got)
	}
}
