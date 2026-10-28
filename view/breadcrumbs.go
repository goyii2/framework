package view

import "github.com/a-h/templ"

// Crumb is one breadcrumb trail entry. An entry with a URL renders as a
// link; one without renders as the trail's current stop (unlinked,
// highlighted), which is how the last entry marks the current page.
type Crumb struct {
	Label string
	URL   string
}

// Breadcrumbs renders the trail navigation (Home / Posts / Current page).
// Configure as a struct and call Render to obtain the component.
type Breadcrumbs struct {
	Links []Crumb
}

// Render returns the breadcrumb trail as an ordered list of links.
func (b Breadcrumbs) Render() templ.Component { return breadcrumbs(b.Links) }
