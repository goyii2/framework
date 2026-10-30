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

	"github.com/a-h/templ"

	"github.com/goyii2/framework/view"
	"github.com/goyii2/framework/web"
)

// HomeData drives the front page. Env and Debug echo what the
// configuration resolved for this process — the environment overlay's
// observable difference.
type HomeData struct {
	AppName  string
	Env      string
	Debug    bool
	LoginURL string
}

// LoginPlaceholder is where the login form will live once its slice lands;
// until then the layout's navigation and the front page point here as an
// open placeholder. Controllers and the layout chrome share it, so the
// placeholder has one home.
const LoginPlaceholder = "/login"

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
		return shell(shellData{AppName: c.App().Name(), LoginURL: LoginPlaceholder}, p.Title, p.Content)
	}
}

// Register puts the frontend's pages into the named-view registry behind
// Context.Render, plus the "error" override that dresses the unified error
// handler's HTML replies in the frontend frame.
func Register(r *view.Registry) {
	r.Register("site/index", view.Typed(homePage))
	r.Register("error", view.Typed(func(d view.ErrorData) templ.Component {
		return errorPage(errorPageData{
			Heading: strconv.Itoa(d.Status) + " " + d.Name,
			Message: d.Message,
		})
	}))
}
