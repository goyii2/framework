package dataprovider

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseParamsDefaultsOnEmptyRequest(t *testing.T) {
	p, err := ParseParams("", "", "", "id", "name")
	if err != nil {
		t.Fatalf("ParseParams: %v", err)
	}
	if p.Page != 1 || p.PerPage != DefaultPerPage || p.Sort != nil {
		t.Errorf("empty request = %+v, want page 1, per-page %d, no sort", p, DefaultPerPage)
	}
}

func TestParseParamsPageFallsBackToOne(t *testing.T) {
	for _, raw := range []string{"abc", "0", "-3", "  ", "12x"} {
		p, err := ParseParams(raw, "", "", "id")
		if err != nil {
			t.Fatalf("ParseParams(%q): %v", raw, err)
		}
		if p.Page != 1 {
			t.Errorf("page %q = %d, want fallback 1", raw, p.Page)
		}
	}
	if p, _ := ParseParams("7", "", "", "id"); p.Page != 7 {
		t.Errorf("page 7 = %d, want 7", p.Page)
	}
}

func TestParseParamsPerPageDefaultAndCap(t *testing.T) {
	cases := []struct {
		raw  string
		want int
	}{
		{"abc", DefaultPerPage},
		{"0", DefaultPerPage},
		{"-5", DefaultPerPage},
		{"50", 50},
		{"100", MaxPerPage},
		{"1000", MaxPerPage},
		{"999999999999", MaxPerPage},
	}
	for _, tc := range cases {
		p, err := ParseParams("", tc.raw, "", "id")
		if err != nil {
			t.Fatalf("ParseParams(per-page %q): %v", tc.raw, err)
		}
		if p.PerPage != tc.want {
			t.Errorf("per-page %q = %d, want %d", tc.raw, p.PerPage, tc.want)
		}
	}
}

func TestParseSortWhitelist(t *testing.T) {
	p, err := ParseParams("", "", " -created_at , id ", "created_at", "id")
	if err != nil {
		t.Fatalf("ParseParams: %v", err)
	}
	want := []Sort{{Column: "created_at", Desc: true}, {Column: "id"}}
	if len(p.Sort) != len(want) {
		t.Fatalf("sort = %+v, want %+v", p.Sort, want)
	}
	for i, field := range want {
		if p.Sort[i] != field {
			t.Errorf("sort[%d] = %+v, want %+v", i, p.Sort[i], field)
		}
	}
}

func TestParseSortEmptyVariantsSortNothing(t *testing.T) {
	for _, raw := range []string{"", "   ", ",, ", "-"} {
		// "-" alone names no column and is rejected, not silently ignored.
		if raw == "-" {
			if _, err := ParseParams("", "", raw, "id"); !errors.Is(err, ErrInvalidSortField) {
				t.Errorf("sort %q: err = %v, want ErrInvalidSortField", raw, err)
			}
			continue
		}
		p, err := ParseParams("", "", raw, "id")
		if err != nil || p.Sort != nil {
			t.Errorf("sort %q = %+v (err %v), want no sort", raw, p.Sort, err)
		}
	}
}

func TestParseSortRejectsFieldOutsideWhitelist(t *testing.T) {
	for _, raw := range []string{"password", "id,password", "name"} {
		_, err := ParseParams("", "", raw, "id")
		if !errors.Is(err, ErrInvalidSortField) {
			t.Errorf("sort %q: err = %v, want ErrInvalidSortField", raw, err)
		}
	}
}

func TestParseSortRejectsSQLFragmentsEvenWhenWhitelisted(t *testing.T) {
	// A misconfigured whitelist must not open the door: the exact-match
	// token may pass the whitelist, but the identifier backstop still
	// refuses anything that is not a bare column name.
	poisoned := "id; DROP TABLE users"
	_, err := ParseParams("", "", "-"+poisoned, poisoned)
	if !errors.Is(err, ErrInvalidSortField) {
		t.Errorf("err = %v, want ErrInvalidSortField", err)
	}
}

func TestParseSortWithoutDeclaredFieldsIsRejected(t *testing.T) {
	if _, err := ParseParams("", "", "id"); !errors.Is(err, ErrInvalidSortField) {
		t.Errorf("err = %v, want ErrInvalidSortField", err)
	}
}

func TestFromRequestReadsQueryValues(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/items?page=3&per-page=5&sort=-id%2Cname", nil)
	p, err := FromRequest(req, "id", "name")
	if err != nil {
		t.Fatalf("FromRequest: %v", err)
	}
	if p.Page != 3 || p.PerPage != 5 {
		t.Errorf("params = %+v, want page 3 per-page 5", p)
	}
	want := []Sort{{Column: "id", Desc: true}, {Column: "name"}}
	if len(p.Sort) != 2 || p.Sort[0] != want[0] || p.Sort[1] != want[1] {
		t.Errorf("sort = %+v, want %+v", p.Sort, want)
	}

	bad := httptest.NewRequest(http.MethodGet, "/items?sort=evil", nil)
	if _, err := FromRequest(bad, "id"); !errors.Is(err, ErrInvalidSortField) {
		t.Errorf("FromRequest(sort=evil): err = %v, want ErrInvalidSortField", err)
	}
}

func TestParamsPerPageNormalizesHandConstruction(t *testing.T) {
	cases := []struct {
		perPage int
		want    int
	}{
		{0, DefaultPerPage},
		{-1, DefaultPerPage},
		{7, 7},
		{MaxPerPage + 1, MaxPerPage},
	}
	for _, tc := range cases {
		if got := (Params{PerPage: tc.perPage}).perPage(); got != tc.want {
			t.Errorf("perPage() of %d = %d, want %d", tc.perPage, got, tc.want)
		}
	}
}
