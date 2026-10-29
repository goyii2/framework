package view

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/a-h/templ"
)

// ColumnFormat selects how a DataColumn renders its cell value: FormatText
// (the default) escapes everything, FormatHTML passes the value through as
// raw markup. HTML is an explicit opt-in — the default never trusts item
// data.
type ColumnFormat string

const (
	FormatText ColumnFormat = "text"
	FormatHTML ColumnFormat = "html"
)

// ColumnFilter configures the filter-row input of a DataColumn: a GET form
// control named after the column's Attr. Type reuses the ActiveForm field
// types; "" means FieldText. FieldSelect renders the Options and marks the
// one matching the echoed value; every other type renders a typed input
// with the echoed value. Include an empty Value option as the prompt choice
// if the select should be clearable — the widget adds none on its own.
type ColumnFilter struct {
	Type    FieldType
	Options []FieldOption
}

// DataColumn is one column of a GridView. Attr is the column's identity: it
// names the filter input, the sort parameter value it links to (so it must
// be one of the sortable fields the action whitelisted for the data
// provider), and the fallback header label. Value computes the cell from
// the row item; nil renders an empty cell — the widget deliberately does
// not reflect over items, actions resolve values explicitly.
type DataColumn struct {
	// Attr names the column: filter input name, sort parameter value,
	// fallback header label.
	Attr string
	// Label heads the column; "" falls back to Attr.
	Label string
	// Format renders the value escaped (FormatText, the default) or as raw
	// HTML (FormatHTML — explicit opt-in).
	Format ColumnFormat
	// Value computes the cell value from the row item; nil renders an empty
	// cell. A returned templ.Component renders inline; nil an empty cell;
	// anything else its fmt.Sprint form under Format.
	Value func(item any) any
	// Sortable links the header to toggle the dataprovider sort parameter.
	Sortable bool
	// Filter renders an input for this column in the filter row; nil skips
	// the column's filter cell (and no column filtering at all means no
	// filter row and no form wrapper).
	Filter *ColumnFilter
}

// GridView renders the data table of a prepared dataprovider.Result: header
// row with sort links, an optional GET filter row, one row per item and an
// empty-state placeholder. It is a pure renderer — it never queries; the
// action prepares the result and hands its pieces in:
//
//	params, _ := dataprovider.FromRequest(c.Request(), "id", "created_at")
//	res := provider.Prepare(ctx, params)
//	q := c.Request().URL.Query()
//	grid := view.GridView{
//		Columns:     columns,
//		ItemCount:   len(res.Items),
//		Item:        func(i int) any { return res.Items[i] },
//		Sort:        q.Get("sort"),
//		FilterValues: view.QueryValues(c.Request(), "title", "status"),
//	}
//
// The generic items reach the widget through the Item closure (bridge from
// Result[T] to any) and each column's Value computes the cell from that
// item. Filtering splits cleanly: the widget renders the filter inputs and
// echoes their current values, while assembling the query they describe —
// reading the request and applying Where clauses — is the action's job,
// done before Prepare. There is no search-model indirection.
type GridView struct {
	// Columns render left to right.
	Columns []DataColumn

	// ItemCount is the number of items on the current page:
	// len(res.Items).
	ItemCount int

	// Item returns page row i as any — the bridge from the result's generic
	// slice. nil renders ItemCount empty rows.
	Item func(i int) any

	// Sort is the current sort query value in the dataprovider convention
	// ("created_at" ascending, "-created_at" descending, comma-separated
	// for multi-column). It decides the direction state each sortable
	// header shows and the direction its link flips to. The value the links
	// write back is always a single column, matching what the provider's
	// whitelist accepts.
	Sort string

	// SortURL builds the href of a sort link from the sort value it selects
	// (e.g. "-created_at"); nil means "?sort=<value>". Supply it when the
	// links must keep the current query (filters, page) around.
	SortURL func(sortValue string) string

	// FilterValues echoes the filter inputs, keyed by column Attr — after a
	// GET with filters, pass the request's query values back so the row
	// shows what is active. Nil renders the inputs empty.
	FilterValues map[string]string

	// EmptyText renders in the placeholder row when ItemCount is zero; ""
	// means "No results found.".
	EmptyText string
}

// gridViewView is the precomputed grid the template renders.
type gridViewView struct {
	// form wraps the table in a GET form when any column declared a filter
	// — the filter row's inputs need one.
	form       bool
	hasFilters bool
	headers    []headerCell
	// filters stays aligned with headers; unconfigured cells render empty
	// to keep the row's column alignment.
	filters   []filterCellView
	rows      [][]gridCellView
	empty     bool
	colspan   string
	emptyText string
}

// headerCell is one precomputed header: the label and, when sortable, the
// toggle link plus the direction class of the current state.
type headerCell struct {
	label   string
	class   string
	sortURL string
}

// filterCellView is one precomputed filter input with its echoed value
// resolved; a zero cell (no Attr) renders an empty td.
type filterCellView struct {
	name    string
	typ     string
	value   string
	options []activeOptionView
}

// gridCellView is one precomputed body cell: escaped text or an inline
// component (raw HTML arrives as a RawHTML component).
type gridCellView struct {
	text      string
	component templ.Component
}

// QueryValues collects the first query value of each named parameter of a
// GET request — the echo map a GridView's FilterValues wants, straight from
// the request the action serves:
//
//	FilterValues: view.QueryValues(c.Request(), "title", "status"),
//
// The widget stays a pure renderer (map in, markup out); request reading
// lives in this action-side helper.
func QueryValues(r *http.Request, names ...string) map[string]string {
	if len(names) == 0 {
		return nil
	}
	q := r.URL.Query()
	values := make(map[string]string, len(names))
	for _, name := range names {
		values[name] = q.Get(name)
	}
	return values
}

// sortState is a column's direction under the current sort value.
type sortState int

const (
	sortNone sortState = iota
	sortAsc
	sortDesc
)

// sortStateOf reports how raw — a sort value in the dataprovider convention
// ("colA,-colB") — orders attr: ascending, descending or not at all.
func sortStateOf(raw, attr string) sortState {
	for _, token := range strings.Split(raw, ",") {
		token = strings.TrimSpace(token)
		switch token {
		case attr:
			return sortAsc
		case "-" + attr:
			return sortDesc
		}
	}
	return sortNone
}

func (g GridView) prepared() gridViewView {
	v := gridViewView{
		colspan: strconv.Itoa(len(g.Columns)),
	}
	if g.EmptyText != "" {
		v.emptyText = g.EmptyText
	} else {
		v.emptyText = "No results found."
	}

	v.headers = make([]headerCell, 0, len(g.Columns))
	v.filters = make([]filterCellView, 0, len(g.Columns))
	for _, col := range g.Columns {
		v.headers = append(v.headers, g.header(col))
		if col.Filter != nil {
			v.filters = append(v.filters, g.filterCell(col))
			v.hasFilters = true
			v.form = true
		} else {
			v.filters = append(v.filters, filterCellView{})
		}
	}

	if g.ItemCount <= 0 {
		v.empty = true
		return v
	}
	v.rows = make([][]gridCellView, 0, g.ItemCount)
	for i := 0; i < g.ItemCount; i++ {
		var item any
		if g.Item != nil {
			item = g.Item(i)
		}
		row := make([]gridCellView, 0, len(g.Columns))
		for _, col := range g.Columns {
			row = append(row, col.cell(item))
		}
		v.rows = append(v.rows, row)
	}
	return v
}

// header resolves one column's header cell: plain label, or a sortable
// header whose link flips the column's direction — an ascending column
// links to descending, anything else to ascending.
func (g GridView) header(col DataColumn) headerCell {
	label := col.Label
	if label == "" {
		label = col.Attr
	}
	h := headerCell{label: label}
	if !col.Sortable || col.Attr == "" {
		return h
	}
	value := col.Attr
	switch sortStateOf(g.Sort, col.Attr) {
	case sortAsc:
		h.class = classes("sortable", "asc")
		value = "-" + col.Attr
	case sortDesc:
		h.class = classes("sortable", "desc")
	default:
		h.class = "sortable"
	}
	h.sortURL = "?sort=" + value
	if g.SortURL != nil {
		h.sortURL = g.SortURL(value)
	}
	return h
}

// filterCell resolves one column's filter input with its current value
// echoed from FilterValues.
func (g GridView) filterCell(col DataColumn) filterCellView {
	typ := col.Filter.Type
	if typ == "" {
		typ = FieldText
	}
	f := filterCellView{name: col.Attr, typ: string(typ)}
	if typ == FieldSelect {
		f.options = resolveOptions(col.Filter.Options, g.FilterValues[col.Attr])
	} else {
		f.value = g.FilterValues[col.Attr]
	}
	return f
}

// cell resolves the column's cell for one item: no Value or a nil result
// renders an empty cell; a component renders inline; FormatHTML wraps the
// string form as raw markup; the default escapes the string form.
func (c DataColumn) cell(item any) gridCellView {
	var value any
	if c.Value != nil {
		value = c.Value(item)
	}
	switch v := value.(type) {
	case nil:
		return gridCellView{}
	case templ.Component:
		return gridCellView{component: v}
	}
	s := fmt.Sprint(value)
	if c.Format == FormatHTML {
		return gridCellView{component: templ.Raw(s)}
	}
	return gridCellView{text: s}
}

// Render returns the grid table component.
func (g GridView) Render() templ.Component { return gridView(g.prepared()) }
