package migrations

import (
	"context"

	"github.com/goyii2/framework/migrate"
)

// m20260101_000002_create_post_table creates the demo's business table. The
// author reference is a plain column; the application layer expresses it as
// an association (Post.Author) and eager-loads it on demand.
type m20260101_000002_create_post_table struct{}

func (m20260101_000002_create_post_table) Name() string {
	return "m20260101_000002_create_post_table"
}

func (m20260101_000002_create_post_table) Up(ctx context.Context, m *migrate.MigrationContext) error {
	return m.Schema().
		CreateTable("post", migrate.Columns().
			Add("id", m.BigPrimaryKey()).
			Add("title", m.String(200).NotNull()).
			Add("body", m.Text().Null()).
			Add("status", m.String(16).NotNull().DefaultValue("draft")).
			Add("author_id", m.BigInteger().NotNull()),
		).
		Exec(ctx)
}

func (m20260101_000002_create_post_table) Down(ctx context.Context, m *migrate.MigrationContext) error {
	return m.Schema().DropTable("post").Exec(ctx)
}
