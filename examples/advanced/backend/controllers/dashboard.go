// Package controllers holds the backend partition's controllers — the
// pages of the /backend module. The nested system submodule (user
// management) lives in its own package below backend/system.
package controllers

import (
	"github.com/goyii2/framework/examples/advanced/backend/views"
	"github.com/goyii2/framework/web"
)

// DashboardController is the backend's landing controller: the page an
// admin sees after the guard lets them through.
type DashboardController struct {
	web.Controller
}

// ActionIndex renders the dashboard: the module's home page inside the
// dark frame, greeting the account the session resolved.
func (dash *DashboardController) ActionIndex(c *web.Context) error {
	c.SetTitle("Dashboard")
	data := views.DashboardData{AppName: c.App().Name()}
	if identity := c.User().Identity(); identity != nil {
		data.User = identity.GetName()
	}
	return c.Render("backend/dashboard", data)
}
