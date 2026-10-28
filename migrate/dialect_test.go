package migrate

import (
	"reflect"
	"testing"
)

// SQLite-specific divergence only: catalog probes against sqlite_master and
// pragma_table_info, plus the sentinel probes for named constraints that
// SQLite cannot represent. Everything the adapters share in structure is
// asserted by the conformance matrix in dialect_conformance_test.go.

func TestSQLiteTableExistsSQL(t *testing.T) {
	s := SQLiteDialect{}.TableExistsSQL("article")
	want := `SELECT COUNT(*) FROM sqlite_master WHERE type IN ('table', 'view') AND name = ?`
	if s.Query != want {
		t.Fatalf("TableExistsSQL query = %q, want %q", s.Query, want)
	}
	if !reflect.DeepEqual(s.Args, []any{"article"}) {
		t.Fatalf("TableExistsSQL args = %#v", s.Args)
	}
}

func TestSQLiteColumnExistsSQL(t *testing.T) {
	s := SQLiteDialect{}.ColumnExistsSQL("article", "title")
	want := `SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?`
	if s.Query != want {
		t.Fatalf("ColumnExistsSQL query = %q, want %q", s.Query, want)
	}
	if !reflect.DeepEqual(s.Args, []any{"article", "title"}) {
		t.Fatalf("ColumnExistsSQL args = %#v", s.Args)
	}
}

func TestSQLiteIndexExistsSQL(t *testing.T) {
	s := SQLiteDialect{}.IndexExistsSQL("article", "idx_title")
	want := `SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND tbl_name = ? AND name = ?`
	if s.Query != want {
		t.Fatalf("IndexExistsSQL query = %q, want %q", s.Query, want)
	}
	if !reflect.DeepEqual(s.Args, []any{"article", "idx_title"}) {
		t.Fatalf("IndexExistsSQL args = %#v", s.Args)
	}
}

func TestSQLiteNamedConstraintProbesReportAbsent(t *testing.T) {
	d := SQLiteDialect{}

	fk := d.ForeignKeyExistsSQL("article", "fk_user")
	if fk.Query != "SELECT 0" || len(fk.Args) != 0 {
		t.Fatalf("ForeignKeyExistsSQL = %q args %#v, want constant absent probe", fk.Query, fk.Args)
	}

	constraint := d.ConstraintExistsSQL("article", "chk_status")
	if constraint.Query != "SELECT 0" || len(constraint.Args) != 0 {
		t.Fatalf("ConstraintExistsSQL = %q args %#v, want constant absent probe", constraint.Query, constraint.Args)
	}
}
