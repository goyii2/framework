// Package backend is the backend application of the advanced example —
// the Yii2 advanced template's backend maps onto a /backend module: its
// own dark frame, its own controller registry and one module-wide guard.
// The guard is the template's single deliberate divergence from Yii2's
// separate front/back logins: this example has one login, and the module
// requires the admin role through the RBAC auth manager — accounts carry
// no role attributes of their own, the seed command grants the role.
// Register is the partition's assembly function: main.go calls it with
// the handler, the application and the auth manager it installed, and
// everything the backend owns — its frame, its guard and its dashboard —
// is wired here.
package backend

import (
	"fmt"
	"net/http"

	"github.com/goyii2/framework/base"
	"github.com/goyii2/framework/examples/advanced/backend/controllers"
	"github.com/goyii2/framework/examples/advanced/backend/views"
	"github.com/goyii2/framework/web"
)

// prefix is the module's mount point — the backend application's URL
// territory, and the scope its guard protects.
const prefix = "/backend"

// loginRoute is the named route guests are pointed at. The frontend
// partition owns the login page (this template has a single login), so the
// guard resolves the address by name instead of hardcoding another
// partition's path.
const loginRoute = "site/login"

// Register wires the backend partition: the /backend module, the module
// chain guarding every route below the prefix (guests are redirected to
// the login page, everyone else must hold the admin role through the auth
// manager), the dark frame around the module's pages, and the dashboard in
// the module's controller registry (so /backend/dashboard/index resolves
// through the convention fallback as well).
func Register(h *web.Handler, app *base.Application, auth web.AccessChecker) error {
	login, err := loginURL(h)
	if err != nil {
		return err
	}

	mod, err := base.NewModule(app, prefix)
	if err != nil {
		return err
	}
	if err := app.RegisterModule(mod); err != nil {
		return err
	}
	routes, err := h.Module(prefix)
	if err != nil {
		return err
	}
	views.Register(h.Views())

	// The guard chain, outermost first: unauthenticated requests are
	// pointed at the login page (a redirect reads as "sign in first",
	// where a bare 403 would read as "never"), and the ones that carry a
	// session face the role rule — "admin" is checked through the auth
	// manager, never through account attributes.
	routes.Use(loginRequired(login))
	routes.Use(web.AccessControl{Auth: auth, Rules: []web.AccessRule{
		{Allow: true, Roles: []string{"admin"}},
	}}.Middleware())

	// The dark frame replaces the frontend's light shell for the module's
	// pages; actions can still override it per page with Context.SetLayout.
	routes.Layout(views.Layout(app.Name()))

	dash := &controllers.DashboardController{}
	if err := mod.RegisterController("dashboard", dash); err != nil {
		return err
	}
	routes.Get("{$}", dash.ActionIndex).Name("dashboard")

	// The bare prefix is not mountable through the module registrar (every
	// module route joins below the prefix), so it answers with a canonical
	// redirect to the module home; the guard applies one hop later, at
	// /backend/ — a redirecting request carries no session state to lose.
	h.Get(prefix, func(c *web.Context) error {
		return c.Redirect(http.StatusFound, prefix+"/")
	})
	return nil
}

// loginURL resolves the guard's redirect target from the named route
// table. A missing route means the frontend partition was not registered
// (or registered after the backend) — an assembly-order mistake that must
// stop the process instead of redirecting browsers to a convention-built
// ghost path.
func loginURL(h *web.Handler) (string, error) {
	if !h.URLs().Has(loginRoute) {
		return "", fmt.Errorf("backend: route %q is not registered — register the frontend partition first", loginRoute)
	}
	return h.URLs().URL(loginRoute, nil)
}

// loginRequired redirects unauthenticated requests to the login page and
// passes everyone else through.
func loginRequired(login string) web.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if c := web.FromContext(r.Context()); c != nil && c.User().IsGuest() {
				http.Redirect(w, r, login, http.StatusFound)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
