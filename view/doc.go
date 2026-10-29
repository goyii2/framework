// Package view is the templ-based view layer: a named-view registry the web
// layer renders through (no reflection — a map lookup returns a component
// factory), page layouts composed from templ components, and the built-in
// widget set (LinkPager, Breadcrumbs, Menu, DetailView, ActiveForm,
// GridView, ListView).
//
// GridView and ListView are the list widgets: pure renderers over a
// prepared dataprovider result — the action hands in the items (through an
// index closure bridging the provider's generic slice), the paging metadata
// (Pagination), the sort query value and the filter echo values, and the
// widget never queries on its own.
//
// The package depends only on templ, the standard library and the
// framework's own validators package (ActiveForm consumes its validation
// errors), so it stays importable from every layer of an application. HTML
// escaping follows templ defaults: text interpolated in templates is
// escaped, URLs go through templ.URL which only allows http(s)/mailto
// schemes.
package view
