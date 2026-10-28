package view

import (
	"fmt"
	"strconv"

	"github.com/a-h/templ"
)

// LinkPager renders a Yii2-style pagination bar: previous / next buttons
// around a window of numbered page buttons. Configure it as a struct and
// call Render to obtain the component.
//
// URL decides the link target of each page; nil falls back to "?page=N"
// query strings. Build it from a named route for clean URLs:
//
//	pager := view.LinkPager{
//	    Pagination: pg,
//	    URL:        func(page int) string { return url.To("post/index", url.P{"page": page}) },
//	}
type LinkPager struct {
	Pagination Pagination
	// URL builds the href for a 1-based page number; nil means "?page=N".
	URL func(page int) string
	// MaxButtonCount caps the numbered page buttons; <= 0 means 10. The
	// window slides to keep the current page roughly centered.
	MaxButtonCount int
	// PrevPageLabel / NextPageLabel label the step buttons; "" means the
	// Yii2-style single guillemets.
	PrevPageLabel string
	NextPageLabel string
	// FirstPageLabel / LastPageLabel, when non-empty, add jump-to-end
	// buttons (hidden by default, matching Yii2).
	FirstPageLabel string
	LastPageLabel  string
	// HideOnSinglePage suppresses the bar when everything fits one page.
	HideOnSinglePage bool
}

// pagerButton is one rendered button of the bar. Class is the joined li
// class list ("prev disabled", "active", ""); an empty class leaves the
// attribute out. Disabled buttons render plain text instead of links.
type pagerButton struct {
	Label    string
	URL      string
	Class    string
	Disabled bool
}

// classes joins the non-empty class fragments of one button.
func classes(parts ...string) string {
	joined := ""
	for _, part := range parts {
		if part == "" {
			continue
		}
		if joined != "" {
			joined += " "
		}
		joined += part
	}
	return joined
}

func (p LinkPager) urlFor(page int) string {
	if p.URL != nil {
		return p.URL(page)
	}
	return fmt.Sprintf("?page=%d", page)
}

func (p LinkPager) label(or string, set string) string {
	if set != "" {
		return set
	}
	return or
}

// buttons computes the button list: optional first, prev, the numbered
// window, next and optional last.
func (p LinkPager) buttons() []pagerButton {
	pc := p.Pagination.PageCount()
	cur := p.Pagination.CurrentPage()

	maxButtons := p.MaxButtonCount
	if maxButtons <= 0 {
		maxButtons = 10
	}
	start := cur - maxButtons/2
	if start < 1 {
		start = 1
	}
	end := start + maxButtons - 1
	if end > pc {
		end = pc
		if pc > 0 {
			start = end - maxButtons + 1
		}
	}
	if start < 1 {
		start = 1
	}

	var bs []pagerButton
	if p.FirstPageLabel != "" {
		bs = append(bs, pagerButton{
			Label:    p.FirstPageLabel,
			URL:      p.urlFor(1),
			Class:    classes("first", activeClass(cur == 1), disabledClass(cur == 1)),
			Disabled: cur == 1,
		})
	}
	if p.Pagination.HasPrev() {
		bs = append(bs, pagerButton{Label: p.label("«", p.PrevPageLabel), URL: p.urlFor(cur - 1), Class: "prev"})
	} else {
		bs = append(bs, pagerButton{Label: p.label("«", p.PrevPageLabel), Class: "prev disabled", Disabled: true})
	}
	for page := start; page <= end; page++ {
		bs = append(bs, pagerButton{
			Label: strconv.Itoa(page),
			URL:   p.urlFor(page),
			Class: activeClass(page == cur),
		})
	}
	if p.Pagination.HasNext() {
		bs = append(bs, pagerButton{Label: p.label("»", p.NextPageLabel), URL: p.urlFor(cur + 1), Class: "next"})
	} else {
		bs = append(bs, pagerButton{Label: p.label("»", p.NextPageLabel), Class: "next disabled", Disabled: true})
	}
	if p.LastPageLabel != "" {
		atEnd := cur == pc || pc == 0
		bs = append(bs, pagerButton{
			Label:    p.LastPageLabel,
			URL:      p.urlFor(max(pc, 1)),
			Class:    classes("last", activeClass(cur == pc && pc > 0), disabledClass(atEnd)),
			Disabled: atEnd,
		})
	}
	return bs
}

func activeClass(active bool) string {
	if active {
		return "active"
	}
	return ""
}

func disabledClass(disabled bool) string {
	if disabled {
		return "disabled"
	}
	return ""
}

// Render returns the pagination bar component; Blank when HideOnSinglePage
// holds and everything fits one page.
func (p LinkPager) Render() templ.Component {
	if p.HideOnSinglePage && p.Pagination.PageCount() <= 1 {
		return Blank()
	}
	return linkPager(p.buttons())
}
