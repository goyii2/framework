package dataprovider

import "github.com/goyii2/framework/view"

// Result is the response shape both providers answer with: the current
// page's items plus the paging metadata. The field tags make it directly
// JSON-encodable for API replies (c.JSON(res) sends items + meta in one
// payload), and Pagination maps the metadata onto the view layer's contract
// for the LinkPager widget — the two consumers the ticket chain shares it
// between.
type Result[T any] struct {
	// Items holds the current page's records; never nil, so JSON encodes an
	// empty array rather than null.
	Items []T `json:"items"`
	// TotalCount is the number of matching items across all pages.
	TotalCount int `json:"totalCount"`
	// PageCount is the number of pages; zero when nothing matches.
	PageCount int `json:"pageCount"`
	// Page is the effective current page, 1-based: the requested page
	// already clamped into range.
	Page int `json:"page"`
	// PerPage is the effective page size.
	PerPage int `json:"perPage"`
}

// Pagination maps the result onto the view layer's paging contract. The
// page is the already-clamped effective one, so LinkPager renders the true
// position: a ?page=99 request beyond the last page renders as the last
// page, not as a dead link.
func (r Result[T]) Pagination() view.Pagination {
	return view.Pagination{
		TotalCount: r.TotalCount,
		PerPage:    r.PerPage,
		Page:       r.Page,
	}
}
