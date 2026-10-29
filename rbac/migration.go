package rbac

import (
	"context"

	"github.com/goyii2/framework/migrate"
)

// NewMigration returns the migration creating the four authorization tables
// with the framework's migration engine. Applications append it to their
// migration list — the same schema reaches every supported database through
// the dialect layer, so the manager never creates tables itself:
//
//	migrations.All() = append(migrations.All(), rbac.NewMigration())
//
// Two deliberate divergences from the Yii2 DDL, both forced by dialect
// portability: the composite keys of auth_item_child and auth_assignment are
// UNIQUE indexes rather than PRIMARY KEY constraints (SQLite cannot add a
// primary key after the fact), and the tables carry no foreign keys (the
// manager removes edges and assignments explicitly when an item goes away, so
// referential integrity does not depend on the engine).
func NewMigration() migrate.Migration { return createAuthTables{} }

// createAuthTables creates auth_rule, auth_item, auth_item_child and
// auth_assignment. auth_rule stores only rule names and their creation time —
// rule behavior is code, never data.
type createAuthTables struct{}

// Name sorts after the example application's own table migrations, so
// applying the combined list creates the business tables first.
func (createAuthTables) Name() string { return "m20260101_000003_create_auth_tables" }

func (createAuthTables) Up(ctx context.Context, m *migrate.MigrationContext) error {
	return m.Schema().
		CreateTable("auth_rule", migrate.Columns().
			// A string primary key spelled through Append: the PrimaryKey
			// builder flag renders as INTEGER on SQLite, which would be the
			// wrong type for a name column.
			Add("name", m.String(64).NotNull().Append("PRIMARY KEY")).
			Add("created_at", m.BigInteger().NotNull()),
		).
		CreateTable("auth_item", migrate.Columns().
			Add("name", m.String(64).NotNull().Append("PRIMARY KEY")).
			Add("type", m.SmallInteger().NotNull()).
			Add("description", m.Text()).
			Add("rule_name", m.String(64)).
			Add("created_at", m.BigInteger().NotNull()).
			Add("updated_at", m.BigInteger().NotNull()),
		).
		CreateTable("auth_item_child", migrate.Columns().
			Add("parent", m.String(64).NotNull()).
			Add("child", m.String(64).NotNull()),
		).
		CreateIndexIfNotExists(ctx, "ux_auth_item_child", "auth_item_child", []string{"parent", "child"}, true).
		CreateTable("auth_assignment", migrate.Columns().
			Add("item_name", m.String(64).NotNull()).
			Add("user_id", m.String(64).NotNull()).
			Add("created_at", m.BigInteger().NotNull()),
		).
		CreateIndexIfNotExists(ctx, "ux_auth_assignment", "auth_assignment", []string{"item_name", "user_id"}, true).
		Exec(ctx)
}

func (createAuthTables) Down(ctx context.Context, m *migrate.MigrationContext) error {
	return m.Schema().
		DropTableIfExists(ctx, "auth_assignment").
		DropTableIfExists(ctx, "auth_item_child").
		DropTableIfExists(ctx, "auth_item").
		DropTableIfExists(ctx, "auth_rule").
		Exec(ctx)
}
