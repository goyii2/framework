// Package migrations holds the schema migrations of the common partition —
// the tables every application of the advanced template shares. All() is
// the list handed to the goyii migrate command; versions apply in the
// order listed here, and later slices append their own (RBAC, posts).
package migrations

import (
	"github.com/goyii2/framework/migrate"
)

// All returns every migration in apply order.
func All() []migrate.Migration {
	return []migrate.Migration{
		m20260101_000001_create_user_table{},
	}
}
