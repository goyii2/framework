package dataprovider

import (
	"fmt"
	"math"

	"github.com/goyii2/framework/db/ar"
)

// ActiveDataProvider feeds a list view from an Active Record query: it runs
// one COUNT for the total and one LIMIT/OFFSET SELECT for the current page,
// pushing pagination and sorting down to SQL instead of dragging the table
// into memory. Configure it as a struct and call Prepare.
//
// The base query fixes the row set — its Where and Join shape both the
// count and the page fetch — while the provider adds only ORDER BY, LIMIT
// and OFFSET on immutable branches, leaving the caller's query untouched.
//
// Ordering comes from Params.Sort (request-driven, whitelist-validated) and
// falls back to DefaultOrder when the request sorts nothing; a provider
// with neither is ordered by whatever the database returns.
type ActiveDataProvider[T any] struct {
	// Query is the base query whose matching rows the provider pages.
	Query *ar.Query[T]
	// Params carries the sanitized list-request parameters.
	Params Params
	// DefaultOrder applies when the request carries no sort; e.g. newest
	// first as the listing default.
	DefaultOrder []Sort
}

// Prepare executes the provider: the total count and the current page's
// records, both from the base query. An out-of-range page clamps to the
// last page (page 1 with an empty item set when nothing matches), and the
// returned Result reports that effective page.
func (p ActiveDataProvider[T]) Prepare() (*Result[*T], error) {
	perPage := p.Params.perPage()

	total, err := p.Query.Count()
	if err != nil {
		return nil, fmt.Errorf("dataprovider: count total: %w", err)
	}
	// A total beyond int's range (only possible on 32-bit builds) cannot be
	// paged meaningfully; refuse rather than truncate.
	if total > int64(math.MaxInt) {
		return nil, fmt.Errorf("dataprovider: total %d exceeds the addressable item count", total)
	}
	totalCount := int(total)
	if totalCount == 0 {
		return &Result[*T]{Items: []*T{}, Page: 1, PerPage: perPage}, nil
	}

	pageCount := pageCountOf(totalCount, perPage)
	page := clampPage(p.Params.Page, pageCount)

	query := p.Query
	exprs, err := orderExpressions(effectiveOrder(p.Params.Sort, p.DefaultOrder))
	if err != nil {
		return nil, err
	}
	if len(exprs) > 0 {
		query = query.OrderBy(exprs...)
	}
	items, err := query.Limit(perPage).Offset((page - 1) * perPage).All()
	if err != nil {
		return nil, fmt.Errorf("dataprovider: fetch page %d: %w", page, err)
	}
	if items == nil {
		items = []*T{}
	}
	return &Result[*T]{
		Items:      items,
		TotalCount: totalCount,
		PageCount:  pageCount,
		Page:       page,
		PerPage:    perPage,
	}, nil
}
