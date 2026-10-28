package view

// Pagination is the paging metadata contract between the data provider
// layer and list views: total item count, page size and the current
// (1-based) page. Providers report totalCount / pageCount / current page in
// their responses and map that meta onto this struct; the LinkPager widget
// and any custom list view consume the same shape.
//
// The zero value is a valid empty pagination: no items, one empty page.
type Pagination struct {
	// TotalCount is the number of items across all pages.
	TotalCount int
	// PerPage is the page size; values <= 0 fall back to DefaultPerPage.
	PerPage int
	// Page is the current page as the client addresses it, 1-based. Values
	// outside [1, PageCount] clamp on read (CurrentPage), mirroring how a
	// provider answers an out-of-range ?page= query with the last page.
	Page int
}

// DefaultPerPage is the page size used when Pagination.PerPage <= 0.
const DefaultPerPage = 20

func (p Pagination) perPage() int {
	if p.PerPage <= 0 {
		return DefaultPerPage
	}
	return p.PerPage
}

// PageCount returns the number of pages; zero when there are no items.
func (p Pagination) PageCount() int {
	if p.TotalCount <= 0 {
		return 0
	}
	return (p.TotalCount-1)/p.perPage() + 1
}

// CurrentPage returns the effective page: Page clamped into [1, PageCount]
// (to 1 when there are no pages at all).
func (p Pagination) CurrentPage() int {
	if p.Page < 1 {
		return 1
	}
	if pc := p.PageCount(); p.Page > pc {
		if pc < 1 {
			return 1
		}
		return pc
	}
	return p.Page
}

// Offset returns the zero-based item offset of the current page — the
// provider's SQL OFFSET.
func (p Pagination) Offset() int {
	return (p.CurrentPage() - 1) * p.perPage()
}

// HasPrev reports whether a page before the current one exists.
func (p Pagination) HasPrev() bool { return p.CurrentPage() > 1 }

// HasNext reports whether a page after the current one exists.
func (p Pagination) HasNext() bool { return p.CurrentPage() < p.PageCount() }
