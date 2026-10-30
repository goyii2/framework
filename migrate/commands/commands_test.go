package commands

import (
	"context"
	"database/sql"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/goyii2/framework/base"
	"github.com/goyii2/framework/console"
	"github.com/goyii2/framework/db"
	"github.com/goyii2/framework/migrate"
)

// tableMigration creates and drops one table; it mirrors what a generated
// migration file compiles to.
type tableMigration struct {
	name  string
	table string
}

func (m tableMigration) Name() string { return m.name }

func (m tableMigration) Up(ctx context.Context, mc *migrate.MigrationContext) error {
	return mc.Execute(ctx, `CREATE TABLE "`+m.table+`" (id INTEGER PRIMARY KEY, name TEXT NOT NULL)`)
}

func (m tableMigration) Down(ctx context.Context, mc *migrate.MigrationContext) error {
	return mc.Execute(ctx, `DROP TABLE "`+m.table+`"`)
}

var flowMigrations = []migrate.Migration{
	tableMigration{name: "m20260101_000001_create_first_table", table: "first"},
	tableMigration{name: "m20260101_000002_create_second_table", table: "second"},
}

// newConsole builds a console with the migrate command tree attached over a
// sqlite-backed application (the connection DML shares) plus any named
// targets; run returns the command's error and the captured output.
func newConsole(t *testing.T, dsn string, migrations []migrate.Migration, targets ...Target) (*console.Console, *strings.Builder) {
	t.Helper()
	app := base.New()
	conn := sqliteConn(t, dsn)
	app.SetDB(conn)
	var out strings.Builder
	c := console.New(app).AddCommand(New(app, migrations, targets...))
	c.Command().SetOut(&out)
	c.Command().SetErr(&out)
	return c, &out
}

// sqliteConn opens a throwaway sqlite connection the test closes on cleanup.
func sqliteConn(t *testing.T, dsn string) *db.Conn {
	t.Helper()
	conn, err := db.New(db.Config{Driver: "sqlite", DSN: dsn})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// runMigrate runs one migrate invocation as if it were a fresh process.
// cobra keeps previously parsed flag values on a reused command tree, so the
// --db flag resets before every call — otherwise an earlier --db=stats run
// would keep routing a later plain "up" to the stats target.
func runMigrate(t *testing.T, c *console.Console, args ...string) error {
	t.Helper()
	root := c.Command()
	for _, cmd := range root.Commands() {
		if cmd.Name() == "migrate" {
			if f := cmd.PersistentFlags().Lookup("db"); f != nil {
				_ = f.Value.Set("")
			}
		}
	}
	root.SetArgs(append([]string{"migrate"}, args...))
	return root.Execute()
}

// querySQLite runs one scalar query against the sqlite file behind dsn.
func querySQLite(t *testing.T, dsn, query string, args ...any) int64 {
	t.Helper()
	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	defer conn.Close()
	var value int64
	if err := conn.QueryRowContext(context.Background(), query, args...).Scan(&value); err != nil {
		t.Fatalf("query %q: %v", query, err)
	}
	return value
}

func tableCount(t *testing.T, dsn, table string) int64 {
	return querySQLite(t, dsn, "SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?", table)
}

func appliedCount(t *testing.T, dsn string) int64 {
	return querySQLite(t, dsn, `SELECT COUNT(*) FROM "migration"`)
}

func sqliteDSN(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "migrate.db")
}

func TestResolveDriverDefaultsToMySQL(t *testing.T) {
	driverName, dialect, err := resolveDriver("")
	if err != nil {
		t.Fatalf("resolveDriver(\"\") error = %v", err)
	}
	if driverName != "mysql" {
		t.Fatalf("driverName = %q, want mysql", driverName)
	}
	if _, ok := dialect.(migrate.MySQLDialect); !ok {
		t.Fatalf("dialect = %T, want migrate.MySQLDialect", dialect)
	}
}

func TestResolveDriverSupportsAliases(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"sqlite", "sqlite"},
		{"sqlite3", "sqlite"},
		{" SQLite3 ", "sqlite"},
		{"postgres", "postgres"},
		{"postgresql", "postgres"},
		{" PostgreSQL ", "postgres"},
		{"sqlserver", "sqlserver"},
		{"mssql", "sqlserver"},
		{" MSSQL ", "sqlserver"},
	}
	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			driverName, dialect, err := resolveDriver(test.input)
			if err != nil {
				t.Fatalf("resolveDriver(%q) error = %v", test.input, err)
			}
			if driverName != test.want {
				t.Fatalf("resolveDriver(%q) driverName = %q, want %q", test.input, driverName, test.want)
			}
			if dialect == nil || dialect.Name() != test.want {
				t.Fatalf("resolveDriver(%q) dialect = %v, want registered %q dialect", test.input, dialect, test.want)
			}
		})
	}
}

func TestResolveDriverRejectsUnsupportedDialect(t *testing.T) {
	_, _, err := resolveDriver("oracle")
	if err == nil {
		t.Fatal("resolveDriver(oracle) returned nil error")
	}
	if !strings.Contains(err.Error(), `unsupported db dialect "oracle"`) {
		t.Fatalf("error = %v", err)
	}
	for _, name := range migrate.DialectNames() {
		if !strings.Contains(err.Error(), name) {
			t.Fatalf("error does not list supported dialect %q: %v", name, err)
		}
	}
}

// Every dialect the migrate registry exposes must resolve to a database/sql
// driver through the commands, so the registry can grow without silently
// breaking command wiring.
func TestResolveDriverSupportsEveryRegisteredDialect(t *testing.T) {
	for _, name := range migrate.DialectNames() {
		t.Run(name, func(t *testing.T) {
			driverName, dialect, err := resolveDriver(name)
			if err != nil {
				t.Fatalf("resolveDriver(%q) error = %v", name, err)
			}
			if driverName == "" {
				t.Fatalf("resolveDriver(%q) returned empty driver name", name)
			}
			if dialect == nil || dialect.Name() != name {
				t.Fatalf("resolveDriver(%q) dialect = %v, want registered %q dialect", name, dialect, name)
			}
		})
	}
}

func TestDialectLockableFollowsDialectLockSupport(t *testing.T) {
	if !dialectLockable(migrate.MySQLDialect{}) {
		t.Error("mysql must run commands under its advisory lock")
	}
	if dialectLockable(migrate.SQLiteDialect{}) {
		t.Error("sqlite has no advisory lock; commands must run without the lock step")
	}
}

func TestOptionalIntFallsBackOnMissingOrInvalidArg(t *testing.T) {
	if got := optionalInt(nil, 7); got != 7 {
		t.Errorf("optionalInt(nil, 7) = %d, want 7", got)
	}
	if got := optionalInt([]string{"not-a-number"}, 7); got != 7 {
		t.Errorf("optionalInt(not-a-number, 7) = %d, want 7", got)
	}
	if got := optionalInt([]string{"3"}, 7); got != 3 {
		t.Errorf("optionalInt(3, 7) = %d, want 3", got)
	}
}

func TestMigrateCommandTreeExposesEightSubcommands(t *testing.T) {
	c, _ := newConsole(t, sqliteDSN(t), nil)
	root := c.Command()
	root.SetArgs([]string{"migrate", "--help"})
	if err := root.Execute(); err != nil {
		t.Fatalf("migrate --help error = %v", err)
	}

	var migrateCmd *cobra.Command
	for _, cmd := range root.Commands() {
		if cmd.Name() == "migrate" {
			migrateCmd = cmd
		}
	}
	if migrateCmd == nil {
		t.Fatal("migrate command is not attached below the console root command")
	}
	subs := map[string]bool{}
	for _, cmd := range migrateCmd.Commands() {
		subs[cmd.Name()] = true
	}
	for _, name := range []string{"up", "down", "redo", "history", "new", "mark", "to", "create"} {
		if !subs[name] {
			t.Errorf("subcommand migrate %s is missing", name)
		}
	}
}

// TestSQLiteCreateUpDownRedoFlow is the ticket's end-to-end acceptance: the
// create command generates a migration file, then the database commands run
// the full up -> down -> redo cycle against a real SQLite database.
func TestSQLiteCreateUpDownRedoFlow(t *testing.T) {
	dsn := sqliteDSN(t)
	c, out := newConsole(t, dsn, flowMigrations)

	// create: generates a parseable migration template targeting the
	// configured sqlite dialect into the working directory's migrations dir.
	dir := t.TempDir()
	t.Chdir(dir)
	if err := runMigrate(t, c, "create", "create_post_table", "--fields=title:string(128):notNull"); err != nil {
		t.Fatalf("migrate create error = %v", err)
	}
	if !strings.Contains(out.String(), "created migrations"+string(filepath.Separator)) {
		t.Fatalf("create output %q must report the generated file path", out.String())
	}
	entries, err := os.ReadDir(filepath.Join(dir, "migrations"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("reading generated migrations dir: %v (%d entries)", err, len(entries))
	}
	generated, err := os.ReadFile(filepath.Join(dir, "migrations", entries[0].Name()))
	if err != nil {
		t.Fatalf("reading generated file: %v", err)
	}
	content := string(generated)
	for _, want := range []string{
		"package migrations",
		`"github.com/goyii2/framework/migrate"`,
		`Add("id", m.PrimaryKey()).`,
		`Add("title", m.String(128).NotNull())`,
		`CreateTable("post", migrate.Columns().`,
		`DropTable("post").`,
	} {
		if !strings.Contains(content, want) {
			t.Errorf("generated migration missing %q:\n%s", want, content)
		}
	}
	if _, err := parser.ParseFile(token.NewFileSet(), entries[0].Name(), generated, parser.ParseComments); err != nil {
		t.Fatalf("generated migration does not parse: %v", err)
	}

	// up: both registered migrations apply.
	if err := runMigrate(t, c, "up"); err != nil {
		t.Fatalf("migrate up error = %v", err)
	}
	if got := tableCount(t, dsn, "first"); got != 1 {
		t.Fatalf("table first after up: %d rows in sqlite_master, want 1", got)
	}
	if got := tableCount(t, dsn, "second"); got != 1 {
		t.Fatalf("table second after up: %d rows in sqlite_master, want 1", got)
	}
	if got := appliedCount(t, dsn); got != 2 {
		t.Fatalf("migration history after up: %d rows, want 2", got)
	}

	// down: reverts the most recent migration only (default limit 1).
	if err := runMigrate(t, c, "down"); err != nil {
		t.Fatalf("migrate down error = %v", err)
	}
	if got := tableCount(t, dsn, "second"); got != 0 {
		t.Fatalf("table second after down: %d rows in sqlite_master, want 0", got)
	}
	if got := tableCount(t, dsn, "first"); got != 1 {
		t.Fatalf("table first must survive down 1: %d rows in sqlite_master, want 1", got)
	}
	if got := appliedCount(t, dsn); got != 1 {
		t.Fatalf("migration history after down: %d rows, want 1", got)
	}

	// up: the reverted migration is pending again and re-applies.
	if err := runMigrate(t, c, "up"); err != nil {
		t.Fatalf("migrate up after down error = %v", err)
	}
	if got := tableCount(t, dsn, "second"); got != 1 {
		t.Fatalf("table second after re-up: %d rows in sqlite_master, want 1", got)
	}

	// redo: reverts and re-applies the most recent migration; the table is
	// dropped and recreated, and the history ends up unchanged.
	if err := runMigrate(t, c, "redo"); err != nil {
		t.Fatalf("migrate redo error = %v", err)
	}
	if got := tableCount(t, dsn, "second"); got != 1 {
		t.Fatalf("table second after redo: %d rows in sqlite_master, want 1", got)
	}
	if got := tableCount(t, dsn, "first"); got != 1 {
		t.Fatalf("table first after redo: %d rows in sqlite_master, want 1", got)
	}
	if got := appliedCount(t, dsn); got != 2 {
		t.Fatalf("migration history after redo: %d rows, want 2", got)
	}
}

func TestMigrateHistoryNewToMarkSemantics(t *testing.T) {
	dsn := sqliteDSN(t)
	c, out := newConsole(t, dsn, flowMigrations)

	// new lists both pending migrations before anything is applied.
	if err := runMigrate(t, c, "new"); err != nil {
		t.Fatalf("migrate new error = %v", err)
	}
	for _, name := range []string{flowMigrations[0].Name(), flowMigrations[1].Name()} {
		if !strings.Contains(out.String(), name) {
			t.Fatalf("new output %q must list pending migration %q", out.String(), name)
		}
	}
	out.Reset()

	if err := runMigrate(t, c, "up"); err != nil {
		t.Fatalf("migrate up error = %v", err)
	}

	// history lists applied migrations newest first.
	if err := runMigrate(t, c, "history"); err != nil {
		t.Fatalf("migrate history error = %v", err)
	}
	history := out.String()
	firstLine := strings.SplitN(strings.TrimSpace(history), "\n", 2)[0]
	if !strings.HasPrefix(firstLine, flowMigrations[1].Name()) {
		t.Fatalf("history output %q must start with the newest migration", history)
	}
	out.Reset()

	// to rolls back down to the given version.
	if err := runMigrate(t, c, "to", flowMigrations[0].Name()); err != nil {
		t.Fatalf("migrate to error = %v", err)
	}
	if got := appliedCount(t, dsn); got != 1 {
		t.Fatalf("migration history after to: %d rows, want 1", got)
	}
	if got := tableCount(t, dsn, "second"); got != 0 {
		t.Fatalf("table second after to: %d rows in sqlite_master, want 0", got)
	}

	// mark re-marks history up to the version without executing migrations.
	if err := runMigrate(t, c, "mark", flowMigrations[1].Name()); err != nil {
		t.Fatalf("migrate mark error = %v", err)
	}
	if got := appliedCount(t, dsn); got != 2 {
		t.Fatalf("migration history after mark: %d rows, want 2", got)
	}
	if got := tableCount(t, dsn, "second"); got != 0 {
		t.Fatalf("mark must not execute migrations: table second = %d rows in sqlite_master, want 0", got)
	}

	// new is empty once everything is marked applied.
	if err := runMigrate(t, c, "new"); err != nil {
		t.Fatalf("migrate new error = %v", err)
	}
	if strings.TrimSpace(out.String()) != "" {
		t.Fatalf("new output after full apply = %q, want empty", out.String())
	}
}

func TestMigrateDryRunExecutesNothing(t *testing.T) {
	dsn := sqliteDSN(t)
	c, _ := newConsole(t, dsn, flowMigrations)

	t.Setenv("MIGRATE_DRY_RUN", "1")
	if err := runMigrate(t, c, "up"); err != nil {
		t.Fatalf("migrate up (dry-run) error = %v", err)
	}

	// Dry-run prints SQL without executing it: no user tables and no
	// migration history may exist afterwards.
	if got := tableCount(t, dsn, "first"); got != 0 {
		t.Errorf("dry-run must not create table first: %d rows in sqlite_master", got)
	}
	if got := tableCount(t, dsn, "migration"); got != 0 {
		t.Errorf("dry-run must not create the history table: %d rows in sqlite_master", got)
	}
}

func TestMigrateDSNFromEnvironment(t *testing.T) {
	// The application entry opens the connection with no DSN of its own; the
	// DB_DSN environment variable (kept from the standalone yiimigrate CLI)
	// must supply it — db.New applies the override at open time.
	dsn := sqliteDSN(t)
	app := base.New()
	t.Setenv("DB_DSN", dsn)
	conn, err := db.New(db.Config{Driver: "sqlite"})
	if err != nil {
		t.Fatalf("open with DB_DSN: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	app.SetDB(conn)

	c := console.New(app).AddCommand(New(app, flowMigrations))
	c.Command().SetArgs([]string{"migrate", "up"})
	if err := c.Command().Execute(); err != nil {
		t.Fatalf("migrate up with DB_DSN error: %v", err)
	}
	if got := appliedCount(t, dsn); got != 2 {
		t.Fatalf("migration history after up: %d rows, want 2", got)
	}
}

// TestMigrateWithoutDBPanics verifies the database commands treat a missing
// db component as the programmer error it is: the panic names the wiring
// step.
func TestMigrateWithoutDBPanics(t *testing.T) {
	app := base.New()
	c := console.New(app).AddCommand(New(app, flowMigrations))
	c.Command().SetArgs([]string{"migrate", "up"})
	c.Command().SetErr(&strings.Builder{})

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("migrate up without a db component must panic")
		}
		msg, _ := r.(string)
		if !strings.Contains(msg, "SetDB") {
			t.Errorf("panic message = %q, want it to name SetDB", msg)
		}
	}()
	_ = c.Command().Execute()
}

func TestDefaultCreateDialectFollowsConnectionAndEnvironment(t *testing.T) {
	app := base.New()
	conn, err := db.New(db.Config{Driver: "sqlite", DSN: sqliteDSN(t)})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	app.SetDB(conn)
	if got := defaultCreateDialect(app); got != "sqlite" {
		t.Fatalf("defaultCreateDialect() = %q, want the connection's sqlite", got)
	}

	t.Setenv("DB_DIALECT", " sqlserver ")
	if got := defaultCreateDialect(app); got != "sqlserver" {
		t.Fatalf("defaultCreateDialect() with DB_DIALECT = %q, want sqlserver", got)
	}
}

// TestMigrateNamedTargetRunsOnItsOwnDatabase is the --db acceptance: the
// named target's migrations apply to its connection only, and the primary
// database keeps working untouched afterwards.
func TestMigrateNamedTargetRunsOnItsOwnDatabase(t *testing.T) {
	primaryDSN, statsDSN := sqliteDSN(t), sqliteDSN(t)
	statsMigrations := []migrate.Migration{
		tableMigration{name: "m20260101_000001_create_stats_table", table: "stats_only"},
	}
	c, out := newConsole(t, primaryDSN, flowMigrations, Target{
		Name:       "stats",
		Conn:       sqliteConn(t, statsDSN),
		Migrations: statsMigrations,
	})

	if err := runMigrate(t, c, "--db=stats", "up"); err != nil {
		t.Fatalf("migrate --db=stats up error = %v", err)
	}
	if got := tableCount(t, statsDSN, "stats_only"); got != 1 {
		t.Fatalf("stats target after up: table stats_only = %d rows in sqlite_master, want 1", got)
	}
	if got := appliedCount(t, statsDSN); got != 1 {
		t.Fatalf("stats target history after up: %d rows, want 1", got)
	}
	// The primary database is untouched by the --db=stats run: no history
	// table was even created there.
	if got := tableCount(t, primaryDSN, "migration"); got != 0 {
		t.Fatalf("primary after --db=stats up: history table = %d rows in sqlite_master, want 0", got)
	}
	if got := tableCount(t, primaryDSN, "first"); got != 0 {
		t.Fatalf("primary after --db=stats up: table first = %d rows in sqlite_master, want 0", got)
	}

	out.Reset()
	if err := runMigrate(t, c, "--db=stats", "history"); err != nil {
		t.Fatalf("migrate --db=stats history error = %v", err)
	}
	if !strings.Contains(out.String(), statsMigrations[0].Name()) {
		t.Fatalf("stats history output %q must list the applied migration", out.String())
	}

	// The default (no --db) still runs the primary list on the primary
	// connection, target and primary side by side.
	if err := runMigrate(t, c, "up"); err != nil {
		t.Fatalf("migrate up error = %v", err)
	}
	if got := appliedCount(t, primaryDSN); got != 2 {
		t.Fatalf("primary history after up: %d rows, want 2", got)
	}
	if got := tableCount(t, primaryDSN, "first"); got != 1 {
		t.Fatalf("primary table first after up: %d rows in sqlite_master, want 1", got)
	}
}

func TestMigrateUnknownTargetListsRegisteredNames(t *testing.T) {
	c, _ := newConsole(t, sqliteDSN(t), flowMigrations, Target{
		Name:       "stats",
		Conn:       sqliteConn(t, sqliteDSN(t)),
		Migrations: flowMigrations[:1],
	})

	err := runMigrate(t, c, "--db=analytics", "up")
	if err == nil {
		t.Fatal("migrate --db=analytics must fail: no such target")
	}
	for _, want := range []string{"analytics", "stats"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q must mention %q", err, want)
		}
	}
}

func TestMigrateDuplicateTargetPanicsAtAssembly(t *testing.T) {
	app := base.New()
	conn := sqliteConn(t, sqliteDSN(t))
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("a duplicate target name must panic at assembly time")
		}
		msg, _ := r.(string)
		if !strings.Contains(msg, "stats") {
			t.Errorf("panic message = %q, want it to name the duplicated target", msg)
		}
	}()
	New(app, flowMigrations,
		Target{Name: "stats", Conn: conn, Migrations: flowMigrations[:1]},
		Target{Name: "stats", Conn: conn, Migrations: flowMigrations[:1]},
	)
}
