// View plumbing of the browser pages: the data shapes actions pass to the
// templ components, the application layout (menu, session area, language
// switch around the content) and the named-view registrations.
package main

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/a-h/templ"

	"github.com/goyii2/framework/i18n"
	"github.com/goyii2/framework/url"
	"github.com/goyii2/framework/view"
	"github.com/goyii2/framework/web"
)

// navData is the chrome of every framed page. It is assembled at render
// time, so the menu reflects the identity and permissions of the request
// being answered.
type navData struct {
	AppName   string
	Lang      string // html lang attribute
	Menu      templ.Component
	User      string // display name; empty for guests
	CSRF      string
	LoginURL  string
	LogoutURL string
	Langs     []langLink
}

// langLink is one entry of the language switcher.
type langLink struct {
	Label   string
	URL     string
	Current bool
}

// homeData drives the front page.
type homeData struct {
	SignedIn  bool
	PostsURL  string
	LoginURL  string
	NotifyURL string
	CSRF      string
}

// loginData drives the login form; Errors carries the inline schema issues
// per field, Error the general (wrong credentials) note.
type loginData struct {
	Action   string
	CSRF     string
	Username string
	Errors   map[string][]string
	Error    string
}

// postRow is one list row with everything the template renders: display
// strings and per-row action links, permissions already resolved.
type postRow struct {
	ID        string
	Title     string
	Status    string
	Author    string
	CreatedAt string
	ViewURL   string
	EditURL   string
	ShowEdit  bool
}

// sortHeader is one sortable column heading: the toggle link and the active
// direction marker.
type sortHeader struct {
	URL   string
	Arrow string
}

// postsListData drives the listing page. Query is the active search text —
// echoed into the search box and switched on for the empty-search hint — and
// SearchAction the endpoint the GET form submits to.
type postsListData struct {
	Rows         []postRow
	Total        int
	Pager        templ.Component
	SortID       sortHeader
	SortTitle    sortHeader
	SortStatus   sortHeader
	SortCreated  sortHeader
	CanCreate    bool
	CreateURL    string
	SearchAction string
	Query        string
}

// postShowData drives the detail page.
type postShowData struct {
	Breadcrumbs templ.Component
	Detail      templ.Component
	Title       string
	CanUpdate   bool
	CanDelete   bool
	EditURL     string
	DeleteURL   string
	BackURL     string
	CSRF        string
}

// postFormData drives the shared create/edit form; on 422 re-renders,
// Errors carries the save pipeline's field -> messages map and the entered
// values echo back into the inputs.
type postFormData struct {
	Breadcrumbs templ.Component
	Heading     string
	Action      string
	CancelURL   string
	CSRF        string
	Title       string
	Body        string
	Status      string
	Errors      map[string][]string
}

// notifyData drives the mailer result page.
type notifyData struct {
	Recipient string
	HomeURL   string
}

// fieldErrors joins one field's validation messages for display below its
// input.
func fieldErrors(errors map[string][]string, field string) string {
	msgs := errors[field]
	if len(msgs) == 0 {
		return ""
	}
	return strings.Join(msgs, "; ")
}

// pageLayout installs the application frame on every request: the frame
// wraps HTML renders only, so JSON replies never touch it. The frame closes
// over the request Context, which keeps the navigation current (identity,
// permissions, display language) even for error pages rendered after the
// action failed.
func pageLayout() web.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if c := web.FromContext(r.Context()); c != nil {
				c.SetLayout(appLayout(c))
			}
			next.ServeHTTP(w, r)
		})
	}
}

// appLayout adapts one request into its page frame.
func appLayout(c *web.Context) view.Layout {
	return func(p view.Page) templ.Component {
		return appShell(buildNav(c), p.Title, p.Content)
	}
}

// offeredLangs are the languages the switcher offers; each label is written
// in its own language on purpose.
var offeredLangs = []struct{ Tag, Label string }{
	{"en-US", "English"},
	{"zh-CN", "中文"},
}

// buildNav assembles the chrome at render time. The menu is the view.Menu
// widget; the RBAC manager decides which items appear — editor and admin
// hold post/create through the role hierarchy, so both see "New post" while
// a plain account does not, and nobody but admin inherits post/delete.
func buildNav(c *web.Context) navData {
	ctx := c.Context()
	lang, _ := i18n.LanguageFromContext(ctx)
	nav := navData{
		AppName:   c.App().Config().App.Name,
		Lang:      lang,
		LoginURL:  url.To("site/login", nil),
		LogoutURL: url.To("site/logout", nil),
		CSRF:      c.CsrfToken(),
	}

	path := c.Request().URL.Path
	active := func(target string) bool { return path == target }
	items := []view.MenuItem{
		{Label: i18n.T(ctx, "app", "Home", nil), URL: url.To("site/home", nil), Active: active("/")},
		{Label: i18n.T(ctx, "app", "Posts", nil), URL: url.To("post/list", nil), Active: active("/posts")},
	}
	if c.User() != nil && c.User().Can("post/create") {
		items = append(items, view.MenuItem{
			Label:  i18n.T(ctx, "app", "New post", nil),
			URL:    url.To("post/new", nil),
			Active: active("/posts/new"),
		})
	}
	nav.Menu = view.Menu{Items: items}.Render()

	if c.User() != nil {
		if identity := c.User().Identity(); identity != nil {
			nav.User = identity.GetName()
		}
	}
	for _, l := range offeredLangs {
		nav.Langs = append(nav.Langs, langLink{
			Label:   l.Label,
			URL:     path + "?lang=" + l.Tag,
			Current: lang == l.Tag,
		})
	}
	return nav
}

// breadcrumbs builds the trail of an inner page: Home / Posts / the current
// stop, whose missing URL marks it as the active page.
func breadcrumbs(ctx context.Context, current string) templ.Component {
	return view.Breadcrumbs{Links: []view.Crumb{
		{Label: i18n.T(ctx, "app", "Home", nil), URL: url.To("site/home", nil)},
		{Label: i18n.T(ctx, "app", "Posts", nil), URL: url.To("post/list", nil)},
		{Label: current},
	}}.Render()
}

// registerViews puts every page of the application into the named-view
// registry behind ctx.Render, plus the "error" override that dresses the
// unified error handler's HTML replies in the application frame. Known
// message texts translate; unknown ones render verbatim.
func registerViews(r *view.Registry) {
	r.Register("site/home", view.Typed(homePage))
	r.Register("site/login", view.Typed(loginPage))
	r.Register("site/notify", view.Typed(notifyPage))
	r.Register("post/list", view.Typed(postsPage))
	r.Register("post/show", view.Typed(postShowPage))
	r.Register("post/form", view.Typed(postFormPage))
	r.Register("error", view.Typed(func(d view.ErrorData) templ.Component {
		return errorPage(errorPageData{
			Heading: strconv.Itoa(d.Status) + " " + d.Name,
			Message: d.Message,
		})
	}))
}
