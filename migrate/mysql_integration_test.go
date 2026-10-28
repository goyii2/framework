package migrate

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

func TestMySQLIntegrationSchemaAndQueryHelpers(t *testing.T) {
	dsn := os.Getenv("MYSQL_TEST_DSN")
	if dsn == "" {
		t.Skip("MYSQL_TEST_DSN is not set")
	}

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	migration := NewMigrationContext(db, MySQLDialect{})
	table := fmt.Sprintf("yiimigrate_it_%d", time.Now().UnixNano())
	defer func() {
		_ = migration.Schema().DropTableIfExists(ctx, table).Exec(ctx)
	}()

	if err := migration.Schema().
		CreateTableIfNotExists(ctx, table, Columns().
			Add("id", migration.UnsignedBigPrimaryKey()).
			Add("username", migration.String(64).NotNull()).
			Add("metadata", migration.Json().Null()),
			"ENGINE=InnoDB DEFAULT CHARSET=utf8mb4",
		).
		CreateIndexIfNotExists(ctx, "idx-"+table+"-username", table, []string{"username"}, true).
		Insert(table, Row{
			"username": "admin",
			"metadata": Expr("JSON_OBJECT('source', 'integration')"),
		}).
		Exec(ctx); err != nil {
		t.Fatalf("schema exec error = %v", err)
	}

	exists, err := migration.RowExists(ctx, table, "username = ?", "admin")
	if err != nil {
		t.Fatalf("RowExists() error = %v", err)
	}
	if !exists {
		t.Fatalf("RowExists() = false, want true")
	}

	count, err := migration.CountRows(ctx, table, "username = ?", "admin")
	if err != nil {
		t.Fatalf("CountRows() error = %v", err)
	}
	if count != 1 {
		t.Fatalf("CountRows() = %d, want 1", count)
	}
}
