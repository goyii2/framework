// Package view is the templ-based view layer: a named-view registry the web
// layer renders through (no reflection — a map lookup returns a component
// factory), page layouts composed from templ components, and the built-in
// widget set (LinkPager, Breadcrumbs, Menu, DetailView).
//
// The package depends only on templ and the standard library, so it stays
// importable from every layer of an application. HTML escaping follows templ
// defaults: text interpolated in templates is escaped, URLs go through
// templ.URL which only allows http(s)/mailto schemes.
package view
