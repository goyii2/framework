// The browser-facing actions: pages built on the templ view layer, the
// data provider, the widgets and the RBAC manager. They live beside the
// JSON API actions of controllers.go and share the same models and access
// rules; the write pages additionally gate on the RBAC permissions the seed
// grants through the role hierarchy (editor writes, admin also deletes).
package main

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"gorm.io/gorm"

	"github.com/goyii2/framework/dataprovider"
	"github.com/goyii2/framework/db/ar"
	"github.com/goyii2/framework/i18n"
	"github.com/goyii2/framework/mail"
	"github.com/goyii2/framework/url"
	"github.com/goyii2/framework/validators"
	"github.com/goyii2/framework/view"
	"github.com/goyii2/framework/web"
)

// sortablePostFields is the whitelist the listing accepts sort requests
// for. A request naming anything else answers 400 before any SQL is built.
var sortablePostFields = []string{"id", "title", "status", "created_at"}

// listPerPage is the page size of the browser listing.
const listPerPage = 10

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

// requirePermission gates a page action on an RBAC item: guests are sent to
// the login page (the redirect answered the request, so the returned error
// is nil), signed-in requesters without the item get the 403 error page.
// Actions return the result directly — an action's own return value is what
// the pipeline renders.
func requirePermission(c *web.Context, item string) error {
	if c.User() == nil || c.User().IsGuest() {
		return c.Redirect(http.StatusFound, url.To("site/login", nil))
	}
	if !c.User().Can(item) {
		return web.Forbidden("you are not allowed to perform this action")
	}
	return nil
}

// can reports whether the current user holds an RBAC item (false for
// guests); for menu and button visibility.
func can(c *web.Context, item string) bool {
	return c.User() != nil && c.User().Can(item)
}

// postID formats a row id for URL parameters.
func postID(post *Post) string { return strconv.FormatInt(post.ID, 10) }

// ActionList is the browser listing: an ActiveDataProvider pushes the
// pagination and the whitelisted sort down to SQL, the LinkPager widget
// renders the paging bar and every column heading links its sort toggle.
func (pc *PostController) ActionList(c *web.Context) error {
	params, err := dataprovider.FromRequest(c.Request(), sortablePostFields...)
	if err != nil {
		return web.BadRequest(err.Error())
	}
	if c.Request().URL.Query().Get("per-page") == "" {
		// The page fixes its own page size; an explicit per-page from the
		// query string still wins (the parser already clamped it).
		params.PerPage = listPerPage
	}
	res, err := dataprovider.ActiveDataProvider[Post]{
		Query:        ar.New[Post](c.Context()).With("Author"),
		Params:       params,
		DefaultOrder: []dataprovider.Sort{{Column: "id", Desc: true}},
	}.Prepare()
	if err != nil {
		return err
	}

	// The pager keeps the active sort across pages; the numbered buttons
	// address pages, the sort toggles restart from page one.
	sortParam := c.Request().URL.Query().Get("sort")
	pager := view.LinkPager{
		Pagination:     res.Pagination(),
		MaxButtonCount: 5,
		URL: func(page int) string {
			p := url.P{"page": strconv.Itoa(page)}
			if sortParam != "" {
				p["sort"] = sortParam
			}
			return url.To("post/list", p)
		},
	}.Render()

	ctx := c.Context()
	format := i18n.F(ctx)
	canUpdate := can(c, "post/update")
	rows := make([]postRow, 0, len(res.Items))
	for _, post := range res.Items {
		row := postRow{
			ID:        postID(post),
			Title:     post.Title,
			Status:    post.Status,
			CreatedAt: format.DateTime(post.CreatedAt),
			ViewURL:   url.To("post/show", url.P{"id": postID(post)}),
			ShowEdit:  canUpdate,
		}
		if canUpdate {
			row.EditURL = url.To("post/edit", url.P{"id": postID(post)})
		}
		if post.Author != nil {
			row.Author = post.Author.Username
		}
		rows = append(rows, row)
	}

	c.SetTitle(i18n.T(ctx, "app", "Posts", nil))
	return c.Render("post/list", postsListData{
		Rows:        rows,
		Total:       res.TotalCount,
		Pager:       pager,
		SortID:      sortToggle(c, "id", params),
		SortTitle:   sortToggle(c, "title", params),
		SortStatus:  sortToggle(c, "status", params),
		SortCreated: sortToggle(c, "created_at", params),
		CanCreate:   can(c, "post/create"),
		CreateURL:   url.To("post/new", nil),
	})
}

// sortToggle builds one column heading: the link flips the column's
// direction (ascending first, the leading "-" inverts a column already
// sorted ascending) and the arrow marks the active direction.
func sortToggle(c *web.Context, column string, params dataprovider.Params) sortHeader {
	h := sortHeader{URL: url.To("post/list", url.P{"sort": column})}
	for _, s := range params.Sort {
		if s.Column != column {
			continue
		}
		if s.Desc {
			h.Arrow = " ↓"
		} else {
			h.Arrow = " ↑"
			h.URL = url.To("post/list", url.P{"sort": "-" + column})
		}
	}
	return h
}

// findPost loads the post addressed by the id path parameter with its
// author eager-loaded; a missing row answers the negotiated 404.
func (pc *PostController) findPost(c *web.Context) (*Post, error) {
	id, err := c.ParamInt64("id")
	if err != nil {
		return nil, web.NotFound("post not found")
	}
	post, err := ar.New[Post](c.Context()).With("Author").Where("id = ?", id).One()
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, web.NotFound("post not found")
	}
	if err != nil {
		return nil, err
	}
	return post, nil
}

// ActionShow renders the detail page: the DetailView widget over explicit
// rows, the breadcrumbs trail, and the action buttons the RBAC manager
// allows (edit for post/update holders, delete for admin through the
// hierarchy).
func (pc *PostController) ActionShow(c *web.Context) error {
	post, err := pc.findPost(c)
	if err != nil {
		return err
	}
	ctx := c.Context()
	t := func(msgID string) string { return i18n.T(ctx, "app", msgID, nil) }
	format := i18n.F(ctx)
	author := ""
	if post.Author != nil {
		author = post.Author.Username
	}
	detail := view.DetailView{Rows: []view.DetailRow{
		{Label: t("ID"), Value: postID(post)},
		{Label: t("Title"), Value: post.Title},
		{Label: t("Status"), Value: t(post.Status)},
		{Label: t("Author"), Value: author},
		{Label: t("Created at"), Value: format.DateTime(post.CreatedAt)},
		{Label: t("Updated at"), Value: format.DateTime(post.UpdatedAt)},
	}}.Render()

	c.SetTitle(post.Title)
	return c.Render("post/show", postShowData{
		Breadcrumbs: breadcrumbs(ctx, post.Title),
		Detail:      detail,
		Title:       post.Title,
		CanUpdate:   can(c, "post/update"),
		CanDelete:   can(c, "post/delete"),
		EditURL:     url.To("post/edit", url.P{"id": postID(post)}),
		DeleteURL:   url.To("post/destroy", url.P{"id": postID(post)}),
		BackURL:     url.To("post/list", nil),
		CSRF:        c.CsrfToken(),
	})
}

// ActionNew renders the create form; guests go to the login page, accounts
// without post/create see the 403 page.
func (pc *PostController) ActionNew(c *web.Context) error {
	if err := requirePermission(c, "post/create"); err != nil {
		return err
	}
	return renderPostForm(c, false, http.StatusOK, postFormInput{status: PostStatusDraft}, nil)
}

// ActionStore receives the create form. The CSRF middleware verified the
// submission; the save pipeline runs the insert scenario and a rejected
// payload re-renders the form under 422 with the messages and the entered
// values echoed back.
func (pc *PostController) ActionStore(c *web.Context) error {
	if err := requirePermission(c, "post/create"); err != nil {
		return err
	}
	author, ok := c.User().Identity().(*User)
	if !ok || author == nil {
		return web.Unauthorized("authentication required")
	}
	input := readPostForm(c)
	post := ar.New[Post](c.Context()).New()
	post.Title, post.Body, post.Status = input.title, input.body, input.status
	post.AuthorID = author.ID
	if err := post.Save(); err != nil {
		return replayPostForm(c, false, input, err)
	}
	return c.Redirect(http.StatusFound, url.To("post/show", url.P{"id": postID(post)}))
}

// ActionEdit renders the update form with the record's current values.
func (pc *PostController) ActionEdit(c *web.Context) error {
	if err := requirePermission(c, "post/update"); err != nil {
		return err
	}
	post, err := pc.findPost(c)
	if err != nil {
		return err
	}
	return renderPostForm(c, true, http.StatusOK, postFormInput{title: post.Title, body: post.Body, status: post.Status}, nil)
}

// ActionSave receives the update form; like the create form it re-renders
// with 422 and the entered values when the update scenario rejects the
// payload.
func (pc *PostController) ActionSave(c *web.Context) error {
	if err := requirePermission(c, "post/update"); err != nil {
		return err
	}
	post, err := pc.findPost(c)
	if err != nil {
		return err
	}
	input := readPostForm(c)
	post.Title, post.Body, post.Status = input.title, input.body, input.status
	if err := post.Save(); err != nil {
		return replayPostForm(c, true, input, err)
	}
	return c.Redirect(http.StatusFound, url.To("post/show", url.P{"id": postID(post)}))
}

// ActionDestroy receives the delete button of the detail page. Only the
// admin hierarchy reaches it; everyone else sees the 403 page.
func (pc *PostController) ActionDestroy(c *web.Context) error {
	if err := requirePermission(c, "post/delete"); err != nil {
		return err
	}
	post, err := pc.findPost(c)
	if err != nil {
		return err
	}
	if err := post.Delete(); err != nil {
		return err
	}
	return c.Redirect(http.StatusFound, url.To("post/list", nil))
}

// postFormInput carries the form field values between reading, saving and
// re-rendering.
type postFormInput struct {
	title  string
	body   string
	status string
}

// readPostForm reads the submitted form fields.
func readPostForm(c *web.Context) postFormInput {
	r := c.Request()
	return postFormInput{
		title:  r.PostFormValue("title"),
		body:   r.PostFormValue("body"),
		status: r.PostFormValue("status"),
	}
}

// renderPostForm renders the shared form template for create and edit;
// editing picks the heading and targets the row's save route instead of the
// create route. status is 200 for a fresh form and 422 when a rejected
// submission re-renders with its messages.
func renderPostForm(c *web.Context, editing bool, status int, input postFormInput, fields map[string][]string) error {
	ctx := c.Context()
	heading := i18n.T(ctx, "app", "New post", nil)
	action := url.To("post/store", nil)
	if editing {
		heading = i18n.T(ctx, "app", "Edit post", nil)
		action = url.To("post/save", url.P{"id": c.Param("id")})
	}
	c.SetTitle(heading)
	return c.RenderStatus(status, "post/form", postFormData{
		Breadcrumbs: breadcrumbs(ctx, heading),
		Heading:     heading,
		Action:      action,
		CancelURL:   url.To("post/list", nil),
		CSRF:        c.CsrfToken(),
		Title:       input.title,
		Body:        input.body,
		Status:      input.status,
		Errors:      fields,
	})
}

// replayPostForm re-renders the form after a rejected save: validation
// failures come back as the 422 field map (already translated by the save
// pipeline's flatten boundary), anything else is a real error.
func replayPostForm(c *web.Context, editing bool, input postFormInput, err error) error {
	var verrs *validators.Errors
	if !errors.As(err, &verrs) {
		return err
	}
	return renderPostForm(c, editing, http.StatusUnprocessableEntity, input, verrs.Fields)
}
