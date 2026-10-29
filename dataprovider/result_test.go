package dataprovider

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/goyii2/framework/view"
)

// renderPager renders the LinkPager consuming pg to a string, for asserting
// on the HTML the widget emits from provider metadata.
func renderPager(t *testing.T, pg view.Pagination) string {
	t.Helper()
	var sb strings.Builder
	pager := view.LinkPager{Pagination: pg}
	if err := pager.Render().Render(context.Background(), &sb); err != nil {
		t.Fatalf("render pager: %v", err)
	}
	return sb.String()
}

func TestResultPaginationFeedsLinkPager(t *testing.T) {
	res := &Result[*tag]{
		Items:      []*tag{},
		TotalCount: 45,
		PageCount:  5,
		Page:       2,
		PerPage:    10,
	}

	pg := res.Pagination()
	if pg != (view.Pagination{TotalCount: 45, PerPage: 10, Page: 2}) {
		t.Errorf("Pagination() = %+v", pg)
	}
	// The derived contract the pager reads must agree with the provider's
	// own metadata.
	if pg.PageCount() != res.PageCount || pg.CurrentPage() != res.Page || pg.Offset() != 10 || !pg.HasPrev() || !pg.HasNext() {
		t.Errorf("pager-visible pagination disagrees with result: %+v", pg)
	}

	html := renderPager(t, pg)
	for _, want := range []string{`?page=1`, `?page=3`, "active"} {
		if !strings.Contains(html, want) {
			t.Errorf("pager HTML misses %q: %s", want, html)
		}
	}
}

func TestResultPaginationClampedPageRendersTruePosition(t *testing.T) {
	// A ?page=99 request the provider clamped to the last page must render
	// as the last page, not as a dead link beyond the end.
	res := &Result[*tag]{TotalCount: 25, PageCount: 3, Page: 3, PerPage: 10}
	html := renderPager(t, res.Pagination())
	if strings.Contains(html, "?page=4") {
		t.Errorf("pager links beyond the last page: %s", html)
	}
	if !strings.Contains(html, "next disabled") {
		t.Errorf("pager misses the disabled next button: %s", html)
	}
}

func TestResultJSONCarriesItemsAndMeta(t *testing.T) {
	res, err := ArrayDataProvider[*tag]{
		Items:  sampleTags(),
		Params: Params{Page: 2, PerPage: 2, Sort: []Sort{{Column: "name"}}},
		Key:    tagKey,
	}.Prepare()
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	raw, err := json.Marshal(res)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, key := range []string{"items", "totalCount", "pageCount", "page", "perPage"} {
		if _, ok := payload[key]; !ok {
			t.Errorf("JSON payload misses %q: %s", key, raw)
		}
	}
	if payload["totalCount"].(float64) != 5 || payload["page"].(float64) != 2 {
		t.Errorf("JSON meta = %s", raw)
	}
}

func TestResultEmptyItemsEncodeAsArray(t *testing.T) {
	res := &Result[*tag]{Items: []*tag{}, Page: 1, PerPage: 10}
	raw, err := json.Marshal(res)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(raw), `"items":[]`) {
		t.Errorf("empty items must encode as [], got %s", raw)
	}
}
