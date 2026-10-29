package behaviors

import (
	"reflect"
	"strconv"

	"gorm.io/gorm"

	"github.com/goyii2/framework/web"
)

// Blameable records who created the row and who last changed it, resolved
// from the current identity at save time — the counterpart of Yii2's
// BlameableBehavior. The identifier is the numeric identity id (the usual
// user-table primary key shape); identities with non-numeric ids, guests
// and contexts without any identity notion (a CLI context without
// web.WithIdentity) leave the columns at 0, and an update as a guest sets
// updated_by back to 0 rather than preserving the earlier attribution —
// the fill is unconditional, matching the Yii2 default of writing whatever
// the current user resolves to.
//
// The identity comes from web.IdentityFromContext on the context the save
// runs with: inside a request that is the logged-in user (session or a
// stateless authenticator's transient identity), elsewhere whatever
// web.WithIdentity published for the job at hand.
type Blameable struct {
	CreatedBy int64 `gorm:"column:created_by"`
	UpdatedBy int64 `gorm:"column:updated_by"`
}

// onCreate stamps both columns with the current identity id.
func (b *Blameable) onCreate(model reflect.Value, tx *gorm.DB) error {
	who := CurrentUserID(tx)
	b.CreatedBy = who
	b.UpdatedBy = who
	return nil
}

// onUpdate stamps the updated_by column with the current identity id.
func (b *Blameable) onUpdate(model reflect.Value, tx *gorm.DB) error {
	b.UpdatedBy = CurrentUserID(tx)
	return nil
}

// CurrentUserID resolves the current identity for the statement tx carries
// and returns its numeric id; 0 when the context holds no identity or the
// id does not parse as a number.
func CurrentUserID(tx *gorm.DB) int64 {
	if tx.Statement == nil || tx.Statement.Context == nil {
		return 0
	}
	identity := web.IdentityFromContext(tx.Statement.Context)
	if identity == nil {
		return 0
	}
	id, err := strconv.ParseInt(identity.GetID(), 10, 64)
	if err != nil {
		return 0
	}
	return id
}
