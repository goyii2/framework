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
	"github.com/goyii2/framework/form"
	"github.com/goyii2/framework/url"
	"github.com/goyii2/framework/validators"
	"github.com/goyii2/framework/view"
	"github.com/goyii2/framework/web"
)

// PostController is the backend's post management controller. BackendHost is
// the partition's own scheme-and-host prefix, handed in by wiring (it comes
// from the project's configuration): the edit page's permalink builds its
// absolute URL on it explicitly — the deployment shape where the admin face
// runs on a domain of its own while the url manager's host info names the
// public site.
type PostController struct {
	web.Controller

	// BackendHost is the partition's scheme-and-host prefix
	// ("scheme://host[:port]"); empty means the permalink degrades to the
	// "#" cannot-create marker rather than guessing a domain.
	BackendHost string
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

// ActionNew renders the compose form with a fresh draft; the refill values
// come from a zero record, so the status select defaults to draft. No row
// exists yet, so the page carries no permalink.
func (pc *PostController) ActionNew(c *web.Context) error {
	post := ar.New[models.Post](c.Context()).New()
	post.Status = models.PostStatusDraft
	return pc.renderPostForm(c, false, http.StatusOK, form.Values(post), nil)
}

// ActionStore receives the compose form. The field values bind straight
// into the zero record — the model's form tags are the whitelist, so a form
// field the record did not declare (an id, an author reference) assigns
// nothing — and LoadForm runs the insert scenario before any write: a
// payload the rules reject never reaches the database, the form re-renders
// under 422 with the messages below the failing fields and the entered
// values refilled. The author reference is server-side state, set after the
// binding so a submission can never carry it.
func (pc *PostController) ActionStore(c *web.Context) error {
	author, ok := c.User().Identity().(*models.User)
	if !ok || author == nil {
		return web.Unauthorized("authentication required")
	}
	post := ar.New[models.Post](c.Context()).New()
	if errs := c.LoadForm(post, validators.ScenarioInsert); errs != nil {
		return pc.replayPostForm(c, false, form.Values(post), errs)
	}
	post.AuthorID = author.ID
	if err := post.Save(); err != nil {
		return pc.replayPostForm(c, false, form.Values(post), err)
	}
	c.App().Log().Info("post created", "id", post.ID, "title", post.Title)
	return c.Redirect(http.StatusFound, url.To("backend/post/index", nil))
}

// ActionEdit renders the update form with the record's current values. The
// page also carries the row's permalink — the partition's one absolute link,
// built on the backend's own domain.
func (pc *PostController) ActionEdit(c *web.Context) error {
	post, err := pc.findPost(c)
	if err != nil {
		return err
	}
	return pc.renderPostForm(c, true, http.StatusOK, form.Values(post), nil)
}

// ActionSave receives the update form; the record is fetched first and the
// submission overwrites it under the update scenario, so fields the form
// left out keep their stored values instead of being blanked. A rejected
// payload re-renders with 422 and the entered values.
func (pc *PostController) ActionSave(c *web.Context) error {
	post, err := pc.findPost(c)
	if err != nil {
		return err
	}
	if errs := c.LoadForm(post, validators.ScenarioUpdate); errs != nil {
		return pc.replayPostForm(c, true, form.Values(post), errs)
	}
	if err := post.Save(); err != nil {
		return pc.replayPostForm(c, true, form.Values(post), err)
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

// renderPostForm renders the shared form for compose and edit through the
// ActiveForm widget: labels, controls and — on a 422 — the validator
// messages under the failing fields with the submitted values refilled.
// Editing picks the heading, targets the row's save route instead of the
// create route, and carries the row's permalink — the absolute URL built on
// the backend's own domain (the explicit host the controller's wiring
// injected) rather than the route table's default host info, so the link
// stays correct in the deployment shape where the admin face runs on a
// domain of its own. A malformed host degrades the permalink to the "#"
// cannot-create marker, never to a half URL. status is 200 for a fresh form
// and 422 when a rejected submission re-renders. values is the refill map
// the ActiveForm echoes back — form.Values over the record the action holds,
// whether a prefilled row or a rejected submission.
func (pc *PostController) renderPostForm(c *web.Context, editing bool, status int, values map[string]string, verrs *validators.Errors) error {
	heading := "New post"
	action := url.To("backend/post/store", nil)
	permalink := ""
	if editing {
		heading = "Edit post"
		action = url.To("backend/post/save", url.P{"id": c.Param("id")})
		permalink = url.Build("backend/post/edit", url.P{"id": c.Param("id")}).Host(pc.BackendHost).Absolute()
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
		Values: values,
		CSRF:   c.CsrfToken(),
	}
	c.SetTitle(heading)
	return c.RenderStatus(status, "backend/post/form", views.PostFormData{
		Heading:      heading,
		Form:         form.Render(),
		PermalinkURL: permalink,
		CancelURL:    url.To("backend/post/index", nil),
	})
}

// replayPostForm re-renders the form after a rejected save: validation
// failures come back as the 422 error set, anything else is a real error.
func (pc *PostController) replayPostForm(c *web.Context, editing bool, values map[string]string, err error) error {
	var verrs *validators.Errors
	if !errors.As(err, &verrs) {
		return err
	}
	return pc.renderPostForm(c, editing, http.StatusUnprocessableEntity, values, verrs)
}
