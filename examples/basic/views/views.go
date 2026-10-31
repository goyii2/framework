// View plumbing of the browser pages: the data shapes actions pass to the
// templ components, the application layout (menu, session area, language
// switch around the content) and the named-view registrations. Controllers
// construct the exported data types; the templates and the chrome helpers
// stay private to this package.
package views

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

// HeadAssets is the Vite entry of the frontend (script and stylesheet tags),
// resolved once at startup in main and rendered into every page's <head> by
// the application layout.
var HeadAssets templ.Component

// navData is the chrome of every framed page. It is assembled at render
// time, so the menu reflects the identity and permissions of the request
// being answered. Head carries the frontend entry tags.
type navData struct {
	AppName   string
	Lang      string // html lang attribute
	Head      templ.Component
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

// HomeData drives the front page.
type HomeData struct {
	SignedIn  bool
	PostsURL  string
	LoginURL  string
	NotifyURL string
	CSRF      string
}

// LoginData drives the login form; Errors carries the inline schema issues
// per field, Error the general (wrong credentials) note.
type LoginData struct {
	Action   string
	CSRF     string
	Username string
	Errors   map[string][]string
	Error    string
}

// PostsListData drives the listing page: the rendered GridView (sortable
// headers and the filter row), the pager and the summary line; permissions
// already resolved.
type PostsListData struct {
	Grid      templ.Component
	Pager     templ.Component
	Total     int
	CanCreate bool
	CreateURL string
}

// PostShowData drives the detail page. PermalinkURL is the one absolute link
// of the browser face — the host info main.go installs on the url manager
// prefixed onto the named route — while every other link stays relative.
type PostShowData struct {
	Breadcrumbs  templ.Component
	Detail       templ.Component
	Title        string
	PermalinkURL string
	CanUpdate    bool
	CanDelete    bool
	EditURL      string
	DeleteURL    string
	BackURL      string
	CSRF         string
}

// PostFormData drives the shared create/edit form page; Form is the
// ActiveForm widget's rendered component — labels, controls and, on a 422
// re-render, the validators messages under the failing fields with the
// submitted values refilled.
type PostFormData struct {
	Breadcrumbs templ.Component
	Heading     string
	Form        templ.Component
	CancelURL   string
}

// NotifyData drives the notification result page: the recipient the message
// went to and the sender the notifier composed it from (the custom email
// section's value, visible next to the recipient).
type NotifyData struct {
	Recipient string
	Sender    string
	HomeURL   string
}

// QueuePageData drives the queue demo page. The counts are preformatted
// strings — templ text interpolation writes strings only.
type QueuePageData struct {
	Waiting  string
	Reserved string
	Failed   string
	// PushedID echoes the message id of a just-pushed job (empty otherwise).
	PushedID string
	PushURL  string
	CSRF     string
}

// PageCacheData drives the page-cache demonstration page. GeneratedAt is
// stamped per render — a replayed copy serves the frozen value.
type PageCacheData struct {
	GeneratedAt string
	TTL         string
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

// Layout installs the application frame on every request: the frame wraps
// HTML renders only, so JSON replies never touch it. The frame closes over
// the request Context, which keeps the navigation current (identity,
// permissions, display language) even for error pages rendered after the
// action failed.
func Layout() web.Middleware {
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
		AppName:   c.App().Name(),
		Lang:      lang,
		Head:      HeadAssets,
		LoginURL:  url.To("site/login", nil),
		LogoutURL: url.To("site/logout", nil),
		CSRF:      c.CsrfToken(),
	}

	path := c.Request().URL.Path
	active := func(target string) bool { return path == target }
	items := []view.MenuItem{
		{Label: i18n.T(ctx, "app", "Home", nil), URL: url.To("site/home", nil), Active: active("/")},
		{Label: i18n.T(ctx, "app", "Posts", nil), URL: url.To("post/list", nil), Active: active("/posts")},
		{Label: i18n.T(ctx, "app", "Queue", nil), URL: url.To("queue/index", nil), Active: active("/queue")},
		{Label: i18n.T(ctx, "app", "Page cache", nil), URL: url.To("site/pagecache", nil), Active: active("/pagecache")},
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

// Breadcrumbs builds the trail of an inner page: Home / Posts / the current
// stop, whose missing URL marks it as the active page.
func Breadcrumbs(ctx context.Context, current string) templ.Component {
	return view.Breadcrumbs{Links: []view.Crumb{
		{Label: i18n.T(ctx, "app", "Home", nil), URL: url.To("site/home", nil)},
		{Label: i18n.T(ctx, "app", "Posts", nil), URL: url.To("post/list", nil)},
		{Label: current},
	}}.Render()
}

// Register puts every page of the application into the named-view registry
// behind ctx.Render, plus the "error" override that dresses the unified
// error handler's HTML replies in the application frame. Known message
// texts translate; unknown ones render verbatim.
func Register(r *view.Registry) {
	r.Register("site/home", view.Typed(homePage))
	r.Register("site/login", view.Typed(loginPage))
	r.Register("site/notify", view.Typed(notifyPage))
	r.Register("site/pagecache", view.Typed(pageCachePage))
	r.Register("queue/index", view.Typed(queuePage))
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
