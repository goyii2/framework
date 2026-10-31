// The shared content record of the advanced example: the posts the backend
// manages through its CRUD pages and the frontend shows anonymously.
package models

import (
	z "github.com/Oudwins/zog"

	"github.com/goyii2/framework/behaviors"
	"github.com/goyii2/framework/db/ar"
	"github.com/goyii2/framework/validators"
)

// Post is the shared content record. AuthorID is the database-level
// reference to the account that wrote it; the Author association
// eager-loads that row with With("Author"). Status carries the publishing
// workflow — the frontend shows the published posts only, so drafts stay
// backend-visible alone. The `form` tags whitelist what a submitted form or
// JSON body may assign on direct binding: the editable text columns only —
// the primary key, the author reference and the association stay
// server-assigned, and the Timestamp behavior owns created_at / updated_at.
// The embedded Hooks dispatcher fans GORM lifecycle events out to the
// Timestamp behavior, which stamps created_at / updated_at on every save.
type Post struct {
	ar.Model
	ID       int64  `gorm:"column:id;primaryKey" json:"id"`
	Title    string `gorm:"column:title" form:"title" json:"title"`
	Body     string `gorm:"column:body" form:"body" json:"body"`
	Status   string `gorm:"column:status" form:"status" json:"status"`
	AuthorID int64  `gorm:"column:author_id" json:"author_id"`
	Author   *User  `gorm:"foreignKey:AuthorID" json:"author,omitempty"`

	behaviors.Hooks
	behaviors.Timestamp
}

func (Post) TableName() string { return "post" }

// AuthorName reports the author's login name, blank when the account row is
// gone — the display form of the association both partitions render.
func (p *Post) AuthorName() string {
	if p.Author != nil {
		return p.Author.Username
	}
	return ""
}

// PostStatusDraft and PostStatusPublished are the workflow statuses a post's
// Status field accepts.
const (
	PostStatusDraft     = "draft"
	PostStatusPublished = "published"
)

// Rules validates posts per save scenario, the Active Record way: insert
// demands the full record (title, status, bounded body) while update
// re-checks title and status and leaves the body free-form — one model, two
// rule sets, chosen automatically by Save.
func (p *Post) Rules() validators.RuleSet {
	title := func() *z.StringSchema[string] {
		return z.String().
			Required(z.Message("title cannot be blank")).
			Min(3, z.Message("title must be at least 3 characters")).
			Max(200, z.Message("title must be at most 200 characters"))
	}
	status := func() *z.StringSchema[string] {
		return z.String().Required(z.Message("status cannot be blank")).
			OneOf([]string{PostStatusDraft, PostStatusPublished},
				z.Message("status must be one of: draft, published"))
	}
	return validators.RuleSet{
		validators.ScenarioInsert: z.Struct(z.Shape{
			"title":  title(),
			"status": status(),
			"body":   z.String().Max(10000, z.Message("body must be at most 10000 characters")),
		}),
		validators.ScenarioUpdate: z.Struct(z.Shape{
			"title":  title(),
			"status": status(),
		}),
	}
}
