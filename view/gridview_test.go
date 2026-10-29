package view_test

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/goyii2/framework/view"
)

// gridPost is the row type of the test grid: the generic item an action's
// Item closure would hand the widget.
type gridPost struct {
	ID    int
	Title string
}

func gridPosts() []gridPost {
	return []gridPost{
		{ID: 1, Title: "Hello"},
		{ID: 2, Title: "World"},
	}
}

// grid is a two-column grid over the test posts: a sort-toggling ID column
// and an escaped Title column.
func grid(columns []view.DataColumn) view.GridView {
	posts := gridPosts()
	if columns == nil {
		columns = []view.DataColumn{
			{Attr: "id", Sortable: true, Value: func(item any) any { return item.(gridPost).ID }},
			{Attr: "title", Label: "Title", Value: func(item any) any { return item.(gridPost).Title }},
		}
	}
	return view.GridView{
		Columns:   columns,
		ItemCount: len(posts),
		Item:      func(i int) any { return posts[i] },
	}
}

func TestGridViewRendersTableRows(t *testing.T) {
	got := render(t, grid(nil).Render())

	for _, want := range []string{
		`<table class="table table-striped table-bordered grid-view">`,
		// sortable header links; a missing Label falls back to Attr
		`<th class="sortable"><a href="?sort=id">id</a></th>`,
		"<th>Title</th>",
		// one td per column, values resolved through Item + Value
		"<td>1</td>", "<td>Hello</td>",
		"<td>2</td>", "<td>World</td>",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("grid missing %q:\n%s", want, got)
		}
	}
	// two body rows (the header tr lives in thead, outside tbody)
	if n := strings.Count(got[strings.Index(got, "<tbody>"):], "<tr>"); n != 2 {
		t.Errorf("rendered %d body rows, want 2:\n%s", n, got)
	}
	// no column declared a filter: no form wrapper, no filter row
	for _, unwanted := range []string{"<form", "filters"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("unfiltered grid renders %q:\n%s", unwanted, got)
		}
	}
}

func TestGridViewSortLinksFollowSortParameter(t *testing.T) {
	base := grid(nil)

	// unsorted column links ascending
	got := render(t, base.Render())
	if !strings.Contains(got, `<th class="sortable"><a href="?sort=id">id</a></th>`) {
		t.Errorf("unsorted header does not link ascending:\n%s", got)
	}

	// the sorted column shows its state and links the flip
	asc := base
	asc.Sort = "-title,id"
	got = render(t, asc.Render())
	for _, want := range []string{
		`<th class="sortable asc"><a href="?sort=-id">id</a></th>`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("ascending header missing %q:\n%s", want, got)
		}
	}

	desc := base
	desc.Sort = "-id"
	got = render(t, desc.Render())
	if !strings.Contains(got, `<th class="sortable desc"><a href="?sort=id">id</a></th>`) {
		t.Errorf("descending header not marked or not flipping:\n%s", got)
	}

	// a custom URL builder replaces the default query-string target
	// (templ escapes the ampersand inside the attribute)
	custom := base
	custom.SortURL = func(sortValue string) string { return "/post?status=1&sort=" + sortValue }
	got = render(t, custom.Render())
	if !strings.Contains(got, `<a href="/post?status=1&amp;sort=id">id</a>`) {
		t.Errorf("custom sort URL missing:\n%s", got)
	}

	// a non-sortable column renders a plain header with no link
	plain := view.GridView{
		Columns:   []view.DataColumn{{Attr: "title", Value: func(any) any { return "x" }}},
		ItemCount: 1, Item: func(int) any { return nil },
	}
	got = render(t, plain.Render())
	if strings.Contains(got, "<a ") {
		t.Errorf("unsortable column renders a sort link:\n%s", got)
	}
}

func TestGridViewEscapesByDefaultHTMLOptIn(t *testing.T) {
	got := render(t, grid([]view.DataColumn{
		{Attr: "title", Value: func(any) any { return `<script>alert("x")</script>` }},
		{Attr: "body", Format: view.FormatHTML, Value: func(any) any { return `<b>bold</b>` }},
		{Attr: "none", Value: nil},
		{Attr: "badge", Value: func(any) any { return text(`<span class="badge">new</span>`) }},
	}).Render())

	if strings.Contains(got, "<script>") {
		t.Fatalf("unescaped script tag in default-format cell:\n%s", got)
	}
	for _, want := range []string{
		// the default format escapes the value
		`<td>&lt;script&gt;alert(&#34;x&#34;)&lt;/script&gt;</td>`,
		// FormatHTML passes raw markup through — explicit opt-in
		`<td><b>bold</b></td>`,
		// a nil Value renders an empty cell
		"<th>none</th>", "<td></td>",
		// component values render inline
		`<span class="badge">new</span>`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("grid missing %q:\n%s", want, got)
		}
	}
}

func TestGridViewFilterRowEchoesValues(t *testing.T) {
	g := grid([]view.DataColumn{
		{Attr: "id", Sortable: true, Value: func(item any) any { return item.(gridPost).ID }},
		{Attr: "title", Value: func(item any) any { return item.(gridPost).Title },
			Filter: &view.ColumnFilter{Type: view.FieldText}},
		{Attr: "status", Value: func(any) any { return "ok" },
			Filter: &view.ColumnFilter{Type: view.FieldSelect, Options: []view.FieldOption{
				{Value: "", Label: "All"},
				{Value: "1", Label: "Active"},
			}}},
	})
	g.FilterValues = map[string]string{"title": "hello", "status": "1"}
	got := render(t, g.Render())

	for _, want := range []string{
		// any filter turns the table into a GET form
		`<form method="get">`,
		`<tr class="filters">`,
		// the unfiltered leading column keeps the row aligned with an empty cell
		"<td></td>",
		// text input named after the column with the echoed value
		`<input type="text" name="title" class="form-control" value="hello">`,
		// select marks the echoed choice
		`<select name="status" class="form-control">`,
		`<option value="">All</option>`,
		`<option value="1" selected>Active</option>`,
		`</select>`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("filter row missing %q:\n%s", want, got)
		}
	}
}

func TestGridViewFilterValuesFromRequestHelper(t *testing.T) {
	r := httptest.NewRequest("GET", "/post?title=hi&status=1&extra=ignored", nil)
	got := view.QueryValues(r, "title", "status")
	want := map[string]string{"title": "hi", "status": "1"}
	if len(got) != len(want) {
		t.Fatalf("QueryValues = %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("QueryValues[%q] = %q, want %q", k, got[k], v)
		}
	}
	if view.QueryValues(r) != nil {
		t.Error("QueryValues with no names should return nil")
	}
}

func TestGridViewEmptyState(t *testing.T) {
	g := grid(nil)
	g.ItemCount = 0
	g.Item = nil
	got := render(t, g.Render())
	if !strings.Contains(got, `<tr><td colspan="2" class="empty">No results found.</td></tr>`) {
		t.Errorf("default empty placeholder missing:\n%s", got)
	}

	g.EmptyText = "没有文章。"
	got = render(t, g.Render())
	if !strings.Contains(got, `class="empty">没有文章。</td>`) {
		t.Errorf("custom empty text missing:\n%s", got)
	}
	if strings.Contains(got, "<td>1</td>") {
		t.Errorf("empty grid renders body rows:\n%s", got)
	}
}
