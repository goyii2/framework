package dataprovider

import (
	"strings"
	"testing"

	"github.com/goyii2/framework/db/ar"
)

// idsOf collects the member ids of one page in order.
func idsOf(t *testing.T, items []*member) []int64 {
	t.Helper()
	ids := make([]int64, 0, len(items))
	for _, m := range items {
		ids = append(ids, m.ID)
	}
	return ids
}

func TestActiveProviderPushesPaginationAndSortToSQL(t *testing.T) {
	ctx, rec := newActiveTestContext(t)
	seedMembers(t, ctx, 12)
	rec.reset()

	res, err := ActiveDataProvider[member]{
		Query:  ar.New[member](ctx),
		Params: Params{Page: 2, PerPage: 5, Sort: []Sort{{Column: "id", Desc: true}}},
	}.Prepare()
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}

	if res.TotalCount != 12 || res.PageCount != 3 || res.Page != 2 || res.PerPage != 5 {
		t.Errorf("meta = total %d, pages %d, page %d, perPage %d; want 12/3/2/5",
			res.TotalCount, res.PageCount, res.Page, res.PerPage)
	}
	if got := idsOf(t, res.Items); len(got) != 5 || got[0] != 7 || got[4] != 3 {
		t.Errorf("page items = %v, want ids 7..3 (id DESC, page 2 of 3)", got)
	}

	// The push-down contract: exactly two statements — one counting the
	// total, one fetching the page with ORDER BY / LIMIT / OFFSET.
	stmts := rec.recorded()
	if len(stmts) != 2 {
		t.Fatalf("executed %d statements, want 2 (count + page select): %q", len(stmts), stmts)
	}
	if !strings.Contains(stmts[0], "count") {
		t.Errorf("first statement is not the count: %q", stmts[0])
	}
	selectStmt := strings.ToUpper(stmts[1])
	for _, fragment := range []string{"ORDER BY", "LIMIT", "OFFSET"} {
		if !strings.Contains(selectStmt, fragment) {
			t.Errorf("page select misses %s: %q", fragment, stmts[1])
		}
	}
}

func TestActiveProviderClampsPageBeyondLast(t *testing.T) {
	ctx, _ := newActiveTestContext(t)
	seedMembers(t, ctx, 12)

	res, err := ActiveDataProvider[member]{
		Query:  ar.New[member](ctx),
		Params: Params{Page: 99, PerPage: 5},
	}.Prepare()
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if res.Page != 3 {
		t.Errorf("page = %d, want clamp to last page 3", res.Page)
	}
	if got := idsOf(t, res.Items); len(got) != 2 || got[0] != 11 || got[1] != 12 {
		t.Errorf("items = %v, want the two rows of the last page", got)
	}
}

func TestActiveProviderEmptySet(t *testing.T) {
	ctx, rec := newActiveTestContext(t)
	rec.reset()

	res, err := ActiveDataProvider[member]{
		Query:  ar.New[member](ctx),
		Params: Params{Page: 4, PerPage: 5},
	}.Prepare()
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if res.TotalCount != 0 || res.PageCount != 0 || res.Page != 1 {
		t.Errorf("meta = %+v, want empty set on page 1", res)
	}
	if res.Items == nil || len(res.Items) != 0 {
		t.Errorf("items = %#v, want non-nil empty slice", res.Items)
	}
	// Nothing matches: the count alone runs, no page select.
	if stmts := rec.recorded(); len(stmts) != 1 {
		t.Errorf("executed %d statements on empty set, want 1 (count): %q", len(stmts), stmts)
	}
}

func TestActiveProviderRespectsBaseQueryConditions(t *testing.T) {
	ctx, _ := newActiveTestContext(t)
	seedMembers(t, ctx, 10)

	res, err := ActiveDataProvider[member]{
		Query:  ar.New[member](ctx).Where("status = ?", 2),
		Params: Params{Page: 1, PerPage: 3},
	}.Prepare()
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if res.TotalCount != 5 || res.PageCount != 2 {
		t.Errorf("meta = total %d, pages %d; want the filtered 5 rows over 2 pages", res.TotalCount, res.PageCount)
	}
	for _, m := range res.Items {
		if m.Status != 2 {
			t.Errorf("item %d has status %d, want 2", m.ID, m.Status)
		}
	}
}

func TestActiveProviderDefaultOrderWithoutSortParam(t *testing.T) {
	ctx, _ := newActiveTestContext(t)
	seedMembers(t, ctx, 7)

	res, err := ActiveDataProvider[member]{
		Query:        ar.New[member](ctx),
		Params:       Params{Page: 1, PerPage: 3},
		DefaultOrder: []Sort{{Column: "id", Desc: true}},
	}.Prepare()
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if got := idsOf(t, res.Items); got[0] != 7 || got[2] != 5 {
		t.Errorf("items = %v, want id DESC via default order", got)
	}
}

func TestActiveProviderHandBuiltSortNeverReachesSQL(t *testing.T) {
	ctx, rec := newActiveTestContext(t)
	seedMembers(t, ctx, 3)
	rec.reset()

	// Even bypassing the parser with a hand-built Params, the provider
	// refuses anything but bare identifiers and runs no page statement.
	_, err := ActiveDataProvider[member]{
		Query:  ar.New[member](ctx),
		Params: Params{Page: 1, Sort: []Sort{{Column: "id; DROP TABLE members", Desc: true}}},
	}.Prepare()
	if err == nil || !strings.Contains(err.Error(), "bare column name") {
		t.Errorf("err = %v, want the bare-column-name rejection", err)
	}
	if stmts := rec.recorded(); len(stmts) != 1 {
		t.Errorf("executed %d statements, want only the count (no page select): %q", len(stmts), stmts)
	}
}

func TestActiveProviderLeavesBaseQueryUntouched(t *testing.T) {
	ctx, _ := newActiveTestContext(t)
	seedMembers(t, ctx, 8)

	base := ar.New[member](ctx)
	if _, err := (ActiveDataProvider[member]{
		Query:  base,
		Params: Params{Page: 2, PerPage: 3, Sort: []Sort{{Column: "id"}}},
	}).Prepare(); err != nil {
		t.Fatalf("Prepare: %v", err)
	}

	// The provider's branches must not leak ORDER BY/LIMIT/OFFSET into the
	// caller's query.
	all, err := base.All()
	if err != nil {
		t.Fatalf("base All: %v", err)
	}
	if len(all) != 8 {
		t.Errorf("base query fetched %d rows after provider ran, want all 8", len(all))
	}
}
