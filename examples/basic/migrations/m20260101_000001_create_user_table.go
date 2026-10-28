package migrations

import (
	"context"

	"github.com/goyii2/framework/migrate"
)

// m20260101_000001_create_user_table creates the account table of the
// example: login name, bcrypt hash and the role that drives access control.
type m20260101_000001_create_user_table struct{}

func (m20260101_000001_create_user_table) Name() string {
	return "m20260101_000001_create_user_table"
}

func (m20260101_000001_create_user_table) Up(ctx context.Context, m *migrate.MigrationContext) error {
	return m.Schema().
		CreateTable("user", migrate.Columns().
			Add("id", m.BigPrimaryKey()).
			Add("username", m.String(64).NotNull()).
			Add("password_hash", m.String(128).NotNull()).
			Add("role", m.String(16).NotNull().DefaultValue("user")),
		).
		Exec(ctx)
}

func (m20260101_000001_create_user_table) Down(ctx context.Context, m *migrate.MigrationContext) error {
	return m.Schema().DropTable("user").Exec(ctx)
}
