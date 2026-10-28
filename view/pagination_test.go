package view_test

import (
	"testing"

	"github.com/goyii2/framework/view"
)

func TestPageCount(t *testing.T) {
	cases := []struct {
		total, perPage, want int
	}{
		{0, 20, 0},
		{-5, 20, 0},
		{1, 20, 1},
		{20, 20, 1},
		{21, 20, 2},
		{45, 10, 5},
		{40, 0, 2}, // PerPage <= 0 falls back to DefaultPerPage (20)
	}
	for _, tc := range cases {
		p := view.Pagination{TotalCount: tc.total, PerPage: tc.perPage}
		if got := p.PageCount(); got != tc.want {
			t.Errorf("PageCount(total=%d, perPage=%d) = %d, want %d", tc.total, tc.perPage, got, tc.want)
		}
	}
}

func TestCurrentPageClamping(t *testing.T) {
	cases := []struct {
		page, total, perPage, want int
	}{
		{0, 100, 10, 1},   // below range
		{-3, 100, 10, 1},  // negative
		{1, 100, 10, 1},   // first
		{5, 100, 10, 5},   // inside
		{10, 100, 10, 10}, // last
		{11, 100, 10, 10}, // above range clamps to last page
		{7, 0, 10, 1},     // no items at all
	}
	for _, tc := range cases {
		p := view.Pagination{TotalCount: tc.total, PerPage: tc.perPage, Page: tc.page}
		if got := p.CurrentPage(); got != tc.want {
			t.Errorf("CurrentPage(page=%d, total=%d) = %d, want %d", tc.page, tc.total, got, tc.want)
		}
	}
}

func TestOffset(t *testing.T) {
	cases := []struct {
		page, perPage, want int
	}{
		{1, 10, 0},
		{3, 10, 20},
		{2, 0, 20}, // default per-page 20
	}
	for _, tc := range cases {
		p := view.Pagination{TotalCount: 1000, PerPage: tc.perPage, Page: tc.page}
		if got := p.Offset(); got != tc.want {
			t.Errorf("Offset(page=%d, perPage=%d) = %d, want %d", tc.page, tc.perPage, got, tc.want)
		}
	}
}

func TestHasPrevHasNext(t *testing.T) {
	first := view.Pagination{TotalCount: 50, PerPage: 10, Page: 1}
	if first.HasPrev() {
		t.Error("first page reports HasPrev")
	}
	if !first.HasNext() {
		t.Error("first page of five reports no HasNext")
	}
	last := view.Pagination{TotalCount: 50, PerPage: 10, Page: 5}
	if !last.HasPrev() {
		t.Error("last page reports no HasPrev")
	}
	if last.HasNext() {
		t.Error("last page reports HasNext")
	}
	empty := view.Pagination{}
	if empty.HasPrev() || empty.HasNext() {
		t.Error("empty pagination reports step availability")
	}
}
