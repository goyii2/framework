package view

import (
	"strconv"

	"github.com/a-h/templ"
)

// ListView renders the item list of a prepared dataprovider.Result: a
// summary line (total and current-page range), one rendered item per row
// and the pagination bar. Like GridView it is a pure renderer — it never
// queries; the action prepares the result and hands its pieces in:
//
//	params, _ := dataprovider.FromRequest(c.Request(), "created_at")
//	res := provider.Prepare(ctx, params)
//	list := view.ListView{
//		Pagination: res.Pagination(),
//		ItemCount:  len(res.Items),
//		Item:       func(i int) any { return res.Items[i] },
//		ItemView:   func(item any) templ.Component { return postRow(item.(Post)) },
//		Pager:      view.LinkPager{URL: func(page int) string { return url.To("post/index", url.P{"page": page}) }},
//	}
//
// ItemView is a plain func(any) templ.Component so it works both with templ
// components (a generated function call) and with hand-built components.
type ListView struct {
	// Pagination is the result's paging metadata: pass res.Pagination().
	// It feeds both the summary line and the pager.
	Pagination Pagination

	// ItemCount is the number of items on the current page:
	// len(res.Items).
	ItemCount int

	// Item returns page row i as any — the bridge from the result's generic
	// slice, same contract as GridView's.
	Item func(i int) any

	// ItemView renders one item of the page. Required whenever ItemCount is
	// above zero; without it the list renders its summary and pager only.
	ItemView func(item any) templ.Component

	// EmptyText renders when ItemCount is zero; "" means
	// "No results found.".
	EmptyText string

	// Pager renders the pagination bar at the bottom. Its Pagination is
	// always taken from the list's own — both describe the same result — so
	// configure only the rest here (URL callback, labels,
	// HideOnSinglePage...).
	Pager LinkPager
}

// listViewView is the precomputed list the template renders.
type listViewView struct {
	// showRange reports a non-empty result, whose summary carries the
	// current-page range; an empty one summarizes as a zero total.
	showRange  bool
	rangeLabel string
	totalLabel string
	empty      bool
	emptyText  string
	items      []templ.Component
	pager      templ.Component
}

func (l ListView) prepared() listViewView {
	v := listViewView{
		showRange:  l.Pagination.TotalCount > 0,
		totalLabel: strconv.Itoa(l.Pagination.TotalCount),
	}
	if l.EmptyText != "" {
		v.emptyText = l.EmptyText
	} else {
		v.emptyText = "No results found."
	}

	if l.ItemCount <= 0 {
		v.empty = true
	} else {
		cur := l.Pagination.CurrentPage()
		begin := (cur-1)*l.Pagination.perPage() + 1
		v.rangeLabel = strconv.Itoa(begin) + "-" + strconv.Itoa(begin+l.ItemCount-1)

		if l.ItemView != nil {
			v.items = make([]templ.Component, 0, l.ItemCount)
			for i := 0; i < l.ItemCount; i++ {
				var item any
				if l.Item != nil {
					item = l.Item(i)
				}
				v.items = append(v.items, l.ItemView(item))
			}
		}
	}

	pager := l.Pager
	pager.Pagination = l.Pagination
	v.pager = pager.Render()
	return v
}

// Render returns the list component.
func (l ListView) Render() templ.Component { return listView(l.prepared()) }
