// Package statsmigrations holds the secondary database's migrations. The
// "stats" migrate target (registered in main.go) runs exactly this list on
// its own connection — migrate --db=stats up — keeping the secondary schema
// separate from the primary list in the migrations package.
package statsmigrations

import (
	"context"

	"github.com/goyii2/framework/migrate"
)

// m20260101_000001_create_visit_log creates the stats database's single
// table: one row per recorded page visit, unix seconds as the timestamp so
// every dialect stores it the same way.
type m20260101_000001_create_visit_log struct{}

func (m20260101_000001_create_visit_log) Name() string {
	return "m20260101_000001_create_visit_log"
}

func (m20260101_000001_create_visit_log) Up(ctx context.Context, m *migrate.MigrationContext) error {
	return m.Schema().
		CreateTable("visit_log", migrate.Columns().
			Add("id", m.BigPrimaryKey()).
			Add("path", m.String(255).NotNull()).
			Add("visited_at", m.BigInteger().NotNull()),
		).
		Exec(ctx)
}

func (m20260101_000001_create_visit_log) Down(ctx context.Context, m *migrate.MigrationContext) error {
	return m.Schema().DropTable("visit_log").Exec(ctx)
}

// All returns the stats database's migrations in apply order.
func All() []migrate.Migration {
	return []migrate.Migration{
		m20260101_000001_create_visit_log{},
	}
}
