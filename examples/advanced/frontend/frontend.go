// Package frontend is the frontend application of the advanced example —
// the Yii2 advanced template's frontend maps onto the handler's root scope
// (not a module), so its routes answer directly below "/". Register is the
// partition's assembly function: main.go calls it with the handler and the
// application, and everything the frontend owns — its light layout, its
// named views and its root-scope routes — is wired here.
package frontend

import (
	"github.com/goyii2/framework/base"
	"github.com/goyii2/framework/examples/advanced/frontend/controllers"
	"github.com/goyii2/framework/examples/advanced/frontend/views"
	"github.com/goyii2/framework/web"
)

// Register wires the frontend partition: the light layout around every
// page, the named views behind Context.Render, the explicit "/" home route
// and the site controller in the root registry, so /site/{action} resolves
// through the convention fallback as well — the root-scope mapping the
// Yii2 advanced template gives its frontend application.
func Register(h *web.Handler, app *base.Application) error {
	h.Use(views.Layout())
	views.Register(h.Views())

	site := &controllers.SiteController{}
	sc := h.Ctrl(site)
	sc.Get("/", site.ActionIndex).Name("site/index")

	return app.RegisterController("site", site)
}
