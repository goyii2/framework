// View plumbing of the backend partition: the data shape the dashboard
// action passes to its templ component, the dark frame mounted as the
// module's layout, and the named-view registration. The frame is a plain
// view.Layout (no request state of its own — request-dependent bits belong
// to the pages), the brand name is bound when Register assembles the
// module. Controllers construct the exported data types; the templates and
// the chrome helpers stay private to this package.
package views

import (
	"github.com/a-h/templ"

	"github.com/goyii2/framework/view"
)

// DashboardData drives the dashboard page. User carries the account name
// the session resolved — the guard guarantees a signed-in admin by the
// time the page renders.
type DashboardData struct {
	AppName string
	User    string
}

// Layout builds the backend's dark frame around every page of the module:
// a dark sidebar with the brand and the module's navigation beside the
// content area — the visual counterpart of the frontend's light shell, so
// the two applications of the template are told apart at a glance.
func Layout(appName string) view.Layout {
	return func(p view.Page) templ.Component {
		return shell(shellData{AppName: appName}, p.Title, p.Content)
	}
}

// Register puts the backend's pages into the named-view registry behind
// Context.Render. The unified error pages stay the frontend's: the
// registry is handler-wide, and the backend partition has no error view of
// its own to override them with.
func Register(r *view.Registry) {
	r.Register("backend/dashboard", view.Typed(dashboardPage))
}
