// Package migrations holds the example application's database migrations.
// All() is the list handed to the goyii migrate command tree; versions apply
// in the order listed here.
package migrations

import (
	"github.com/goyii2/framework/migrate"
	"github.com/goyii2/framework/queue"
	"github.com/goyii2/framework/rbac"
)

// All returns every migration in apply order. The RBAC tables and the queue
// table come from the framework's own migrations (auth_item, auth_item_child,
// auth_rule, auth_assignment and the queue driver's table), so the example's
// schema stays the one every goyii application gets. The queue table name is
// the default the config section also declares (jobs.Table in the jobs package).
func All() []migrate.Migration {
	return []migrate.Migration{
		m20260101_000001_create_user_table{},
		m20260101_000002_create_post_table{},
		rbac.NewMigration(),
		m20260101_000004_add_post_timestamps{},
		queue.NewMigration("queue"),
	}
}
