package view_test

import (
	"strings"
	"testing"

	"github.com/goyii2/framework/view"
)

func TestShellRendersFullDocument(t *testing.T) {
	p := view.Page{
		Title:   "Dashboard",
		Head:    text(`<meta name="x" content="y"/>`),
		Nav:     text(`<nav>top</nav>`),
		Content: text("<p>body</p>"),
		Footer:  text("<footer>bot</footer>"),
	}
	got := render(t, view.Shell(p))

	// templ lowercases the doctype and void-element slashes; the head slot
	// passes through untouched.
	for _, want := range []string{
		"<!doctype html>",
		"<html lang=\"en\">",
		"<title>Dashboard</title>",
		`<meta name="x" content="y"/>`,
		"<nav>top</nav>",
		"<main><p>body</p></main>",
		"<footer>bot</footer>",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("shell output missing %q:\n%s", want, got)
		}
	}
}

func TestShellOmitsEmptySlots(t *testing.T) {
	got := render(t, view.Shell(view.Page{Title: "T", Content: text("c")}))
	if strings.Contains(got, "<nav>") || strings.Contains(got, "<footer>") {
		t.Errorf("unset slots rendered:\n%s", got)
	}
	if !strings.Contains(got, "<main>c</main>") {
		t.Errorf("content missing:\n%s", got)
	}
}

func TestShellNilContent(t *testing.T) {
	got := render(t, view.Shell(view.Page{Title: "T"}))
	if !strings.Contains(got, "<main></main>") {
		t.Errorf("nil content should render an empty main:\n%s", got)
	}
}

func TestBlankRendersNothing(t *testing.T) {
	if got := render(t, view.Blank()); got != "" {
		t.Errorf("Blank() rendered %q, want empty", got)
	}
}
