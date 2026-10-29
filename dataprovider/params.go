package dataprovider

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/goyii2/framework/view"
)

// DefaultPerPage is the page size applied when the request asks for none or
// asks for something unusable. It aliases the view layer's fallback so
// provider metadata and LinkPager agree on one default by construction.
const DefaultPerPage = view.DefaultPerPage

// MaxPerPage caps the page size one request may ask for: anything larger is
// a drag-the-whole-table query in disguise and clamps down to this.
const MaxPerPage = 100

// ErrInvalidSortField marks a sort request naming a field outside the
// declared whitelist, or carrying anything but a bare column name. Actions
// typically map it to a 400 reply; it is raised before any statement is
// built, so no request text ever reaches the ORDER BY clause.
var ErrInvalidSortField = errors.New("dataprovider: invalid sort field")

// Sort is one ordering instruction: a whitelisted column name and its
// direction. It is the shared currency of the parameter parser and both
// providers.
type Sort struct {
	// Column is a bare column name that the action declared sortable.
	Column string
	// Desc orders the column descending.
	Desc bool
}

// Params are the sanitized list-request parameters both providers consume.
// ParseParams and FromRequest produce them; hand-construction works too and
// is normalized defensively at use (page floors at 1, per-page falls back to
// DefaultPerPage and clamps to MaxPerPage).
type Params struct {
	// Page is the requested page, 1-based. Providers clamp it into
	// [1, pageCount] — an out-of-range request answers the last page.
	Page int
	// PerPage is the requested page size.
	PerPage int
	// Sort holds the parsed sort instructions, already whitelist-validated;
	// nil means the request sorted nothing and the provider's default order
	// applies.
	Sort []Sort
}

// perPage returns the effective page size: requested values collapse into
// (0, MaxPerPage] with DefaultPerPage standing in for anything unusable.
func (p Params) perPage() int {
	if p.PerPage <= 0 {
		return DefaultPerPage
	}
	if p.PerPage > MaxPerPage {
		return MaxPerPage
	}
	return p.PerPage
}

// ParseParams turns the raw page, per-page and sort query values of a list
// request into sanitized Params against the sortable whitelist.
//
// Recovery rules — a malformed request never errors, it falls back:
//
//   - page: unparsable, zero or negative becomes 1 (out-of-range high pages
//     clamp later, when the total count is known).
//   - per-page: unparsable or < 1 becomes DefaultPerPage, > MaxPerPage
//     clamps to MaxPerPage.
//   - sort: empty (or only commas/whitespace) sorts nothing; any token that
//     is not exactly one of the declared sortable columns — or is not a bare
//     identifier even if the whitelist somehow lists it — is rejected with
//     an error wrapping ErrInvalidSortField.
//
// sortable lists the columns the action allows sorting by; with none
// declared, any sort request is rejected.
func ParseParams(page, perPage, sort string, sortable ...string) (Params, error) {
	p := Params{
		Page:    parsePage(page),
		PerPage: parsePerPage(perPage),
	}
	if strings.TrimSpace(sort) == "" {
		return p, nil
	}
	fields, err := parseSort(sort, sortable)
	if err != nil {
		return Params{}, err
	}
	p.Sort = fields
	return p, nil
}

// FromRequest parses the list-request parameters from an HTTP request's URL
// query — the convenience entry for actions, wired straight from the request
// web.Context exposes:
//
//	params, err := dataprovider.FromRequest(c.Request(), "id", "created_at")
//
// It reads the page, per-page and sort query values and hands them to
// ParseParams with the given sortable whitelist.
func FromRequest(r *http.Request, sortable ...string) (Params, error) {
	q := r.URL.Query()
	return ParseParams(q.Get("page"), q.Get("per-page"), q.Get("sort"), sortable...)
}

// parsePage resolves the raw page value to a 1-based page, falling back to 1
// for anything unparsable or below range.
func parsePage(raw string) int {
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || n < 1 {
		return 1
	}
	return n
}

// parsePerPage resolves the raw per-page value to an effective page size,
// falling back to DefaultPerPage for anything unparsable or below range and
// clamping to MaxPerPage.
func parsePerPage(raw string) int {
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || n < 1 {
		return DefaultPerPage
	}
	if n > MaxPerPage {
		return MaxPerPage
	}
	return n
}

// parseSort splits a Yii2-style sort value ("colA,-colB" — leading "-"
// descending, columns comma-separated) into validated instructions. Each
// token must equal a whitelisted column and be a bare identifier; the exact
// match plus the identifier check together guarantee that user text never
// flows into the SQL the providers build.
func parseSort(raw string, sortable []string) ([]Sort, error) {
	if len(sortable) == 0 {
		return nil, fmt.Errorf("%w: no sortable fields declared, refusing to sort by %q", ErrInvalidSortField, raw)
	}
	var fields []Sort
	for _, token := range strings.Split(raw, ",") {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		field := Sort{}
		if strings.HasPrefix(token, "-") {
			field.Desc = true
			token = strings.TrimPrefix(token, "-")
		}
		if !isIdentifier(token) || !containsExact(sortable, token) {
			return nil, fmt.Errorf("%w: %q is not sortable; allowed fields: %s",
				ErrInvalidSortField, token, strings.Join(sortable, ", "))
		}
		field.Column = token
		fields = append(fields, field)
	}
	return fields, nil
}

// containsExact reports whether list holds s verbatim.
func containsExact(list []string, s string) bool {
	for _, item := range list {
		if item == s {
			return true
		}
	}
	return false
}

// isIdentifier reports whether s is a bare column identifier: letters,
// digits and underscores, starting with a letter or underscore. It is the
// backstop behind the whitelist — even a misconfigured whitelist entry
// carrying SQL fragments can never ride into an ORDER BY.
func isIdentifier(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '_', 'a' <= c && c <= 'z', 'A' <= c && c <= 'Z':
		case i > 0 && '0' <= c && c <= '9':
		default:
			return false
		}
	}
	return true
}

// orderExpressions renders validated sort instructions as SQL ORDER BY
// expressions ("col", "col DESC"). Callers pass developer-declared columns;
// the identifier check keeps the output a bare column name plus an optional
// DESC keyword and nothing else.
func orderExpressions(order []Sort) ([]string, error) {
	exprs := make([]string, 0, len(order))
	for _, field := range order {
		if !isIdentifier(field.Column) {
			return nil, fmt.Errorf("dataprovider: sort column %q is not a bare column name; sortable fields must be plain identifiers", field.Column)
		}
		if field.Desc {
			exprs = append(exprs, field.Column+" DESC")
		} else {
			exprs = append(exprs, field.Column)
		}
	}
	return exprs, nil
}

// effectiveOrder picks the ordering to apply: the request's sort when it
// carries one, the provider's default order otherwise.
func effectiveOrder(requested, fallback []Sort) []Sort {
	if len(requested) > 0 {
		return requested
	}
	return fallback
}

// clampPage pulls a requested 1-based page into [1, pageCount]; pageCount
// below 1 means there are no pages at all and the answer is page 1 with an
// empty item set.
func clampPage(page, pageCount int) int {
	if page < 1 || pageCount < 1 {
		return 1
	}
	if page > pageCount {
		return pageCount
	}
	return page
}

// pageCountOf derives the page count from a total; zero items means zero
// pages.
func pageCountOf(total, perPage int) int {
	if total <= 0 {
		return 0
	}
	return (total-1)/perPage + 1
}
