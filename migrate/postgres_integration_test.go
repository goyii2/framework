package migrate

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"
)

func pgDSN() string { return os.Getenv("POSTGRES_TEST_DSN") }

func TestPGIntegrationCreateTableAndCRUD(t *testing.T) {
	dsn := pgDSN()
	if dsn == "" {
		t.Skip("POSTGRES_TEST_DSN not set")
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	table := fmt.Sprintf("test_%d", time.Now().UnixNano())
	m := NewMigrationContext(db, PostgreSQLDialect{})

	err = m.Schema().
		CreateTableIfNotExists(ctx, table, Columns().
			Add("id", m.BigPrimaryKey()).
			Add("name", m.String(100).NotNull()).
			Add("data", m.Json().Null()).
			Add("created_at", m.Timestamp(0).NotNull().DefaultExpression("CURRENT_TIMESTAMP")),
			"",
		).Exec(ctx)
	if err != nil {
		t.Fatalf("create table: %v", err)
	}

	err = m.Schema().Insert(table, Row{
		"name": "test",
	}).Exec(ctx)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	exists, err := m.RowExists(ctx, table, "name = $1", "test")
	if err != nil {
		t.Fatalf("row exists: %v", err)
	}
	if !exists {
		t.Fatal("expected row to exist")
	}

	count, err := m.CountRows(ctx, table, "")
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 row, got %d", count)
	}

	err = m.Schema().DropTableIfExists(ctx, table).Exec(ctx)
	if err != nil {
		t.Fatalf("drop table: %v", err)
	}
}
