package migrations

import (
	"context"

	"github.com/goyii2/framework/migrate"
)

// m20260101_000004_add_post_timestamps adds the audit columns the Timestamp
// behavior fills: created_at on insert, updated_at on insert and every
// update. Existing rows keep NULL — the behavior stamps rows as they are
// written from now on.
type m20260101_000004_add_post_timestamps struct{}

func (m20260101_000004_add_post_timestamps) Name() string {
	return "m20260101_000004_add_post_timestamps"
}

func (m20260101_000004_add_post_timestamps) Up(ctx context.Context, m *migrate.MigrationContext) error {
	return m.Schema().
		AddColumnIfNotExists(ctx, "post", "created_at", m.DateTime().Null()).
		AddColumnIfNotExists(ctx, "post", "updated_at", m.DateTime().Null()).
		Exec(ctx)
}

func (m20260101_000004_add_post_timestamps) Down(ctx context.Context, m *migrate.MigrationContext) error {
	return m.Schema().
		DropColumnIfExists(ctx, "post", "created_at").
		DropColumnIfExists(ctx, "post", "updated_at").
		Exec(ctx)
}
