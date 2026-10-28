package migrate

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/microsoft/go-mssqldb"
)

func mssqlDSN() string { return os.Getenv("SQLSERVER_TEST_DSN") }

func TestMSSQLIntegrationCreateTableAndCRUD(t *testing.T) {
	dsn := mssqlDSN()
	if dsn == "" {
		t.Skip("SQLSERVER_TEST_DSN not set")
	}

	db, err := sql.Open("sqlserver", dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	table := fmt.Sprintf("test_%d", time.Now().UnixNano())
	m := NewMigrationContext(db, SQLServerDialect{})

	err = m.Schema().
		CreateTableIfNotExists(ctx, table, Columns().
			Add("id", m.BigPrimaryKey()).
			Add("name", m.String(100).NotNull()).
			Add("data", m.Json().Null()).
			Add("created_at", m.Timestamp(0).NotNull().DefaultExpression("GETDATE()")),
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

	exists, err := m.RowExists(ctx, table, "name = @p1", "test")
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
