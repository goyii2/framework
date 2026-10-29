package dataprovider

import (
	"fmt"
	"sort"
	"time"
)

// ArrayDataProvider feeds a list view from an in-memory slice: it sorts a
// copy of the items and windows out the current page, leaving the caller's
// slice untouched. Sorting is the request-driven whitelist-validated order
// (Params.Sort), falling back to DefaultOrder; pagination follows the same
// clamping rules as the active provider.
//
// In-memory sorting needs a way to compare items per column, which SQL
// gives the active provider for free. Key supplies it: it receives an item
// and a column name and returns the value to order by — a string, a number
// (any int/uint/float width), a bool or a time.Time. Anything else, or a
// column mixing types, fails Prepare with an error rather than ordering by
// accident. A provider that never sorts (request and default order both
// empty) does not need Key.
type ArrayDataProvider[T any] struct {
	// Items is the full in-memory row set the provider pages.
	Items []T
	// Params carries the sanitized list-request parameters.
	Params Params
	// DefaultOrder applies when the request carries no sort.
	DefaultOrder []Sort
	// Key returns the comparison value of column for item. Required
	// whenever an order ends up applied.
	Key func(item T, column string) any
}

// Prepare sorts and windows the items, returning the current page plus the
// paging metadata. The caller's slice is never reordered — sorting works on
// a copy.
func (p ArrayDataProvider[T]) Prepare() (*Result[T], error) {
	perPage := p.Params.perPage()
	totalCount := len(p.Items)
	if totalCount == 0 {
		return &Result[T]{Items: []T{}, Page: 1, PerPage: perPage}, nil
	}

	items := p.Items
	order := effectiveOrder(p.Params.Sort, p.DefaultOrder)
	if len(order) > 0 {
		sorted, err := sortItems(items, order, p.Key)
		if err != nil {
			return nil, err
		}
		items = sorted
	}

	pageCount := pageCountOf(totalCount, perPage)
	page := clampPage(p.Params.Page, pageCount)
	start := (page - 1) * perPage
	end := start + perPage
	if end > totalCount {
		end = totalCount
	}
	return &Result[T]{
		Items:      items[start:end],
		TotalCount: totalCount,
		PageCount:  pageCount,
		Page:       page,
		PerPage:    perPage,
	}, nil
}

// sortItems returns items reordered by order. Keys are extracted once per
// item and column and pair-validated against the column's first key, so by
// the time the comparison-based sort runs every pair compares cleanly and
// nothing fails midway through an unstable state.
func sortItems[T any](items []T, order []Sort, key func(T, string) any) ([]T, error) {
	if key == nil {
		return nil, fmt.Errorf("dataprovider: sorting an ArrayDataProvider requires its Key function")
	}
	columns := make([][]any, len(order))
	for c, field := range order {
		values := make([]any, len(items))
		for i, item := range items {
			values[i] = key(item, field.Column)
			if _, err := compareValues(values[0], values[i]); err != nil {
				return nil, fmt.Errorf("dataprovider: sort column %q: %w", field.Column, err)
			}
		}
		columns[c] = values
	}

	index := make([]int, len(items))
	for i := range index {
		index[i] = i
	}
	sort.SliceStable(index, func(a, b int) bool {
		for c, field := range order {
			// compareValues cannot fail here: every pair in a column was
			// pair-validated during extraction.
			cmp, _ := compareValues(columns[c][index[a]], columns[c][index[b]])
			if cmp == 0 {
				continue
			}
			if field.Desc {
				return cmp > 0
			}
			return cmp < 0
		}
		return false
	})
	sorted := make([]T, len(items))
	for i, at := range index {
		sorted[i] = items[at]
	}
	return sorted, nil
}

// compareValues orders two key values of one sort column. Supported key
// types: strings, all int and uint widths, floats, bools (false before
// true) and time.Time; values of different types — or any other type — are
// an error, keeping the ordering explicit instead of silently undefined.
func compareValues(a, b any) (int, error) {
	switch av := a.(type) {
	case string:
		if bv, ok := b.(string); ok {
			return compareOrdered(av, bv), nil
		}
	case bool:
		bv, ok := b.(bool)
		if ok {
			if av == bv {
				return 0, nil
			}
			if bv {
				return -1, nil
			}
			return 1, nil
		}
	case int:
		if bv, ok := b.(int); ok {
			return compareOrdered(av, bv), nil
		}
	case int8:
		if bv, ok := b.(int8); ok {
			return compareOrdered(av, bv), nil
		}
	case int16:
		if bv, ok := b.(int16); ok {
			return compareOrdered(av, bv), nil
		}
	case int32:
		if bv, ok := b.(int32); ok {
			return compareOrdered(av, bv), nil
		}
	case int64:
		if bv, ok := b.(int64); ok {
			return compareOrdered(av, bv), nil
		}
	case uint:
		if bv, ok := b.(uint); ok {
			return compareOrdered(av, bv), nil
		}
	case uint8:
		if bv, ok := b.(uint8); ok {
			return compareOrdered(av, bv), nil
		}
	case uint16:
		if bv, ok := b.(uint16); ok {
			return compareOrdered(av, bv), nil
		}
	case uint32:
		if bv, ok := b.(uint32); ok {
			return compareOrdered(av, bv), nil
		}
	case uint64:
		if bv, ok := b.(uint64); ok {
			return compareOrdered(av, bv), nil
		}
	case float32:
		if bv, ok := b.(float32); ok {
			return compareOrdered(av, bv), nil
		}
	case float64:
		if bv, ok := b.(float64); ok {
			return compareOrdered(av, bv), nil
		}
	case time.Time:
		if bv, ok := b.(time.Time); ok {
			if av.Before(bv) {
				return -1, nil
			}
			if av.After(bv) {
				return 1, nil
			}
			return 0, nil
		}
	}
	return 0, fmt.Errorf("sort keys must be strings, numbers, bools or time.Time, got %T and %T", a, b)
}

// compareOrdered orders two values of any ordered type by their built-in
// comparison — no subtraction, so extreme values cannot overflow.
func compareOrdered[T interface {
	~string |
		~int | ~int8 | ~int16 | ~int32 | ~int64 |
		~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 |
		~float32 | ~float64
}](a, b T) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}
