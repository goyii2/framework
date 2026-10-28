package migrate

import (
	"reflect"
	"testing"
)

// SQL Server-specific divergence only: catalog probes, application locks and
// quoting/identity quirks. Everything the adapters share in structure is
// asserted by the conformance matrix in dialect_conformance_test.go.

func TestSQLServerQuoteColumnBracketEscape(t *testing.T) {
	d := SQLServerDialect{}
	if got := d.QuoteColumn("name]"); got != "[name]]]" {
		t.Fatalf("QuoteColumn with ] = %q, want %q", got, "[name]]]")
	}
}

func TestSQLServerBuildColumnPrimaryKeyNonIdentity(t *testing.T) {
	m := NewMigrationContext(nil, SQLServerDialect{})
	d := SQLServerDialect{}

	got, err := d.BuildColumn(m.String(36).PrimaryKey())
	if err != nil {
		t.Fatalf("PrimaryKey non-identity returned error: %v", err)
	}
	if got != "NVARCHAR(36) NOT NULL PRIMARY KEY" {
		t.Fatalf("PrimaryKey non-identity = %q, want %q", got, "NVARCHAR(36) NOT NULL PRIMARY KEY")
	}
}

func TestSQLServerTableExistsSQL(t *testing.T) {
	d := SQLServerDialect{}
	stmt := d.TableExistsSQL("article")
	want := "SELECT COUNT(*) FROM sys.tables WHERE SCHEMA_NAME(schema_id) = SCHEMA_NAME() AND name = @p1"
	if stmt.Query != want {
		t.Fatalf("TableExistsSQL query = %q, want %q", stmt.Query, want)
	}
	if !reflect.DeepEqual(stmt.Args, []any{"article"}) {
		t.Fatalf("TableExistsSQL args = %#v", stmt.Args)
	}
}

func TestSQLServerColumnExistsSQL(t *testing.T) {
	d := SQLServerDialect{}
	stmt := d.ColumnExistsSQL("article", "title")
	if stmt.Query == "" {
		t.Fatalf("ColumnExistsSQL query is empty")
	}
	if !reflect.DeepEqual(stmt.Args, []any{"article", "title"}) {
		t.Fatalf("ColumnExistsSQL args = %#v", stmt.Args)
	}
}

func TestSQLServerIndexExistsSQL(t *testing.T) {
	d := SQLServerDialect{}
	stmt := d.IndexExistsSQL("article", "idx_title")
	if !reflect.DeepEqual(stmt.Args, []any{"article", "idx_title"}) {
		t.Fatalf("IndexExistsSQL args = %#v", stmt.Args)
	}
}

func TestSQLServerForeignKeyExistsSQL(t *testing.T) {
	d := SQLServerDialect{}
	stmt := d.ForeignKeyExistsSQL("article", "fk_user")
	if !reflect.DeepEqual(stmt.Args, []any{"article", "fk_user"}) {
		t.Fatalf("ForeignKeyExistsSQL args = %#v", stmt.Args)
	}
}

func TestSQLServerConstraintExistsSQL(t *testing.T) {
	d := SQLServerDialect{}
	stmt := d.ConstraintExistsSQL("article", "chk_status")
	if !reflect.DeepEqual(stmt.Args, []any{"article", "chk_status"}) {
		t.Fatalf("ConstraintExistsSQL args = %#v", stmt.Args)
	}
}

func TestSQLServerAcquireLockSQL(t *testing.T) {
	d := SQLServerDialect{}
	stmt, err := d.AcquireLockSQL("migration_lock", 30)
	if err != nil {
		t.Fatalf("AcquireLockSQL error: %v", err)
	}
	if !reflect.DeepEqual(stmt.Args, []any{"migration_lock", 30}) {
		t.Fatalf("AcquireLockSQL args = %#v", stmt.Args)
	}
	if stmt.Query == "" {
		t.Fatalf("AcquireLockSQL query is empty")
	}
}

func TestSQLServerReleaseLockSQL(t *testing.T) {
	d := SQLServerDialect{}
	stmt, err := d.ReleaseLockSQL("migration_lock")
	if err != nil {
		t.Fatalf("ReleaseLockSQL error: %v", err)
	}
	if !reflect.DeepEqual(stmt.Args, []any{"migration_lock"}) {
		t.Fatalf("ReleaseLockSQL args = %#v", stmt.Args)
	}
	if stmt.Query == "" {
		t.Fatalf("ReleaseLockSQL query is empty")
	}
}

// Statements that embed object names inside string literals (sp_rename,
// extended properties, the OBJECT_ID probe) must double every single quote,
// so names containing ' still produce one legal SQL string literal.
func TestSQLServerIdentifierLiteralsEscapeSingleQuotes(t *testing.T) {
	d := SQLServerDialect{}
	cases := []struct {
		name  string
		build func() (string, error)
		want  string
	}{
		{
			name:  "rename table",
			build: func() (string, error) { return d.RenameTable("weird'table", "new'name") },
			want:  "EXEC sp_rename 'weird''table', 'new''name'",
		},
		{
			name:  "rename column",
			build: func() (string, error) { return d.RenameColumn("weird'table", "old'name", "new'name") },
			want:  "EXEC sp_rename 'weird''table.old''name', 'new''name', 'COLUMN'",
		},
		{
			name:  "add comment on column",
			build: func() (string, error) { return d.AddCommentOnColumn("weird'table", "name's", "it's a comment") },
			want:  "EXEC sp_addextendedproperty 'MS_Description', 'it''s a comment', 'SCHEMA', SCHEMA_NAME(), 'TABLE', 'weird''table', 'COLUMN', 'name''s'",
		},
		{
			name:  "drop comment from column",
			build: func() (string, error) { return d.DropCommentFromColumn("weird'table", "name's") },
			want:  "EXEC sp_dropextendedproperty 'MS_Description', 'SCHEMA', SCHEMA_NAME(), 'TABLE', 'weird''table', 'COLUMN', 'name''s'",
		},
		{
			name:  "add comment on table",
			build: func() (string, error) { return d.AddCommentOnTable("weird'table", "it's a table") },
			want:  "EXEC sp_addextendedproperty 'MS_Description', 'it''s a table', 'SCHEMA', SCHEMA_NAME(), 'TABLE', 'weird''table'",
		},
		{
			name:  "drop comment from table",
			build: func() (string, error) { return d.DropCommentFromTable("weird'table") },
			want:  "EXEC sp_dropextendedproperty 'MS_Description', 'SCHEMA', SCHEMA_NAME(), 'TABLE', 'weird''table'",
		},
		{
			name:  "create migration table",
			build: func() (string, error) { return d.CreateMigrationTableSQL("weird'table") },
			want:  "IF OBJECT_ID(N'[weird''table]', 'U') IS NULL BEGIN CREATE TABLE [weird'table] ([version] varchar(180) NOT NULL PRIMARY KEY, [apply_time] int NOT NULL) END",
		},
	}
	for _, tc := range cases {
		got, err := tc.build()
		if err != nil {
			t.Fatalf("%s returned error: %v", tc.name, err)
		}
		if got != tc.want {
			t.Fatalf("%s = %q, want %q", tc.name, got, tc.want)
		}
	}
}
