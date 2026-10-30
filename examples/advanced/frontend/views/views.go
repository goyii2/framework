// View plumbing of the frontend partition: the data shapes actions pass to
// the templ components, the light layout (the frontend's default page
// frame) and the named-view registrations. The backend partition brings its
// own dark frame through ModuleRoutes.Layout in a later slice. Controllers
// construct the exported data types; the templates and the chrome helpers
// stay private to this package.
package views

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/a-h/templ"

	"github.com/goyii2/framework/url"
	"github.com/goyii2/framework/view"
	"github.com/goyii2/framework/web"
)

// The session area's destinations. The frontend owns the root scope, so the
// plain paths are the named routes' addresses at once; controllers and the
// layout chrome share them, so each has one home.
const (
	LoginURL  = "/login"
	LogoutURL = "/logout"
)

// HomeData drives the front page. Env and Debug echo what the configuration
// resolved for this process — the environment overlay's observable
// difference; SignedIn/User carry the session state the greeting shows.
type HomeData struct {
	AppName  string
	Env      string
	Debug    bool
	SignedIn bool
	User     string
	LoginURL string
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

// PostListData drives the public posts listing.
type PostListData struct {
	Posts []PostCard
}

// PostCard is one entry of the public listing: the title's link resolved by
// route name, a body preview and the byline.
type PostCard struct {
	Title   string
	Excerpt string
	Author  string
	Created string
	ViewURL string
}

// PostShowData drives the public detail page.
type PostShowData struct {
	Title   string
	Body    string
	Author  string
	Created string
	BackURL string
}

// Layout installs the frontend's light frame on every request: the frame
// wraps HTML renders only, so non-HTML replies never touch it. The frame
// closes over the request Context, so it stays current even for error
// pages rendered after an action failed.
func Layout() web.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if c := web.FromContext(r.Context()); c != nil {
				c.SetLayout(frontLayout(c))
			}
			next.ServeHTTP(w, r)
		})
	}
}

// frontLayout adapts one request into its page frame.
func frontLayout(c *web.Context) view.Layout {
	return func(p view.Page) templ.Component {
		return shell(buildShell(c), p.Title, p.Content)
	}
}

// buildShell assembles the chrome at render time: the session area flips on
// the request's identity, so the navigation shows the account name and the
// logout form once a login is bound (and the login link for guests). The
// posts entry resolves by route name — the same table every link of the
// partition reads — and Path marks the navigation's active item.
func buildShell(c *web.Context) shellData {
	data := shellData{
		AppName:   c.App().Name(),
		CSRF:      c.CsrfToken(),
		LoginURL:  LoginURL,
		LogoutURL: LogoutURL,
		PostsURL:  url.To("post/index", nil),
		Path:      c.Request().URL.Path,
	}
	if c.User() != nil {
		if identity := c.User().Identity(); identity != nil {
			data.User = identity.GetName()
		}
	}
	return data
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

// Register puts the frontend's pages into the named-view registry behind
// Context.Render, plus the "error" override that dresses the unified error
// handler's HTML replies in the frontend frame.
func Register(r *view.Registry) {
	r.Register("site/index", view.Typed(homePage))
	r.Register("site/login", view.Typed(loginPage))
	r.Register("post/index", view.Typed(postsPage))
	r.Register("post/view", view.Typed(postViewPage))
	r.Register("error", view.Typed(func(d view.ErrorData) templ.Component {
		return errorPage(errorPageData{
			Heading: strconv.Itoa(d.Status) + " " + d.Name,
			Message: d.Message,
		})
	}))
}
