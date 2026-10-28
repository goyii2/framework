package view

import "github.com/a-h/templ"

// MenuItem is one menu entry: a label, an optional target and nested
// sub-items. An empty URL renders a non-link heading (useful above a
// submenu); Active marks the entry (and, by convention, its ancestors) as
// the current location.
type MenuItem struct {
	Label  string
	URL    string
	Active bool
	Items  []MenuItem
}

// Menu renders a (possibly nested) list navigation. Configure as a struct
// and call Render to obtain the component; nesting depth is unbounded.
type Menu struct {
	Items []MenuItem
}

// Render returns the menu as nested unordered lists.
func (m Menu) Render() templ.Component { return menuList(m.Items) }
