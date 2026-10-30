// UserController is the system submodule's account management: the listing of
// the accounts every partition shares. The listing demonstrates the list
// pipeline the template's other pages reuse — an ActiveDataProvider pushing
// pagination and sorting down to SQL, the LinkPager widget rendering the
// paging bar — inside the nested module. The backend module's guard already
// filtered the audience (signed-in admins only), so the action lists records
// and never re-checks access.
package system

import (
	"net/http"
	"strconv"

	"github.com/goyii2/framework/dataprovider"
	"github.com/goyii2/framework/db/ar"
	"github.com/goyii2/framework/examples/advanced/backend/views"
	"github.com/goyii2/framework/examples/advanced/common/models"
	"github.com/goyii2/framework/url"
	"github.com/goyii2/framework/view"
	"github.com/goyii2/framework/web"
)

// usersPerPage fixes the listing's page size — small on purpose, so the demo
// accounts plus a handful of user/create runs already turn the pager on. An
// explicit per-page from the query string still wins (the parser clamped it).
const usersPerPage = 10

// sortableUserFields whitelists the columns a sort request may name; the
// account row exposes nothing else worth ordering by.
var sortableUserFields = []string{"id", "username"}

// UserController carries the user management actions of the system submodule.
// frame is the backend partition's dark shell, set by the assembly and bound
// to every page the controller renders.
type UserController struct {
	web.Controller
	frame view.Layout
}

// Middleware frames the controller's pages in the backend partition's dark
// shell. The submodule serves through the convention fallback, which resolves
// actions without passing the module registrar where a module-wide Layout
// binds — so the controller carries its frame the way the frame-composition
// doc describes: layout-setting middleware on the controller, innermost after
// the module chain.
func (uc *UserController) Middleware() []web.Middleware {
	frame := uc.frame
	return []web.Middleware{func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if c := web.FromContext(r.Context()); c != nil {
				c.SetLayout(frame)
			}
			next.ServeHTTP(w, r)
		})
	}}
}

// ActionList is the account listing: the DataProvider pages the user table
// newest first, the rows and the paging bar render in the dark frame. The
// account row holds a password hash only the login flow reads — the listing
// shows the login names, never the hashes. The route is conventional:
// /backend/system/user/list, resolved by the fallback with no explicit
// declaration; the pager addresses pages by the same convention-built URL.
func (uc *UserController) ActionList(c *web.Context) error {
	params, err := dataprovider.FromRequest(c.Request(), sortableUserFields...)
	if err != nil {
		return web.BadRequest(err.Error()) // a sort request outside the whitelist
	}
	if c.Request().URL.Query().Get("per-page") == "" {
		params.PerPage = usersPerPage
	}
	res, err := dataprovider.ActiveDataProvider[models.User]{
		Query:        ar.New[models.User](c.Context()),
		Params:       params,
		DefaultOrder: []dataprovider.Sort{{Column: "id", Desc: true}},
	}.Prepare()
	if err != nil {
		return err
	}

	rows := make([]views.SystemUserRow, 0, len(res.Items))
	for _, user := range res.Items {
		rows = append(rows, views.SystemUserRow{
			ID:       strconv.FormatInt(user.ID, 10),
			Username: user.Username,
		})
	}
	pager := view.LinkPager{
		Pagination:     res.Pagination(),
		MaxButtonCount: 5,
		URL: func(page int) string {
			return url.To(listRoute, url.P{"page": strconv.Itoa(page)})
		},
	}.Render()
	c.SetTitle("Users")
	return c.Render("backend/system/user", views.SystemUserData{
		Rows:       rows,
		TotalCount: res.TotalCount,
		Page:       res.Page,
		PageCount:  res.PageCount,
		Pager:      pager,
	})
}
