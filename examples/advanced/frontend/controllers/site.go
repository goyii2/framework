// SiteController carries the frontend's public pages. This slice holds the
// home page; login and the post list/detail join in later slices.
package controllers

import (
	"github.com/goyii2/framework/examples/advanced/frontend/views"
	"github.com/goyii2/framework/web"
)

// SiteController is the frontend's site controller: the pages around the
// application's public face.
type SiteController struct {
	web.Controller
}

// ActionIndex is the front page: a light welcome page that also shows the
// environment the configuration resolved (the dev overlay's observable
// difference), framed by the frontend layout.
func (site *SiteController) ActionIndex(c *web.Context) error {
	c.SetTitle("Home")
	return c.Render("site/index", views.HomeData{
		AppName:  c.App().Name(),
		Env:      c.App().Env(),
		Debug:    c.App().Debug(),
		LoginURL: views.LoginPlaceholder,
	})
}
