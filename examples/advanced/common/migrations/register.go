// Package migrations holds the schema migrations of the common partition —
// the tables every application of the advanced template shares. All() is
// the list handed to the goyii migrate command; versions apply in the
// order listed here, and later slices append their own (posts).
package migrations

import (
	"github.com/goyii2/framework/migrate"
	"github.com/goyii2/framework/rbac"
)

// All returns every migration in apply order: the shared account table,
// then the framework's authorization tables (auth_item, auth_assignment,
// ... — the backend module's admin guard reads them through the RBAC
// manager).
func All() []migrate.Migration {
	return []migrate.Migration{
		m20260101_000001_create_user_table{},
		rbac.NewMigration(),
	}
}
