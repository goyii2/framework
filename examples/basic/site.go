// SiteController: the entry points around authentication (session
// login/logout, the CSRF round trip, the identity echo) plus the browser
// front pages — home, the login form and the mailer demonstration.
package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	z "github.com/Oudwins/zog"

	"github.com/goyii2/framework/db/ar"
	"github.com/goyii2/framework/i18n"
	"github.com/goyii2/framework/mail"
	"github.com/goyii2/framework/url"
	"github.com/goyii2/framework/web"
)

// SiteController carries the entry points around authentication. Its one
// piece of middleware scopes the convention-resolved /site/index to safe
// verbs — the VerbFilter demonstration: POST /site/index answers 405 with an
// Allow header instead of running the action.
type SiteController struct {
	web.Controller
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
		"app": c.App().Config().App.Name,
		"env": c.App().Config().Env,
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

// loginPayload is the request body of ActionLogin, validated inline with a
// zog schema before any database lookup happens.
type loginPayload struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

var loginSchema = z.Struct(z.Shape{
	"username": z.String().Required(z.Message("username cannot be blank")),
	"password": z.String().Required(z.Message("password cannot be blank")),
})

// ActionLogin authenticates against the user table and binds the account to
// the session. Unknown users and wrong passwords answer the same 401 — the
// response must not reveal which half was wrong. The action serves both
// shapes of caller: JSON API clients get validated JSON replies, the
// browser login form is re-rendered with its errors under the failing
// status and redirected to the post list on success.
func (site *SiteController) ActionLogin(c *web.Context) error {
	var payload loginPayload
	if strings.HasPrefix(c.Request().Header.Get("Content-Type"), "application/json") {
		if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
			return web.BadRequest("malformed JSON body")
		}
	} else {
		payload.Username = c.Request().PostFormValue("username")
		payload.Password = c.Request().PostFormValue("password")
	}

	html := c.ResponseFormat() == "html"
	if issues := loginSchema.Validate(&payload); len(issues) > 0 {
		fields := translateIssues(c.Context(), z.Issues.Flatten(issues))
		if html {
			c.SetTitle(i18n.T(c.Context(), "app", "Login", nil))
			return c.RenderStatus(http.StatusUnprocessableEntity, "site/login", loginData{
				Action:   url.To("site/login", nil),
				CSRF:     c.CsrfToken(),
				Username: payload.Username,
				Errors:   fields,
			})
		}
		return c.JSONStatus(http.StatusUnprocessableEntity, z.Issues.Flatten(issues))
	}
	user, err := ar.New[User](c.Context()).Where("username = ?", payload.Username).One()
	if err != nil || !user.CheckPassword(payload.Password) {
		if html {
			c.SetTitle(i18n.T(c.Context(), "app", "Login", nil))
			return c.RenderStatus(http.StatusUnauthorized, "site/login", loginData{
				Action:   url.To("site/login", nil),
				CSRF:     c.CsrfToken(),
				Username: payload.Username,
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

// translateIssues maps a field -> messages set through the message source,
// so messages an inline zog schema produced localize the same way model
// rules do (the automatic translation sits in the model save pipeline).
func translateIssues(ctx context.Context, fields map[string][]string) map[string][]string {
	out := make(map[string][]string, len(fields))
	for field, msgs := range fields {
		translated := make([]string, len(msgs))
		for i, msg := range msgs {
			translated[i] = i18n.T(ctx, "app", msg, nil)
		}
		out[field] = translated
	}
	return out
}

// ActionHome is the browser front page: an introduction, the entry links
// and the mailer demonstration button.
func (site *SiteController) ActionHome(c *web.Context) error {
	c.SetTitle(i18n.T(c.Context(), "app", "Home", nil))
	return c.Render("site/home", homeData{
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
	return c.Render("site/login", loginData{
		Action: url.To("site/login", nil),
		CSRF:   c.CsrfToken(),
	})
}

// ActionNotify demonstrates the mail component: it composes a message with
// the chain builder and hands it to the application's mailer. The default
// log driver records the complete message in the application log — the
// development counterpart of an SMTP delivery.
func (site *SiteController) ActionNotify(c *web.Context) error {
	recipient := "guest@example.com"
	if identity := c.User().Identity(); identity != nil {
		recipient = identity.GetName() + "@example.com"
	}
	subject := i18n.T(c.Context(), "app", "A test email from {app}",
		map[string]any{"app": c.App().Config().App.Name})
	body := i18n.T(c.Context(), "app", "This message was composed by the mail example of the basic application.", nil)
	msg := mail.NewMessage().
		SetFrom("goyii basic <no-reply@goyii.example>").
		SetTo(recipient).
		SetSubject(subject).
		SetTextBody(body)
	if err := c.App().Mailer().Send(c.Context(), msg); err != nil {
		return err
	}
	c.SetTitle(i18n.T(c.Context(), "app", "Mail sent", nil))
	return c.Render("site/notify", notifyData{Recipient: recipient, HomeURL: url.To("site/home", nil)})
}
