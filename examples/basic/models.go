// Models of the basic example: the account record (which doubles as the
// session identity) and the business record validated per save scenario.
package main

import (
	"strconv"

	z "github.com/Oudwins/zog"
	"golang.org/x/crypto/bcrypt"

	"github.com/goyii2/framework/behaviors"
	"github.com/goyii2/framework/db/ar"
	"github.com/goyii2/framework/validators"
)

// User is the account record: login name, bcrypt password hash and the role
// driving access control. It implements the web.Identity contract, so an
// instance can be bound to the session as the current user directly.
type User struct {
	ar.Model
	ID           int64  `gorm:"column:id;primaryKey" json:"id"`
	Username     string `gorm:"column:username" json:"username"`
	PasswordHash string `gorm:"column:password_hash" json:"-"` // never leaves the model as JSON
	Role         string `gorm:"column:role" json:"role"`
}

func (User) TableName() string { return "user" }

// GetID, GetName and GetRoles fulfill web.Identity. The ID is the value the
// session stores on login and the IdentityFinder resolves back.
func (u *User) GetID() string      { return strconv.FormatInt(u.ID, 10) }
func (u *User) GetName() string    { return u.Username }
func (u *User) GetRoles() []string { return []string{u.Role} }

// Rules validates accounts on insert: username present, 3+ characters and
// unique in the user table. The uniqueness rule queries the database through
// the record's bound context — the Active Record way of expressing a
// UniqueValidator. Updates carry no schema, so renames pass (a production
// application would scope the same query by id here).
func (u *User) Rules() validators.RuleSet {
	return validators.RuleSet{
		validators.ScenarioInsert: z.Struct(z.Shape{
			"username": z.String().
				Required(z.Message("username cannot be blank")).
				Min(3, z.Message("username must be at least 3 characters")).
				TestFunc(func(val *string, _ z.Ctx) bool {
					n, err := ar.New[User](u.Context()).Where("username = ?", *val).Count()
					return err == nil && n == 0
				}, z.Message("username is already taken")),
			"role": z.String().Required(z.Message("role cannot be blank")),
		}),
	}
}

// SetPassword stores the bcrypt hash of plaintext.
func (u *User) SetPassword(plaintext string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(plaintext), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	u.PasswordHash = string(hash)
	return nil
}

// CheckPassword reports whether plaintext matches the stored hash; bcrypt's
// comparison does not leak mismatch positions.
func (u *User) CheckPassword(plaintext string) bool {
	return bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(plaintext)) == nil
}

// Post is the demo's business record. AuthorID is the database-level
// reference; the Author association is eager-loaded with With("Author").
// The embedded Hooks dispatcher fans GORM lifecycle events out to the
// Timestamp behavior, which stamps created_at / updated_at on save — the
// behaviors a Yii2 model would attach, expressed as struct embedding.
type Post struct {
	ar.Model
	ID       int64  `gorm:"column:id;primaryKey" json:"id"`
	Title    string `gorm:"column:title" json:"title"`
	Body     string `gorm:"column:body" json:"body"`
	Status   string `gorm:"column:status" json:"status"`
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
