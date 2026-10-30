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
// page, the named views behind Context.Render, the explicit routes (home,
// the public post pages, the login form and its submission, logout) and the
// site and post controllers in the root registry, so /site/{action} and
// /post/{action} resolve through the convention fallback as well — the
// root-scope mapping the Yii2 advanced template gives its frontend
// application.
func Register(h *web.Handler, app *base.Application) error {
	h.Use(views.Layout())
	views.Register(h.Views())

	site := &controllers.SiteController{}
	sc := h.Ctrl(site)
	sc.Get("/", site.ActionIndex).Name("site/index")
	sc.Get(views.LoginURL, site.ActionLoginForm).Name("site/login")
	sc.Post(views.LoginURL, site.ActionLogin)
	sc.Post(views.LogoutURL, site.ActionLogout).Name("site/logout")

	post := &controllers.PostController{}
	pq := h.Ctrl(post)
	pq.Get("/posts", post.ActionIndex).Name("post/index")
	pq.Get("/posts/{id:[0-9]+}", post.ActionView).Name("post/view")

	if err := app.RegisterController("site", site); err != nil {
		return err
	}
	return app.RegisterController("post", post)
}
