// PostController carries the backend's content management: the admin-facing
// CRUD over the shared Post record — the listing with every post (drafts
// included, the backend sees the whole table), the compose and edit forms,
// and deletion. The module-wide guard already filtered the audience
// (signed-in admins only), so the actions manage records and never re-check
// access; links and redirects resolve by route name, including the
// cross-partition "view on site" link into the frontend's page.
package controllers

import (
	"errors"
	"net/http"
	"strconv"

	"gorm.io/gorm"

	"github.com/goyii2/framework/db/ar"
	"github.com/goyii2/framework/examples/advanced/backend/views"
	"github.com/goyii2/framework/examples/advanced/common/models"
	"github.com/goyii2/framework/url"
	"github.com/goyii2/framework/validators"
	"github.com/goyii2/framework/view"
	"github.com/goyii2/framework/web"
)

// PostController is the backend's post management controller.
type PostController struct {
	web.Controller
}

// ActionIndex is the management listing: every post newest first with each
// row's actions, rendered in the module's dark frame.
func (pc *PostController) ActionIndex(c *web.Context) error {
	posts, err := ar.New[models.Post](c.Context()).With("Author").OrderBy("id DESC").All()
	if err != nil {
		return err
	}
	rows := make([]views.PostRow, 0, len(posts))
	for _, post := range posts {
		id := strconv.FormatInt(post.ID, 10)
		row := views.PostRow{
			ID:        id,
			Title:     post.Title,
			Status:    post.Status,
			Author:    post.AuthorName(),
			Updated:   post.UpdatedAt.Format("2006-01-02 15:04"),
			EditURL:   url.To("backend/post/edit", url.P{"id": id}),
			DeleteURL: url.To("backend/post/delete", url.P{"id": id}),
		}
		// The frontend publishes finished posts only, so the "view on site"
		// link points somewhere just for them — a draft's link would land on
		// the public 404.
		if post.Status == models.PostStatusPublished {
			row.ViewURL = url.To("post/view", url.P{"id": id})
		}
		rows = append(rows, row)
	}
	c.SetTitle("Posts")
	return c.Render("backend/post/index", views.PostIndexData{
		Rows:      rows,
		CreateURL: url.To("backend/post/new", nil),
		CSRF:      c.CsrfToken(),
	})
}

// findPost loads the post addressed by the id path parameter with its author
// eager-loaded; a missing row answers the unified 404.
func (pc *PostController) findPost(c *web.Context) (*models.Post, error) {
	id, err := c.ParamInt64("id")
	if err != nil {
		return nil, web.NotFound("post not found")
	}
	post, err := ar.New[models.Post](c.Context()).With("Author").Where("id = ?", id).One()
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, web.NotFound("post not found")
	}
	if err != nil {
		return nil, err
	}
	return post, nil
}

// ActionNew renders the compose form with a fresh draft.
func (pc *PostController) ActionNew(c *web.Context) error {
	return renderPostForm(c, false, http.StatusOK, postFormInput{status: models.PostStatusDraft}, nil)
}

// ActionStore receives the compose form. Save runs the insert scenario; a
// payload the rules reject never reaches the database — the form re-renders
// under 422 with the messages below the failing fields and the entered
// values refilled.
func (pc *PostController) ActionStore(c *web.Context) error {
	author, ok := c.User().Identity().(*models.User)
	if !ok || author == nil {
		return web.Unauthorized("authentication required")
	}
	input := readPostForm(c)
	post := ar.New[models.Post](c.Context()).New()
	post.Title, post.Body, post.Status = input.title, input.body, input.status
	post.AuthorID = author.ID
	if err := post.Save(); err != nil {
		return replayPostForm(c, false, input, err)
	}
	c.App().Log().Info("post created", "id", post.ID, "title", post.Title)
	return c.Redirect(http.StatusFound, url.To("backend/post/index", nil))
}

// ActionEdit renders the update form with the record's current values.
func (pc *PostController) ActionEdit(c *web.Context) error {
	post, err := pc.findPost(c)
	if err != nil {
		return err
	}
	input := postFormInput{title: post.Title, body: post.Body, status: post.Status}
	return renderPostForm(c, true, http.StatusOK, input, nil)
}

// ActionSave receives the update form; like the compose form it re-renders
// with 422 and the entered values when the update scenario rejects the
// payload.
func (pc *PostController) ActionSave(c *web.Context) error {
	post, err := pc.findPost(c)
	if err != nil {
		return err
	}
	input := readPostForm(c)
	post.Title, post.Body, post.Status = input.title, input.body, input.status
	if err := post.Save(); err != nil {
		return replayPostForm(c, true, input, err)
	}
	c.App().Log().Info("post updated", "id", post.ID, "title", post.Title)
	return c.Redirect(http.StatusFound, url.To("backend/post/index", nil))
}

// ActionDestroy receives a row's delete button; the module guard already
// made sure only admins reach it.
func (pc *PostController) ActionDestroy(c *web.Context) error {
	post, err := pc.findPost(c)
	if err != nil {
		return err
	}
	if err := post.Delete(); err != nil {
		return err
	}
	c.App().Log().Info("post deleted", "id", post.ID, "title", post.Title)
	return c.Redirect(http.StatusFound, url.To("backend/post/index", nil))
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

// renderPostForm renders the shared form for compose and edit through the
// ActiveForm widget: labels, controls and — on a 422 — the validator
// messages under the failing fields with the submitted values refilled.
// Editing picks the heading and targets the row's save route instead of the
// create route; status is 200 for a fresh form and 422 when a rejected
// submission re-renders.
func renderPostForm(c *web.Context, editing bool, status int, input postFormInput, verrs *validators.Errors) error {
	heading := "New post"
	action := url.To("backend/post/store", nil)
	if editing {
		heading = "Edit post"
		action = url.To("backend/post/save", url.P{"id": c.Param("id")})
	}
	form := view.ActiveForm{
		ID:     "post-form",
		Action: action,
		Fields: []view.ActiveField{
			{Name: "title", Label: "Title", Type: view.FieldText, Required: true, MaxLength: 200},
			{Name: "status", Label: "Status", Type: view.FieldSelect, Options: []view.FieldOption{
				{Value: models.PostStatusDraft, Label: models.PostStatusDraft},
				{Value: models.PostStatusPublished, Label: models.PostStatusPublished},
			}},
			{Name: "body", Label: "Body", Type: view.FieldTextarea, MaxLength: 10000},
		},
		Errors: verrs,
		Values: map[string]string{
			"title":  input.title,
			"status": input.status,
			"body":   input.body,
		},
		CSRF: c.CsrfToken(),
	}
	c.SetTitle(heading)
	return c.RenderStatus(status, "backend/post/form", views.PostFormData{
		Heading:   heading,
		Form:      form.Render(),
		CancelURL: url.To("backend/post/index", nil),
	})
}

// replayPostForm re-renders the form after a rejected save: validation
// failures come back as the 422 error set, anything else is a real error.
func replayPostForm(c *web.Context, editing bool, input postFormInput, err error) error {
	var verrs *validators.Errors
	if !errors.As(err, &verrs) {
		return err
	}
	return renderPostForm(c, editing, http.StatusUnprocessableEntity, input, verrs)
}
