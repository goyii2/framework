package dataprovider

import (
	"strings"
	"testing"
	"time"
)

// tag is the in-memory test item: sortable by name and priority.
type tag struct {
	Name     string
	Priority int
}

func tagKey(t *tag, column string) any {
	switch column {
	case "name":
		return t.Name
	case "priority":
		return t.Priority
	}
	return nil
}

func sampleTags() []*tag {
	return []*tag{
		{Name: "beta", Priority: 2},
		{Name: "alpha", Priority: 2},
		{Name: "gamma", Priority: 1},
		{Name: "delta", Priority: 2},
		{Name: "epsilon", Priority: 3},
	}
}

func TestArrayProviderSortsAndPages(t *testing.T) {
	res, err := ArrayDataProvider[*tag]{
		Items:  sampleTags(),
		Params: Params{Page: 2, PerPage: 2, Sort: []Sort{{Column: "priority", Desc: true}, {Column: "name"}}},
		Key:    tagKey,
	}.Prepare()
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if res.TotalCount != 5 || res.PageCount != 3 || res.Page != 2 || res.PerPage != 2 {
		t.Errorf("meta = %+v, want 5 items over 3 pages on page 2", res)
	}
	// priority DESC: three 2s (alpha, beta, delta by name), then 1s, then 3?
	// No: priorities are 2,2,1,2,3 -> desc order 3, then 2s sorted by name
	// (alpha, beta, delta), then 1. Page 2 of 2 per page = beta, delta.
	names := make([]string, 0, len(res.Items))
	for _, t := range res.Items {
		names = append(names, t.Name)
	}
	if strings.Join(names, ",") != "beta,delta" {
		t.Errorf("items = %v, want [beta delta]", names)
	}
}

func TestArrayProviderSortsOnACopy(t *testing.T) {
	items := sampleTags()
	_, err := ArrayDataProvider[*tag]{
		Items:  items,
		Params: Params{Page: 1, PerPage: 10, Sort: []Sort{{Column: "name"}}},
		Key:    tagKey,
	}.Prepare()
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if items[0].Name != "beta" || items[1].Name != "alpha" {
		t.Errorf("caller slice was reordered: %s, %s", items[0].Name, items[1].Name)
	}
}

func TestArrayProviderStableSortKeepsEqualOrder(t *testing.T) {
	// All priorities equal: stable sort keeps insertion order.
	items := []*tag{
		{Name: "c", Priority: 1},
		{Name: "a", Priority: 1},
		{Name: "b", Priority: 1},
	}
	res, err := ArrayDataProvider[*tag]{
		Items:  items,
		Params: Params{Page: 1, PerPage: 10, Sort: []Sort{{Column: "priority"}}},
		Key:    tagKey,
	}.Prepare()
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if res.Items[0].Name != "c" || res.Items[1].Name != "a" || res.Items[2].Name != "b" {
		t.Errorf("equal keys reordered: %v", res.Items)
	}
}

func TestArrayProviderClampsPageBeyondLast(t *testing.T) {
	res, err := ArrayDataProvider[*tag]{
		Items:  sampleTags(),
		Params: Params{Page: 42, PerPage: 2},
	}.Prepare()
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if res.Page != 3 {
		t.Errorf("page = %d, want clamp to last page 3", res.Page)
	}
	if len(res.Items) != 1 {
		t.Errorf("items = %d, want the single row of the last page", len(res.Items))
	}
}

func TestArrayProviderEmptyItems(t *testing.T) {
	res, err := ArrayDataProvider[*tag]{
		Items:  nil,
		Params: Params{Page: 3, PerPage: 2},
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
}

func TestArrayProviderWithoutKeyFunctionRefusesSort(t *testing.T) {
	_, err := ArrayDataProvider[*tag]{
		Items:  sampleTags(),
		Params: Params{Page: 1, Sort: []Sort{{Column: "name"}}},
	}.Prepare()
	if err == nil || !strings.Contains(err.Error(), "requires its Key function") {
		t.Errorf("err = %v, want the missing-Key rejection", err)
	}
}

func TestArrayProviderRejectsUnsupportedKeyType(t *testing.T) {
	_, err := ArrayDataProvider[time.Time]{
		Items:  []time.Time{time.Now(), time.Now()},
		Params: Params{Page: 1, Sort: []Sort{{Column: "at"}}},
		Key: func(item time.Time, _ string) any {
			return item // time.Time is supported; wrap it to make it unsupported
		},
	}.Prepare()
	if err != nil {
		t.Fatalf("Prepare with time.Time keys: %v", err)
	}

	_, err = ArrayDataProvider[time.Time]{
		Items:  []time.Time{time.Now(), time.Now()},
		Params: Params{Page: 1, Sort: []Sort{{Column: "at"}}},
		Key: func(item time.Time, _ string) any {
			return struct{ t time.Time }{item} // no ordering defined for structs
		},
	}.Prepare()
	if err == nil || !strings.Contains(err.Error(), "sort keys must be") {
		t.Errorf("err = %v, want the unsupported-key-type rejection", err)
	}
}

func TestArrayProviderRejectsMixedKeyTypes(t *testing.T) {
	_, err := ArrayDataProvider[*tag]{
		Items:  sampleTags(),
		Params: Params{Page: 1, Sort: []Sort{{Column: "priority"}}},
		Key: func(t *tag, column string) any {
			if t.Name == "gamma" {
				return t.Name // string in a column otherwise int
			}
			return t.Priority
		},
	}.Prepare()
	if err == nil || !strings.Contains(err.Error(), `sort column "priority"`) {
		t.Errorf("err = %v, want the mixed-type rejection naming the column", err)
	}
}

func TestArrayProviderDefaultOrder(t *testing.T) {
	res, err := ArrayDataProvider[*tag]{
		Items:        sampleTags(),
		Params:       Params{Page: 1, PerPage: 10},
		DefaultOrder: []Sort{{Column: "name"}},
		Key:          tagKey,
	}.Prepare()
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if res.Items[0].Name != "alpha" {
		t.Errorf("first item = %s, want alpha via default order", res.Items[0].Name)
	}
}
