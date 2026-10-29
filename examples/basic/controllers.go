// Controllers of the basic example. Together they exercise the P0 web
// capabilities: session login/logout, role-based access control, verb
// filtering, CSRF round trips, negotiated error rendering, scenario
// validation on save and a cached list endpoint.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	z "github.com/Oudwins/zog"
	"gorm.io/gorm"

	"github.com/goyii2/framework/base"
	"github.com/goyii2/framework/cache"
	"github.com/goyii2/framework/db/ar"
	"github.com/goyii2/framework/i18n"
	"github.com/goyii2/framework/url"
	"github.com/goyii2/framework/validators"
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

// PostController serves the business CRUD. Its access rules are the
// AccessControl demonstration: reading is public (guest "?" and logged-in
// "@" alike), writing needs a login, deleting asks the RBAC manager for the
// post/delete permission (granted through the admin role's hierarchy, not
// through the identity's own role column), and anything no rule covers is
// denied.
type PostController struct {
	web.Controller

	// Auth is the RBAC manager deciding the concrete role names below.
	Auth web.AccessChecker
}

func (pc *PostController) Middleware() []web.Middleware {
	return []web.Middleware{
		web.AccessControl{Auth: pc.Auth, Rules: []web.AccessRule{
			{Allow: true, Roles: []string{"?", "@"}, Verbs: []string{http.MethodGet, http.MethodHead}},
			{Allow: true, Roles: []string{"@"}, Verbs: []string{http.MethodPost, http.MethodPut}},
			{Allow: true, Roles: []string{"post/delete"}, Verbs: []string{http.MethodDelete}},
		}}.Middleware(),
	}
}

// postListCacheKey is the cache key of the list endpoint.
const postListCacheKey = "post-list"

// ActionIndex lists the newest posts. The list is cached for one minute
// behind a value dependency on the table's max id: creating or deleting a
// post changes the dependency, the entry is judged stale and rebuilt on the
// next read. The X-Cache response header and the cache-category log record
// make the hit/miss outcome observable.
func (pc *PostController) ActionIndex(c *web.Context) error {
	ctx := c.Context()
	dep := cache.NewValueDependency(postMaxID(ctx))

	rebuilt := false
	var posts []Post
	err := c.App().Cache().GetOrSet(ctx, postListCacheKey, &posts,
		func(ctx context.Context) (any, error) {
			rebuilt = true
			rows, err := ar.New[Post](ctx).OrderBy("id DESC").Limit(50).All()
			if err != nil {
				return nil, err
			}
			return rows, nil
		},
		time.Minute, dep)
	if err != nil {
		return err
	}

	state := "HIT"
	if rebuilt {
		state = "MISS"
	}
	c.App().Logger().Log(slog.LevelDebug, "cache", "post list served", "state", state, "dependency", postMaxID(ctx))
	c.Response().Header().Set("X-Cache", state)
	return c.JSON(posts)
}

// ActionView fetches one post with its author eager-loaded (With), the
// Active Record association preload.
func (pc *PostController) ActionView(c *web.Context) error {
	id, err := c.ParamInt("id")
	if err != nil {
		return web.NotFound("post not found")
	}
	post, err := ar.New[Post](c.Context()).With("Author").Where("id = ?", id).One()
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return web.NotFound("post not found")
	}
	if err != nil {
		return err
	}
	return c.JSON(post)
}

// postPayload is the request body of the write actions.
type postPayload struct {
	Title  string `json:"title"`
	Body   string `json:"body"`
	Status string `json:"status"`
}

// ActionCreate inserts a post. Save runs the insert scenario first: a body
// the rules reject never reaches the database and comes back as 422 with the
// field -> messages map.
func (pc *PostController) ActionCreate(c *web.Context) error {
	var payload postPayload
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
		return web.BadRequest("malformed JSON body")
	}
	author, ok := c.User().Identity().(*User)
	if !ok || author == nil {
		return web.Unauthorized("authentication required")
	}
	post := ar.New[Post](c.Context()).New()
	post.Title, post.Body, post.Status = payload.Title, payload.Body, payload.Status
	post.AuthorID = author.ID
	if err := post.Save(); err != nil {
		var verrs *validators.Errors
		if errors.As(err, &verrs) {
			return c.JSONStatus(http.StatusUnprocessableEntity, verrs.Fields)
		}
		return err
	}
	// Reverse URL generation: the named route fills the path parameter.
	c.Response().Header().Set("Location", url.To("post/view", url.P{"id": strconv.FormatInt(post.ID, 10)}))
	return c.JSONStatus(http.StatusCreated, post)
}

// ActionUpdate rewrites a post. The update scenario validates the payload;
// a row that vanished between read and write answers 404.
func (pc *PostController) ActionUpdate(c *web.Context) error {
	id, err := c.ParamInt("id")
	if err != nil {
		return web.NotFound("post not found")
	}
	post, err := ar.FindByID[Post](c.Context(), id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return web.NotFound("post not found")
	}
	if err != nil {
		return err
	}
	var payload postPayload
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
		return web.BadRequest("malformed JSON body")
	}
	post.Title, post.Body, post.Status = payload.Title, payload.Body, payload.Status
	if err := post.Save(); err != nil {
		var verrs *validators.Errors
		if errors.As(err, &verrs) {
			return c.JSONStatus(http.StatusUnprocessableEntity, verrs.Fields)
		}
		return err
	}
	return c.JSON(post)
}

// ActionDelete removes a post. Only holders of the post/delete permission
// get here — the access rules already rejected everyone else with 403, and
// the seed grants it through the admin role's hierarchy.
func (pc *PostController) ActionDelete(c *web.Context) error {
	id, err := c.ParamInt("id")
	if err != nil {
		return web.NotFound("post not found")
	}
	post, err := ar.FindByID[Post](c.Context(), id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return web.NotFound("post not found")
	}
	if err != nil {
		return err
	}
	if err := post.Delete(); err != nil {
		return err
	}
	c.Response().WriteHeader(http.StatusNoContent)
	return nil
}

// postMaxID reports the highest post id, the value the list cache depends
// on. A production table would use a version counter or max(updated_at)
// instead — anything that moves when the data does.
func postMaxID(ctx context.Context) int64 {
	conn, err := base.FromContext(ctx).DB()
	if err != nil {
		return -1
	}
	var maxID int64
	if err := conn.Raw("SELECT COALESCE(MAX(id), 0) FROM post").Scan(&maxID).Error; err != nil {
		return -1
	}
	return maxID
}
