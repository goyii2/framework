// Package migrations holds the example application's database migrations.
// All() is the list handed to the goyii migrate command tree; versions apply
// in the order listed here.
package migrations

import "github.com/goyii2/framework/migrate"

// All returns every migration in apply order.
func All() []migrate.Migration {
	return []migrate.Migration{
		m20260101_000001_create_user_table{},
		m20260101_000002_create_post_table{},
	}
}
