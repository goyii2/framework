// SiteController: the entry points around authentication (session
// login/logout, the CSRF round trip, the identity echo) plus the browser
// front pages — home, the login form and the mailer demonstration.
package controllers

import (
	"net/http"

	z "github.com/Oudwins/zog"

	"github.com/goyii2/framework/db/ar"
	"github.com/goyii2/framework/examples/basic/models"
	"github.com/goyii2/framework/examples/basic/notifier"
	"github.com/goyii2/framework/examples/basic/views"
	"github.com/goyii2/framework/form"
	"github.com/goyii2/framework/i18n"
	"github.com/goyii2/framework/url"
	"github.com/goyii2/framework/validators"
	"github.com/goyii2/framework/web"
)

// SiteController carries the entry points around authentication. Its one
// injected dependency, the notifier, is this project's own component: main
// builds it from the custom email section and passes it in — constructor
// injection, no registry. Its middleware scopes the convention-resolved
// /site/index to safe verbs — the VerbFilter demonstration: POST /site/index
// answers 405 with an Allow header instead of running the action.
type SiteController struct {
	web.Controller

	// Notify delivers the transactional demonstration mail; nil only in
	// tests that never touch it.
	Notify *notifier.Notifier
}

func (site *SiteController) Middleware() []web.Middleware {
	return []web.Middleware{
		web.VerbFilter{Rules: []web.VerbRule{
			{Path: "/site/index", Verbs: []string{http.MethodGet, http.MethodHead}},
		}}.Middleware(),
	}
}

// ActionIndex answers through the convention fallback: no explicit route is
// bound for /site/index, the controller registry resolves it instead.
func (site *SiteController) ActionIndex(c *web.Context) error {
	return c.JSON(map[string]any{
		"app": c.App().Name(),
		"env": c.App().Env(),
		"endpoints": []string{
			"/site/csrf", "/login", "/logout", "/me",
			"/post", "/post/{id}",
			"/admin/panel/index", "/admin/report/dashboard",
		},
	})
}

// ActionCsrf hands out the double-submit token: the CSRF middleware issued a
// cookie on this safe request, and the client echoes the same value back on
// every unsafe one (header X-Csrf-Token or form field _csrf).
func (site *SiteController) ActionCsrf(c *web.Context) error {
	return c.JSON(map[string]string{"csrf": c.CsrfToken()})
}

// loginForm is the login form model: a standalone ruled struct — no record,
// no table — that the action binds from the request and validates under the
// login scenario in one LoadForm step. The `form` tags are the binding
// whitelist; the schema keeps the required-field semantics and the message
// ids the translation files already carry.
type loginForm struct {
	Username string `form:"username"`
	Password string `form:"password"`
}

// scenarioLogin is the custom scenario the login form validates under.
const scenarioLogin validators.Scenario = "login"

// Rules requires both fields of a login submission, so blank ones answer
// with their own notes instead of a credentials error.
func (f *loginForm) Rules() validators.RuleSet {
	return validators.RuleSet{
		scenarioLogin: z.Struct(z.Shape{
			"username": z.String().Required(z.Message("username cannot be blank")),
			"password": z.String().Required(z.Message("password cannot be blank")),
		}),
	}
}

// ActionLogin authenticates against the user table and binds the account to
// the session. Unknown users and wrong passwords answer the same 401 — the
// response must not reveal which half was wrong. The action serves both
// shapes of caller: JSON API clients get validated JSON replies, the
// browser login form is re-rendered with its errors under the failing
// status and redirected to the post list on success. Binding and validation
// are one LoadForm step over the form model: a malformed body and a rule
// failure alike come back as the flat per-field error map, translated
// through the validators' message path.
func (site *SiteController) ActionLogin(c *web.Context) error {
	var f loginForm
	html := c.ResponseFormat() == "html"
	if errs := c.LoadForm(&f, scenarioLogin); errs != nil {
		if html {
			c.SetTitle(i18n.T(c.Context(), "app", "Login", nil))
			return c.RenderStatus(http.StatusUnprocessableEntity, "site/login", views.LoginData{
				Action: url.To("site/login", nil),
				CSRF:   c.CsrfToken(),
				// The refilled username is what the model holds after the
				// rejected submission, exported the way every form model
				// refills its view.
				Username: form.Values(&f)["username"],
				Errors:   errs.Fields,
			})
		}
		return c.JSONStatus(http.StatusUnprocessableEntity, errs.Fields)
	}
	user, err := ar.New[models.User](c.Context()).Where("username = ?", f.Username).One()
	if err != nil || !user.CheckPassword(f.Password) {
		if html {
			c.SetTitle(i18n.T(c.Context(), "app", "Login", nil))
			return c.RenderStatus(http.StatusUnauthorized, "site/login", views.LoginData{
				Action:   url.To("site/login", nil),
				CSRF:     c.CsrfToken(),
				Username: form.Values(&f)["username"],
				Error:    i18n.T(c.Context(), "app", "Wrong username or password.", nil),
			})
		}
		return web.Unauthorized("wrong username or password")
	}
	if err := c.User().Login(user); err != nil {
		return err
	}
	c.App().Log().Info("user logged in", "user", user.Username)
	if html {
		return c.Redirect(http.StatusFound, url.To("post/list", nil))
	}
	return c.JSON(map[string]string{"name": user.Username, "role": user.Role})
}

// ActionLogout destroys the session: the stored login, the cookie and the
// request-scoped identity all go away together. Browser submissions land
// back on the home page; API clients get the bare 204.
func (site *SiteController) ActionLogout(c *web.Context) error {
	name := "guest"
	if id := c.User().Identity(); id != nil {
		name = id.GetName()
	}
	c.User().Logout()
	c.App().Log().Info("user logged out", "user", name)
	if c.ResponseFormat() == "html" {
		return c.Redirect(http.StatusFound, url.To("site/home", nil))
	}
	c.Response().WriteHeader(http.StatusNoContent)
	return nil
}

// ActionMe reports the requester the session sees — guest before login, the
// bound identity (name and roles) after.
func (site *SiteController) ActionMe(c *web.Context) error {
	identity := c.User().Identity()
	if identity == nil {
		return c.JSON(map[string]any{"guest": true})
	}
	return c.JSON(map[string]any{
		"guest": false,
		"name":  identity.GetName(),
		"roles": identity.GetRoles(),
	})
}

// ActionHome is the browser front page: an introduction, the entry links
// and the mailer demonstration button.
func (site *SiteController) ActionHome(c *web.Context) error {
	c.SetTitle(i18n.T(c.Context(), "app", "Home", nil))
	return c.Render("site/home", views.HomeData{
		SignedIn:  c.User() != nil && c.User().Identity() != nil,
		PostsURL:  url.To("post/list", nil),
		LoginURL:  url.To("site/login", nil),
		NotifyURL: url.To("site/notify", nil),
		CSRF:      c.CsrfToken(),
	})
}

// ActionLoginForm renders the login page. The form posts back to /login,
// carrying the CSRF token in the hidden field the middleware verifies.
func (site *SiteController) ActionLoginForm(c *web.Context) error {
	c.SetTitle(i18n.T(c.Context(), "app", "Login", nil))
	return c.Render("site/login", views.LoginData{
		Action: url.To("site/login", nil),
		CSRF:   c.CsrfToken(),
	})
}

// ActionNotify demonstrates the project's custom component: the notifier,
// built in main from the custom email section, composes the message with its
// configured sender and hands it to the application's mailer. The default
// log driver records the complete message in the application log — the
// development counterpart of a provider delivery.
func (site *SiteController) ActionNotify(c *web.Context) error {
	recipient := "guest@example.com"
	if identity := c.User().Identity(); identity != nil {
		recipient = identity.GetName() + "@example.com"
	}
	subject := i18n.T(c.Context(), "app", "A test email from {app}",
		map[string]any{"app": c.App().Name()})
	body := i18n.T(c.Context(), "app", "This message was composed by the notifier component of the basic application.", nil)
	if err := site.Notify.Send(c.Context(), recipient, subject, body); err != nil {
		return err
	}
	c.SetTitle(i18n.T(c.Context(), "app", "Mail sent", nil))
	return c.Render("site/notify", views.NotifyData{
		Recipient: recipient,
		Sender:    site.Notify.Sender(),
		HomeURL:   url.To("site/home", nil),
	})
}
