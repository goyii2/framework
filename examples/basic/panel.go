// PanelController: the /admin module's dashboard, guarded by the module's
// own access-control middleware rather than any per-controller rule.
package main

import (
	"github.com/goyii2/framework/url"
	"github.com/goyii2/framework/web"
)

// PanelController is the /admin module's dashboard. It declares no
// middleware of its own: the module-level access control installed on the
// module's routes guards every request below /admin — this convention
// route included — so only logged-in users ever reach the action.
type PanelController struct {
	web.Controller
}

func (panel *PanelController) ActionIndex(c *web.Context) error {
	return c.JSON(map[string]any{
		"module": "admin",
		"user":   c.User().Identity().GetName(),
		// The submodule's route name carries the module path automatically.
		"report": url.To("admin/report/dashboard", nil),
	})
}
