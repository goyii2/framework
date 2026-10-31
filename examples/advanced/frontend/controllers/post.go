// PostController carries the frontend's public content pages: the listing
// of published posts and one post's detail page. Anonymous reach is the
// point — no guard runs here, and drafts stay invisible by the status
// filter, so unpublished content never leaks past the backend.
package controllers

import (
	"errors"
	"strconv"
	"strings"

	"gorm.io/gorm"

	"github.com/goyii2/framework/db/ar"
	"github.com/goyii2/framework/examples/advanced/common/models"
	"github.com/goyii2/framework/examples/advanced/frontend/views"
	"github.com/goyii2/framework/url"
	"github.com/goyii2/framework/web"
)

// PostController is the frontend's post controller: the public read side of
// the shared content record.
type PostController struct {
	web.Controller
}

// excerptLen is where the listing's preview cuts the body.
const excerptLen = 160

// ActionIndex lists the published posts newest first, each card linking to
// the detail page by route name.
func (pc *PostController) ActionIndex(c *web.Context) error {
	posts, err := ar.New[models.Post](c.Context()).
		Where("status = ?", models.PostStatusPublished).
		OrderBy("id DESC").All()
	if err != nil {
		return err
	}
	cards := make([]views.PostCard, 0, len(posts))
	for _, post := range posts {
		cards = append(cards, views.PostCard{
			Title:   post.Title,
			Excerpt: excerpt(post.Body),
			Author:  post.AuthorName(),
			Created: post.CreatedAt.Format("2006-01-02"),
			ViewURL: url.To("post/view", url.P{"id": strconv.FormatInt(post.ID, 10)}),
		})
	}
	c.SetTitle("Posts")
	return c.Render("post/index", views.PostListData{Posts: cards})
}

// ActionView renders one published post: title, byline and body, the
// permalink (the partition's one absolute link, prefixed with the configured
// host info) and the back link to the listing. Drafts and unknown ids answer
// the unified 404 — the public site resolves published rows only.
func (pc *PostController) ActionView(c *web.Context) error {
	id, err := c.ParamInt64("id")
	if err != nil {
		return web.NotFound("post not found")
	}
	post, err := ar.New[models.Post](c.Context()).With("Author").
		Where("id = ? AND status = ?", id, models.PostStatusPublished).One()
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return web.NotFound("post not found")
	}
	if err != nil {
		return err
	}
	c.SetTitle(post.Title)
	return c.Render("post/view", views.PostShowData{
		Title:        post.Title,
		Body:         post.Body,
		Author:       post.AuthorName(),
		Created:      post.CreatedAt.Format("2006-01-02 15:04"),
		PermalinkURL: url.Build("post/view", url.P{"id": strconv.FormatInt(post.ID, 10)}).Absolute(),
		BackURL:      url.To("post/index", nil),
	})
}

// excerpt shortens body to at most excerptLen runes at a word boundary,
// appended with an ellipsis when anything was cut.
func excerpt(body string) string {
	runes := []rune(body)
	if len(runes) <= excerptLen {
		return body
	}
	cut := string(runes[:excerptLen])
	if i := strings.LastIndexAny(cut, " \t\n"); i > excerptLen/2 {
		cut = cut[:i]
	}
	return strings.TrimSpace(cut) + "…"
}
