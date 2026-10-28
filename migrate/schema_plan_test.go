package migrate

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
)

type execCall struct {
	query string
	args  []any
}

type fakeDBTX struct {
	execCalls []execCall
}

func (f *fakeDBTX) ExecContext(_ context.Context, query string, args ...any) (sql.Result, error) {
	f.execCalls = append(f.execCalls, execCall{query: query, args: append([]any(nil), args...)})
	return nil, nil
}

func (f *fakeDBTX) QueryContext(context.Context, string, ...any) (*sql.Rows, error) {
	return nil, errors.New("query not implemented")
}

func (f *fakeDBTX) QueryRowContext(context.Context, string, ...any) *sql.Row {
	return nil
}

func TestSchemaPlanExecutesStatements(t *testing.T) {
	db := &fakeDBTX{}
	m := NewMigrationContext(db, SQLiteDialect{})

	err := m.Schema().
		Raw(`CREATE TABLE "article" ("id" INTEGER)`).
		Insert("article", Row{"id": 1}).
		Exec(context.Background())
	if err != nil {
		t.Fatalf("Exec returned error: %v", err)
	}
	if len(db.execCalls) != 2 {
		t.Fatalf("exec call count = %d, want 2", len(db.execCalls))
	}
	if db.execCalls[1].query != `INSERT INTO "article" ("id") VALUES (?)` {
		t.Fatalf("insert query = %s", db.execCalls[1].query)
	}
}

func TestSchemaPlanDryRunSkipsExecution(t *testing.T) {
	db := &fakeDBTX{}
	m := NewMigrationContext(db, SQLiteDialect{}, WithDryRun(true))

	err := m.Schema().
		Raw(`CREATE TABLE "article" ("id" INTEGER)`).
		Exec(context.Background())
	if err != nil {
		t.Fatalf("Exec returned error: %v", err)
	}
	if len(db.execCalls) != 0 {
		t.Fatalf("dry run executed %d statements", len(db.execCalls))
	}
}

func TestSchemaPlanDryRunIfNotExistsDoesNotQueryMetadata(t *testing.T) {
	db := &fakeDBTX{}
	m := NewMigrationContext(db, SQLiteDialect{}, WithDryRun(true))

	err := m.Schema().
		CreateTableIfNotExists(context.Background(), "article", Columns().
			Add("id", m.PrimaryKey()),
		).
		Exec(context.Background())
	if err != nil {
		t.Fatalf("Exec returned error: %v", err)
	}
	if len(db.execCalls) != 0 {
		t.Fatalf("dry run executed %d statements", len(db.execCalls))
	}
}

func TestSchemaPlanStopsOnAccumulatedError(t *testing.T) {
	db := &fakeDBTX{}
	m := NewMigrationContext(db, SQLiteDialect{})

	err := m.Schema().
		AlterColumn("article", "title", m.String(128)).
		Raw(`CREATE TABLE "article" ("id" INTEGER)`).
		Exec(context.Background())
	if err == nil {
		t.Fatal("Exec returned nil error")
	}
	if !strings.Contains(err.Error(), "sqlite does not support ALTER COLUMN") {
		t.Fatalf("Exec error = %v", err)
	}
	if len(db.execCalls) != 0 {
		t.Fatalf("plan with accumulated error executed %d statements", len(db.execCalls))
	}
}

func TestSchemaPlanStatementsReturnsAccumulatedSQL(t *testing.T) {
	m := NewMigrationContext(nil, SQLiteDialect{}, WithDryRun(true))

	plan := m.Schema().
		Raw(`CREATE TABLE "article" ("id" INTEGER)`).
		Insert("article", Row{"id": 1, "title": "hi"})

	stmts := plan.Statements()
	if len(stmts) != 2 {
		t.Fatalf("Statements() returned %d items, want 2", len(stmts))
	}
	if stmts[0].Query != `CREATE TABLE "article" ("id" INTEGER)` {
		t.Fatalf("stmts[0].Query = %q", stmts[0].Query)
	}
	if !strings.Contains(stmts[1].Query, `INSERT INTO "article"`) {
		t.Fatalf("stmts[1].Query = %q", stmts[1].Query)
	}
	if len(stmts[1].Args) != 2 {
		t.Fatalf("stmts[1].Args = %v (len %d)", stmts[1].Args, len(stmts[1].Args))
	}

	// Mutating the returned slice must not corrupt the plan.
	stmts[0] = SQLStatement{Query: "mutated"}
	again := plan.Statements()
	if again[0].Query != `CREATE TABLE "article" ("id" INTEGER)` {
		t.Fatalf("plan statements mutated via returned slice: %q", again[0].Query)
	}
}

// The drop-class IfExists variants are the only idempotency mechanism: the
// probe decides, and the emitted statement itself is always a strict drop
// with no IF EXISTS (adapter drops report missing objects as errors).
func TestSchemaPlanDropIfExistsProbesBeforeStrictDrop(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name     string
		seed     func(state *fakeSQLState)
		plan     func(m *MigrationContext) *SchemaPlan
		wantDrop string
	}{
		{
			name:     "drop table",
			seed:     func(state *fakeSQLState) { state.tables["article"] = true },
			plan:     func(m *MigrationContext) *SchemaPlan { return m.Schema().DropTableIfExists(ctx, "article") },
			wantDrop: "DROP TABLE `article`",
		},
		{
			name:     "drop column",
			seed:     func(state *fakeSQLState) { state.columns["article.summary"] = true },
			plan:     func(m *MigrationContext) *SchemaPlan { return m.Schema().DropColumnIfExists(ctx, "article", "summary") },
			wantDrop: "ALTER TABLE `article` DROP COLUMN `summary`",
		},
		{
			name: "drop index",
			seed: func(state *fakeSQLState) { state.indexes["article.idx_article_title"] = true },
			plan: func(m *MigrationContext) *SchemaPlan {
				return m.Schema().DropIndexIfExists(ctx, "idx_article_title", "article")
			},
			wantDrop: "DROP INDEX `idx_article_title` ON `article`",
		},
		{
			name: "drop primary key",
			seed: func(state *fakeSQLState) { state.constraints["article.pk_article"] = true },
			plan: func(m *MigrationContext) *SchemaPlan {
				return m.Schema().DropPrimaryKeyIfExists(ctx, "pk_article", "article")
			},
			wantDrop: "ALTER TABLE `article` DROP PRIMARY KEY",
		},
		{
			name: "drop foreign key",
			seed: func(state *fakeSQLState) { state.foreignKeys["article.fk_article_user"] = true },
			plan: func(m *MigrationContext) *SchemaPlan {
				return m.Schema().DropForeignKeyIfExists(ctx, "fk_article_user", "article")
			},
			wantDrop: "ALTER TABLE `article` DROP FOREIGN KEY `fk_article_user`",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			state := newFakeSQLState()
			tc.seed(state)
			db := openFakeSQLDB(t, state)
			m := NewMigrationContext(db, MySQLDialect{})

			if err := tc.plan(m).Exec(ctx); err != nil {
				t.Fatalf("Exec() error: %v", err)
			}
			queries := state.execQueries()
			if len(queries) != 1 || queries[0] != tc.wantDrop {
				t.Fatalf("executed %#v, want [%s]", queries, tc.wantDrop)
			}

			// With the object absent the probe suppresses the statement: no
			// strict drop is attempted against the database.
			emptyState := newFakeSQLState()
			emptyDB := openFakeSQLDB(t, emptyState)
			emptyCtx := NewMigrationContext(emptyDB, MySQLDialect{})
			if err := tc.plan(emptyCtx).Exec(ctx); err != nil {
				t.Fatalf("Exec() with missing object error: %v", err)
			}
			if queries := emptyState.execQueries(); len(queries) != 0 {
				t.Fatalf("drop executed for missing object: %#v", queries)
			}
		})
	}
}

// In dry-run the drop-class IfExists variants skip probing and still surface
// the strict drop statement for inspection, without executing anything.
func TestSchemaPlanDropIfExistsDryRunEmitsStrictDrop(t *testing.T) {
	state := newFakeSQLState()
	db := openFakeSQLDB(t, state)
	m := NewMigrationContext(db, MySQLDialect{}, WithDryRun(true))

	plan := m.Schema().DropTableIfExists(context.Background(), "article")
	stmts := plan.Statements()
	if len(stmts) != 1 || stmts[0].Query != "DROP TABLE `article`" {
		t.Fatalf("statements = %#v, want one strict DROP TABLE `article`", stmts)
	}
	if err := plan.Exec(context.Background()); err != nil {
		t.Fatalf("Exec() error: %v", err)
	}
	if queries := state.execQueries(); len(queries) != 0 {
		t.Fatalf("dry run executed %#v", queries)
	}
}

// The shared existence guard behind every *IfExists variant, exercised
// directly: dry-run emits without probing, the probe result decides emission
// per polarity, and probe failures are recorded on the plan.
func TestSchemaPlanExistenceGuard(t *testing.T) {
	probeFailure := errors.New("probe failed")

	cases := []struct {
		name        string
		dryRun      bool
		emitOnExist bool
		exists      bool
		probeErr    error
		wantProbe   bool
		wantEmit    bool
		wantErr     error
	}{
		{
			name:     "dry run emits without probing",
			dryRun:   true,
			wantEmit: true,
		},
		{
			name:        "drop polarity emits when object exists",
			emitOnExist: true,
			exists:      true,
			wantProbe:   true,
			wantEmit:    true,
		},
		{
			name:        "drop polarity skips when object is absent",
			emitOnExist: true,
			exists:      false,
			wantProbe:   true,
		},
		{
			name:      "create polarity emits when object is absent",
			exists:    false,
			wantProbe: true,
			wantEmit:  true,
		},
		{
			name:      "create polarity skips when object exists",
			exists:    true,
			wantProbe: true,
		},
		{
			name:      "probe error recorded on plan",
			probeErr:  probeFailure,
			wantProbe: true,
			wantErr:   probeFailure,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := NewMigrationContext(nil, SQLiteDialect{}, WithDryRun(tc.dryRun))
			plan := m.Schema()
			probeCalls := 0
			emitCalls := 0

			got := plan.existenceGuard(context.Background(), tc.emitOnExist,
				func(context.Context) (bool, error) {
					probeCalls++
					return tc.exists, tc.probeErr
				},
				func() *SchemaPlan {
					emitCalls++
					return plan.Raw("GUARDED")
				})

			if got != plan {
				t.Fatal("existenceGuard returned a different plan instance")
			}
			wantProbeCalls := 0
			if tc.wantProbe {
				wantProbeCalls = 1
			}
			if probeCalls != wantProbeCalls {
				t.Fatalf("probe calls = %d, want %d", probeCalls, wantProbeCalls)
			}
			wantEmitCalls := 0
			if tc.wantEmit {
				wantEmitCalls = 1
			}
			if emitCalls != wantEmitCalls {
				t.Fatalf("emit calls = %d, want %d", emitCalls, wantEmitCalls)
			}
			if got.err != tc.wantErr {
				t.Fatalf("plan error = %v, want %v", got.err, tc.wantErr)
			}
			stmts := got.Statements()
			if tc.wantEmit {
				if len(stmts) != 1 || stmts[0].Query != "GUARDED" {
					t.Fatalf("statements = %#v, want one GUARDED statement", stmts)
				}
			} else if len(stmts) != 0 {
				t.Fatalf("statements = %#v, want none", stmts)
			}
		})
	}
}
