// ReportController: serves the nested /admin/report submodule — the
// second-level module of the nesting demonstration.
package main

import (
	"github.com/goyii2/framework/web"
)

// ReportController serves the nested /admin/report submodule. Its explicit
// route is named "dashboard" at the mount site; the module path joins in,
// so the name the application knows is "admin/report/dashboard".
type ReportController struct {
	web.Controller
}

func (rc *ReportController) ActionDashboard(c *web.Context) error {
	return c.JSON(map[string]any{
		"module": "admin/report",
		"user":   c.User().Identity().GetName(),
	})
}
