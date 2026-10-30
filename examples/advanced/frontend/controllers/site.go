// SiteController carries the frontend's public pages: the home page and the
// session entry points around authentication (the login form, the login
// submission, logout). The post list and detail join in later slices.
package controllers

import (
	"net/http"

	z "github.com/Oudwins/zog"

	"github.com/goyii2/framework/db/ar"
	"github.com/goyii2/framework/examples/advanced/common/models"
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
// difference) and the session state, framed by the frontend layout.
func (site *SiteController) ActionIndex(c *web.Context) error {
	c.SetTitle("Home")
	data := views.HomeData{
		AppName:  c.App().Name(),
		Env:      c.App().Env(),
		Debug:    c.App().Debug(),
		LoginURL: views.LoginURL,
	}
	if c.User() != nil {
		if identity := c.User().Identity(); identity != nil {
			data.SignedIn, data.User = true, identity.GetName()
		}
	}
	return c.Render("site/index", data)
}

// loginPayload is the submitted form of ActionLogin.
type loginPayload struct {
	Username string
	Password string
}

// loginSchema checks the submission before any database lookup happens, so
// blank fields answer with their own notes instead of a credentials error.
var loginSchema = z.Struct(z.Shape{
	"username": z.String().Required(z.Message("username cannot be blank")),
	"password": z.String().Required(z.Message("password cannot be blank")),
})

// ActionLoginForm renders the login page. The form posts back to /login,
// carrying the CSRF token in the hidden field the middleware verifies.
func (site *SiteController) ActionLoginForm(c *web.Context) error {
	c.SetTitle("Login")
	return c.Render("site/login", views.LoginData{
		Action: views.LoginURL,
		CSRF:   c.CsrfToken(),
	})
}

// ActionLogin authenticates the form submission against the user table and
// binds the account to the session. Unknown users and wrong passwords answer
// the same 401 — the reply must not reveal which half was wrong — with the
// form re-rendered and the submitted username refilled; no login state is
// stored, so the failure leaves the requester a guest.
func (site *SiteController) ActionLogin(c *web.Context) error {
	payload := loginPayload{
		Username: c.Request().PostFormValue("username"),
		Password: c.Request().PostFormValue("password"),
	}
	if issues := loginSchema.Validate(&payload); len(issues) > 0 {
		return site.loginForm(c, http.StatusUnprocessableEntity, views.LoginData{
			Username: payload.Username,
			Errors:   z.Issues.Flatten(issues),
		})
	}
	user, err := ar.New[models.User](c.Context()).Where("username = ?", payload.Username).One()
	if err != nil || !user.CheckPassword(payload.Password) {
		return site.loginForm(c, http.StatusUnauthorized, views.LoginData{
			Username: payload.Username,
			Error:    "Wrong username or password.",
		})
	}
	if err := c.User().Login(user); err != nil {
		return err
	}
	c.App().Log().Info("user logged in", "user", user.Username)
	return c.Redirect(http.StatusFound, "/")
}

// ActionLogout destroys the session: the stored login, the cookie and the
// request-scoped identity all go away together. Browser submissions land
// back on the home page.
func (site *SiteController) ActionLogout(c *web.Context) error {
	name := "guest"
	if identity := c.User().Identity(); identity != nil {
		name = identity.GetName()
	}
	c.User().Logout()
	c.App().Log().Info("user logged out", "user", name)
	return c.Redirect(http.StatusFound, "/")
}

// loginForm re-renders the login page under status with data filled in.
func (site *SiteController) loginForm(c *web.Context, status int, data views.LoginData) error {
	c.SetTitle("Login")
	data.Action, data.CSRF = views.LoginURL, c.CsrfToken()
	return c.RenderStatus(status, "site/login", data)
}
