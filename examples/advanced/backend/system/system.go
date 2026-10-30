// Package system is the backend partition's nested submodule: /backend/system
// extends the backend module's prefix by exactly one segment and hosts the
// template's user management. The slice exists to demonstrate the convention
// side of module routing — the submodule's controllers register in its own
// controller registry and resolve under /backend/system/<controller>/<action>
// through the convention fallback, with no explicit route declared anywhere
// (the basic example's /admin/report shows the explicit-mount counterpart).
// The submodule declares no guard of its own either: the backend module's
// chain (login redirect, then the admin role rule) wraps nested modules'
// routes outermost, so /backend/system is protected exactly like /backend.
package system

import (
	"github.com/goyii2/framework/base"
	"github.com/goyii2/framework/examples/advanced/backend/views"
)

// prefix is the submodule's mount point — the backend partition's territory
// plus one segment, the shape the parent registration enforces.
const prefix = "/backend/system"

// listRoute is the route id of the user listing. It names no registered
// route: links and pager addresses resolve through the convention fallback of
// reverse URL generation ("/" + id), the same convention that routes the
// requests in the first place.
const listRoute = "backend/system/user/list"

// Register wires the submodule below the backend module: it is created for
// the same application, registered on its parent (the prefix must extend the
// parent's by exactly one segment), and its one controller joins the
// submodule's registry under the id that routes /backend/system/user/list.
// Nothing else is wired here on purpose — no explicit route, no guard, no
// frame — the convention fallback carries the routes and the ancestor
// module's chain carries the guard.
func Register(parent *base.Module) error {
	mod, err := base.NewModule(parent.App(), prefix)
	if err != nil {
		return err
	}
	if err := parent.RegisterModule(mod); err != nil {
		return err
	}
	// The controller carries the backend partition's dark frame itself: the
	// module-level Layout call binds only explicitly mounted routes, and the
	// convention fallback resolves this controller without passing one.
	user := &UserController{frame: views.Layout(parent.App().Name())}
	return mod.RegisterController("user", user)
}
