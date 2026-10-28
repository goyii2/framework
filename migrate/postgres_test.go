package migrate

import (
	"reflect"
	"testing"
)

// PostgreSQL-specific divergence only: existence probes against the catalog
// and advisory locks. Everything the adapters share in structure is asserted
// by the conformance matrix in dialect_conformance_test.go.

func TestPostgreSQLTableExistsSQL(t *testing.T) {
	d := PostgreSQLDialect{}
	s := d.TableExistsSQL("article")
	want := "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = current_schema() AND table_name = $1"
	if s.Query != want {
		t.Fatalf("TableExistsSQL query = %q, want %q", s.Query, want)
	}
	if !reflect.DeepEqual(s.Args, []any{"article"}) {
		t.Fatalf("TableExistsSQL args = %#v", s.Args)
	}
}

func TestPostgreSQLColumnExistsSQL(t *testing.T) {
	d := PostgreSQLDialect{}
	s := d.ColumnExistsSQL("article", "title")
	want := "SELECT COUNT(*) FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = $1 AND column_name = $2"
	if s.Query != want {
		t.Fatalf("ColumnExistsSQL query = %q, want %q", s.Query, want)
	}
	if !reflect.DeepEqual(s.Args, []any{"article", "title"}) {
		t.Fatalf("ColumnExistsSQL args = %#v", s.Args)
	}
}

func TestPostgreSQLIndexExistsSQL(t *testing.T) {
	d := PostgreSQLDialect{}
	s := d.IndexExistsSQL("article", "idx-title")
	want := "SELECT COUNT(*) FROM pg_indexes WHERE schemaname = current_schema() AND tablename = $1 AND indexname = $2"
	if s.Query != want {
		t.Fatalf("IndexExistsSQL query = %q, want %q", s.Query, want)
	}
	if !reflect.DeepEqual(s.Args, []any{"article", "idx-title"}) {
		t.Fatalf("IndexExistsSQL args = %#v", s.Args)
	}
}

func TestPostgreSQLForeignKeyExistsSQL(t *testing.T) {
	d := PostgreSQLDialect{}
	s := d.ForeignKeyExistsSQL("article", "fk_user")
	want := "SELECT COUNT(*) FROM information_schema.table_constraints WHERE table_schema = current_schema() AND table_name = $1 AND constraint_name = $2 AND constraint_type = 'FOREIGN KEY'"
	if s.Query != want {
		t.Fatalf("ForeignKeyExistsSQL query = %q, want %q", s.Query, want)
	}
	if !reflect.DeepEqual(s.Args, []any{"article", "fk_user"}) {
		t.Fatalf("ForeignKeyExistsSQL args = %#v", s.Args)
	}
}

func TestPostgreSQLConstraintExistsSQL(t *testing.T) {
	d := PostgreSQLDialect{}
	s := d.ConstraintExistsSQL("article", "chk_id")
	want := "SELECT COUNT(*) FROM information_schema.table_constraints WHERE table_schema = current_schema() AND table_name = $1 AND constraint_name = $2"
	if s.Query != want {
		t.Fatalf("ConstraintExistsSQL query = %q, want %q", s.Query, want)
	}
	if !reflect.DeepEqual(s.Args, []any{"article", "chk_id"}) {
		t.Fatalf("ConstraintExistsSQL args = %#v", s.Args)
	}
}

func TestPostgreSQLAcquireLockSQL(t *testing.T) {
	d := PostgreSQLDialect{}
	stmt, err := d.AcquireLockSQL("my_migration", 10)
	if err != nil {
		t.Fatalf("AcquireLockSQL error: %v", err)
	}
	wantQuery := `SELECT pg_advisory_lock(hashtext($1))`
	if stmt.Query != wantQuery {
		t.Fatalf("AcquireLockSQL query = %q, want %q", stmt.Query, wantQuery)
	}
	if !reflect.DeepEqual(stmt.Args, []any{"my_migration"}) {
		t.Fatalf("AcquireLockSQL args = %#v", stmt.Args)
	}
}

func TestPostgreSQLReleaseLockSQL(t *testing.T) {
	d := PostgreSQLDialect{}
	stmt, err := d.ReleaseLockSQL("my_migration")
	if err != nil {
		t.Fatalf("ReleaseLockSQL error: %v", err)
	}
	wantQuery := `SELECT pg_advisory_unlock(hashtext($1))`
	if stmt.Query != wantQuery {
		t.Fatalf("ReleaseLockSQL query = %q, want %q", stmt.Query, wantQuery)
	}
	if !reflect.DeepEqual(stmt.Args, []any{"my_migration"}) {
		t.Fatalf("ReleaseLockSQL args = %#v", stmt.Args)
	}
}
