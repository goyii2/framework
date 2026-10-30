// View plumbing of the backend partition: the data shapes actions pass to
// the templ components, the dark frame mounted as the module's layout, and
// the named-view registrations. Controllers construct the exported data
// types; the templates and the chrome helpers stay private to this package.
package views

import (
	"context"

	"github.com/a-h/templ"

	"github.com/goyii2/framework/url"
	"github.com/goyii2/framework/view"
	"github.com/goyii2/framework/web"
)

// DashboardData drives the dashboard page. User carries the account name
// the session resolved — the guard guarantees a signed-in admin by the
// time the page renders.
type DashboardData struct {
	AppName string
	User    string
}

// PostIndexData drives the management listing: one row per post, the
// compose entry point and the CSRF token the row-level delete forms echo.
type PostIndexData struct {
	Rows      []PostRow
	CreateURL string
	CSRF      string
}

// PostRow is one row of the management listing. ViewURL is the frontend's
// public page for the post, resolved by route name across partitions; it
// stays empty for drafts (the frontend does not publish them, so the link
// would land on its 404).
type PostRow struct {
	ID        string
	Title     string
	Status    string
	Author    string
	Updated   string
	ViewURL   string
	EditURL   string
	DeleteURL string
}

// PostFormData drives the compose/edit page: the heading and the rendered
// ActiveForm widget (labels, controls, refilled values and — after a
// rejected submission — the field messages).
type PostFormData struct {
	Heading   string
	Form      templ.Component
	CancelURL string
}

// Layout builds the backend's dark frame around every page of the module:
// a dark sidebar with the brand and the module's navigation beside the
// content area — the visual counterpart of the frontend's light shell, so
// the two applications of the template are told apart at a glance. The
// navigation's addresses resolve by route name at render time, when every
// partition's routes are long registered.
func Layout(appName string) view.Layout {
	return func(p view.Page) templ.Component {
		return shell(shellData{
			AppName:      appName,
			DashboardURL: url.To("backend/dashboard", nil),
			PostsURL:     url.To("backend/post/index", nil),
			HomeURL:      url.To("site/index", nil),
		}, p.Title, p.Content)
	}
}

// navActive reports whether target is the current request path — the
// sidebar's active marker, resolved at render time through the request
// context the layout receives (the frame itself stays request-blind).
func navActive(ctx context.Context, target string) bool {
	c := web.FromContext(ctx)
	return c != nil && c.Request().URL.Path == target
}

// Register puts the backend's pages into the named-view registry behind
// Context.Render. The unified error pages stay the frontend's: the
// registry is handler-wide, and the backend partition has no error view of
// its own to override them with.
func Register(r *view.Registry) {
	r.Register("backend/dashboard", view.Typed(dashboardPage))
	r.Register("backend/post/index", view.Typed(postIndexPage))
	r.Register("backend/post/form", view.Typed(postFormPage))
}
