// Package models holds the Active Record models of the common partition —
// the records every application of the advanced template shares, the
// counterpart of the Yii2 template's common/models. The user model joins the
// user table the common migrations create; the post model follows in its own
// slice.
package models

import (
	"context"
	"strconv"

	z "github.com/Oudwins/zog"
	"golang.org/x/crypto/bcrypt"

	"github.com/goyii2/framework/base"
	"github.com/goyii2/framework/db/ar"
	"github.com/goyii2/framework/validators"
	"github.com/goyii2/framework/web"
)

// User is the shared account record: login name, bcrypt password hash and
// the role driving access control (the backend module's admin guard reads
// it). It implements the web.Identity contract, so an instance binds to the
// session as the current user directly.
type User struct {
	ar.Model
	ID           int64  `gorm:"column:id;primaryKey" json:"id"`
	Username     string `gorm:"column:username" json:"username"`
	PasswordHash string `gorm:"column:password_hash" json:"-"` // never leaves the model as JSON
	Role         string `gorm:"column:role" json:"role"`
}

func (User) TableName() string { return "user" }

// GetID, GetName and GetRoles fulfill web.Identity. The ID is the value the
// session stores on login and FindIdentity resolves back.
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

// FindIdentity resolves a stored session identity id back to the account row
// — the bridge between the session layer and the user table that main.go
// installs as the handler's IdentityFinder. An id that no longer matches an
// account (a since-deleted user) degrades to a guest.
func FindIdentity(app *base.Application) func(string) (web.Identity, bool) {
	return func(id string) (web.Identity, bool) {
		uid, err := strconv.ParseInt(id, 10, 64)
		if err != nil {
			return nil, false
		}
		user, err := ar.FindByID[User](app.WithContext(context.Background()), uid)
		if err != nil {
			return nil, false
		}
		return user, true
	}
}
