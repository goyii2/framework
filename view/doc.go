// Package view is the templ-based view layer: a named-view registry the web
// layer renders through (no reflection — a map lookup returns a component
// factory), page layouts composed from templ components, and the built-in
// widget set (LinkPager, Breadcrumbs, Menu, DetailView, ActiveForm).
//
// The package depends only on templ, the standard library and the
// framework's own validators package (ActiveForm consumes its validation
// errors), so it stays importable from every layer of an application. HTML
// escaping follows templ defaults: text interpolated in templates is
// escaped, URLs go through templ.URL which only allows http(s)/mailto
// schemes.
package view
