package migrations

import (
	"context"

	"github.com/goyii2/framework/migrate"
)

// m20260101_000004_create_post_table creates the shared content table: the
// posts the backend manages and the frontend shows. The author reference is
// a plain column (the model's association resolves it to the user table),
// status carries the publishing workflow, and the audit columns are stamped
// by the model's Timestamp behavior on every save.
type m20260101_000004_create_post_table struct{}

func (m20260101_000004_create_post_table) Name() string {
	return "m20260101_000004_create_post_table"
}

func (m20260101_000004_create_post_table) Up(ctx context.Context, m *migrate.MigrationContext) error {
	return m.Schema().
		CreateTable("post", migrate.Columns().
			Add("id", m.BigPrimaryKey()).
			Add("title", m.String(200).NotNull()).
			Add("body", m.Text().Null()).
			Add("status", m.String(16).NotNull().DefaultValue("draft")).
			Add("author_id", m.BigInteger().NotNull()).
			Add("created_at", m.DateTime().Null()).
			Add("updated_at", m.DateTime().Null()),
		).
		Exec(ctx)
}

func (m20260101_000004_create_post_table) Down(ctx context.Context, m *migrate.MigrationContext) error {
	return m.Schema().DropTable("post").Exec(ctx)
}
