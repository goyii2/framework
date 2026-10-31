// The business record of the basic example: the demo's Post, validated per
// save scenario and stamped with timestamps by the embedded behavior.
package models

import (
	z "github.com/Oudwins/zog"

	"github.com/goyii2/framework/behaviors"
	"github.com/goyii2/framework/db/ar"
	"github.com/goyii2/framework/validators"
)

// Post is the demo's business record. AuthorID is the database-level
// reference; the Author association is eager-loaded with With("Author").
// The embedded Hooks dispatcher fans GORM lifecycle events out to the
// Timestamp behavior, which stamps created_at / updated_at on save — the
// behaviors a Yii2 model would attach, expressed as struct embedding.
// The `form` tags on the editable columns are the binding whitelist: the
// write actions bind request values straight into the record, so the
// primary key, the author reference and the association stay writable only
// by server-side code.
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

// PostStatusDraft and PostStatusPublished are the workflow statuses a post's
// Status field accepts.
const (
	PostStatusDraft     = "draft"
	PostStatusPublished = "published"
)

// Rules validates posts per save scenario. Insert demands the full record
// (title, status, author); update re-checks title and status while the body
// stays free-form — one model, two rule sets, chosen automatically by Save.
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
