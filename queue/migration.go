package queue

import (
	"context"

	"github.com/goyii2/framework/migrate"
)

// NewMigration returns the migration creating the queue table — tableName, or
// the default "queue" when empty — with the framework's migration engine.
// Applications append it to their migration list so the same schema reaches
// every supported database through the dialect layer; the db driver itself
// never issues DDL:
//
//	migrations.All() = append(migrations.All(), queue.NewMigration(cfg.Queue.TableName))
//
// All time columns are BIGINT Unix nanoseconds (the reserved_at 0 value means
// "not held"), matching what the db driver writes; there are no foreign keys.
func NewMigration(tableName string) migrate.Migration {
	return createQueueTable{table: tableName}
}

// createQueueTable creates the queue table with its dispatch index.
type createQueueTable struct{ table string }

// Name sorts after the rbac tables migration.
func (createQueueTable) Name() string { return "m20260101_000004_create_queue_table" }

func (c createQueueTable) Up(ctx context.Context, m *migrate.MigrationContext) error {
	table := c.table
	if table == "" {
		table = DefaultTableName
	}
	return m.Schema().
		CreateTable(table, migrate.Columns().
			Add("id", m.BigPrimaryKey()).
			Add("name", m.String(191).NotNull()).
			Add("payload", m.Text().NotNull()).
			Add("status", m.SmallInteger().NotNull()).
			Add("priority", m.Integer().NotNull()).
			Add("attempts", m.Integer().NotNull()).
			Add("attempt", m.Integer().NotNull()).
			Add("ttr", m.BigInteger().NotNull()).
			Add("due_at", m.BigInteger().NotNull()).
			Add("reserved_at", m.BigInteger().NotNull()).
			Add("claim", m.String(36).NotNull()).
			Add("pushed_at", m.BigInteger().NotNull()).
			Add("retry_delays", m.Text().NotNull()),
		).
		// The dispatch scan filters status + due and orders by priority and
		// due; the index covers exactly that path.
		CreateIndexIfNotExists(ctx, "ix_"+table+"_dispatch", table, []string{"status", "priority", "due_at", "id"}, false).
		Exec(ctx)
}

func (c createQueueTable) Down(ctx context.Context, m *migrate.MigrationContext) error {
	table := c.table
	if table == "" {
		table = DefaultTableName
	}
	return m.Schema().
		DropTableIfExists(ctx, table).
		Exec(ctx)
}
