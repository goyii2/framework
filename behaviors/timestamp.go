package behaviors

import (
	"reflect"
	"time"

	"gorm.io/gorm"
)

// Timestamp fills the creation and modification times of the record that
// embeds it: both columns on insert, the updated column again on every
// update — the counterpart of Yii2's TimestampBehavior with time.Time
// columns. Values are always overwritten, exactly like the Yii2 default;
// importing rows with preserved historic times means populating the fields
// after save, outside the hook.
//
// The columns follow the field mapping (created_at / updated_at by
// default). Models whose tables name the columns differently declare their
// own fields and fill them in their own hooks; the two fill variants here
// cover the standard mappings.
type Timestamp struct {
	CreatedAt time.Time `gorm:"column:created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

// onCreate stamps both the creation and the modification time for the
// insert. Times are normalized to UTC so a database round-trip does not
// shift them by the server's local zone.
func (t *Timestamp) onCreate(model reflect.Value, tx *gorm.DB) error {
	now := time.Now().UTC()
	t.CreatedAt = now
	t.UpdatedAt = now
	return nil
}

// onUpdate stamps the modification time before the update statement runs.
func (t *Timestamp) onUpdate(model reflect.Value, tx *gorm.DB) error {
	t.UpdatedAt = time.Now().UTC()
	return nil
}

// TimestampUnix is the integer shape of the same behavior: creation and
// modification times as Unix seconds, the counterpart of Yii2 applications
// storing time() values. Both columns fill on insert, the updated column
// again on every update.
type TimestampUnix struct {
	CreatedAt int64 `gorm:"column:created_at"`
	UpdatedAt int64 `gorm:"column:updated_at"`
}

// onCreate stamps both columns with the current Unix time.
func (t *TimestampUnix) onCreate(model reflect.Value, tx *gorm.DB) error {
	now := time.Now().Unix()
	t.CreatedAt = now
	t.UpdatedAt = now
	return nil
}

// onUpdate stamps the updated column with the current Unix time.
func (t *TimestampUnix) onUpdate(model reflect.Value, tx *gorm.DB) error {
	t.UpdatedAt = time.Now().Unix()
	return nil
}
