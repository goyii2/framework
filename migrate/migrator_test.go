package migrate

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func TestMigratorEnsureMigrationTableExecutesDialectSQL(t *testing.T) {
	state := newFakeSQLState()
	db := openFakeSQLDB(t, state)
	migrator := NewMigrator(db, MySQLDialect{}, nil)

	if err := migrator.EnsureMigrationTable(context.Background()); err != nil {
		t.Fatalf("EnsureMigrationTable() error = %v", err)
	}

	want := "CREATE TABLE IF NOT EXISTS `migration` (`version` varchar(180) NOT NULL PRIMARY KEY, `apply_time` int NOT NULL)"
	queries := state.execQueries()
	if len(queries) != 1 || queries[0] != want {
		t.Fatalf("executed SQL = %#v, want [%s]", queries, want)
	}
}

func TestMigratorEnsureMigrationTableUsesConfiguredTableName(t *testing.T) {
	state := newFakeSQLState()
	db := openFakeSQLDB(t, state)
	migrator := NewMigrator(db, SQLServerDialect{}, nil)
	migrator.MigrationTable = "custom_migration_history"

	if err := migrator.EnsureMigrationTable(context.Background()); err != nil {
		t.Fatalf("EnsureMigrationTable() error = %v", err)
	}

	want := "IF OBJECT_ID(N'[custom_migration_history]', 'U') IS NULL BEGIN CREATE TABLE [custom_migration_history] ([version] varchar(180) NOT NULL PRIMARY KEY, [apply_time] int NOT NULL) END"
	queries := state.execQueries()
	if len(queries) != 1 || queries[0] != want {
		t.Fatalf("executed SQL = %#v, want [%s]", queries, want)
	}
}

func TestMySQLDialectQuotesIdentifiers(t *testing.T) {
	dialect := MySQLDialect{}

	if got := dialect.QuoteTable("app.user"); got != "`app`.`user`" {
		t.Fatalf("QuoteTable() = %q", got)
	}
	if got := dialect.QuoteColumn("user"); got != "`user`" {
		t.Fatalf("QuoteColumn() = %q", got)
	}
}

func TestMigratorAppliedPendingAndHistory(t *testing.T) {
	state := newFakeSQLState()
	state.applied["m20260613_100000_create_user"] = 100
	state.applied["m20260613_110000_create_post"] = 110
	db := openFakeSQLDB(t, state)
	migrator := NewMigrator(db, MySQLDialect{}, []Migration{
		testMigration{name: "m20260613_110000_create_post"},
		testMigration{name: "m20260613_100000_create_user"},
		testMigration{name: "m20260613_120000_create_comment"},
	})

	applied, err := migrator.Applied(context.Background())
	if err != nil {
		t.Fatalf("Applied() error = %v", err)
	}
	if got, want := appliedVersions(applied), []string{"m20260613_100000_create_user", "m20260613_110000_create_post"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Applied() versions = %#v, want %#v", got, want)
	}

	pending, err := migrator.Pending(context.Background())
	if err != nil {
		t.Fatalf("Pending() error = %v", err)
	}
	if got, want := migrationNames(pending), []string{"m20260613_120000_create_comment"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Pending() names = %#v, want %#v", got, want)
	}

	history, err := migrator.History(context.Background(), 1)
	if err != nil {
		t.Fatalf("History() error = %v", err)
	}
	if got, want := appliedVersions(history), []string{"m20260613_110000_create_post"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("History() versions = %#v, want %#v", got, want)
	}
}

func TestMigratorUpAppliesPendingMigrationInTransaction(t *testing.T) {
	state := newFakeSQLState()
	db := openFakeSQLDB(t, state)
	first := testMigration{
		name: "m20260613_100000_create_user",
		up: func(ctx context.Context, m *MigrationContext) error {
			return m.Execute(ctx, "CREATE TABLE `user` (`id` int)")
		},
	}
	second := testMigration{name: "m20260613_110000_create_post"}
	migrator := NewMigrator(db, MySQLDialect{}, []Migration{second, first})

	applied, err := migrator.Up(context.Background(), 1)
	if err != nil {
		t.Fatalf("Up() error = %v", err)
	}
	if got, want := applied, []string{"m20260613_100000_create_user"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Up() applied = %#v, want %#v", got, want)
	}
	if _, ok := state.appliedVersion("m20260613_100000_create_user"); !ok {
		t.Fatalf("migration was not recorded as applied")
	}
	if got := state.commitCount(); got != 1 {
		t.Fatalf("commits = %d, want 1", got)
	}
	if got := state.rollbackCount(); got != 0 {
		t.Fatalf("rollbacks = %d, want 0", got)
	}
}

func TestMigratorUpRollsBackWhenMigrationFails(t *testing.T) {
	state := newFakeSQLState()
	db := openFakeSQLDB(t, state)
	boom := errors.New("boom")
	migrator := NewMigrator(db, MySQLDialect{}, []Migration{
		testMigration{
			name: "m20260613_100000_create_user",
			up: func(ctx context.Context, m *MigrationContext) error {
				if err := m.Execute(ctx, "CREATE TABLE `user` (`id` int)"); err != nil {
					return err
				}
				return boom
			},
		},
	})

	_, err := migrator.Up(context.Background(), 0)
	if !errors.Is(err, boom) {
		t.Fatalf("Up() error = %v, want %v", err, boom)
	}
	if _, ok := state.appliedVersion("m20260613_100000_create_user"); ok {
		t.Fatalf("failed migration was recorded as applied")
	}
	if got := state.commitCount(); got != 0 {
		t.Fatalf("commits = %d, want 0", got)
	}
	if got := state.rollbackCount(); got != 1 {
		t.Fatalf("rollbacks = %d, want 1", got)
	}
}

func TestMigratorDownRevertsLatestMigrationInTransaction(t *testing.T) {
	state := newFakeSQLState()
	state.applied["m20260613_100000_create_user"] = 100
	state.applied["m20260613_110000_create_post"] = 110
	db := openFakeSQLDB(t, state)
	migrator := NewMigrator(db, MySQLDialect{}, []Migration{
		testMigration{name: "m20260613_100000_create_user"},
		testMigration{
			name: "m20260613_110000_create_post",
			down: func(ctx context.Context, m *MigrationContext) error {
				return m.Execute(ctx, "DROP TABLE `post`")
			},
		},
	})

	reverted, err := migrator.Down(context.Background(), 1)
	if err != nil {
		t.Fatalf("Down() error = %v", err)
	}
	if got, want := reverted, []string{"m20260613_110000_create_post"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Down() reverted = %#v, want %#v", got, want)
	}
	if _, ok := state.appliedVersion("m20260613_110000_create_post"); ok {
		t.Fatalf("migration was not removed from applied records")
	}
	if got := state.commitCount(); got != 1 {
		t.Fatalf("commits = %d, want 1", got)
	}
}

func TestMigratorDownReturnsMigrationNotFound(t *testing.T) {
	state := newFakeSQLState()
	state.applied["m20260613_100000_missing"] = 100
	db := openFakeSQLDB(t, state)
	migrator := NewMigrator(db, MySQLDialect{}, nil)

	_, err := migrator.Down(context.Background(), 1)
	if !errors.Is(err, ErrMigrationNotFound) {
		t.Fatalf("Down() error = %v, want ErrMigrationNotFound", err)
	}
}

func TestMigratorUsesLockAroundUp(t *testing.T) {
	state := newFakeSQLState()
	db := openFakeSQLDB(t, state)
	migrator := NewMigrator(db, MySQLDialect{}, []Migration{
		testMigration{name: "m20260613_100000_create_user"},
	})
	migrator.LockName = "test-lock"

	_, err := migrator.Up(context.Background(), 1)
	if err != nil {
		t.Fatalf("Up() error = %v", err)
	}

	if got, want := state.lockQueries(), []string{"SELECT GET_LOCK(?, ?)", "SELECT RELEASE_LOCK(?)"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("lock queries = %#v, want %#v", got, want)
	}
}

func TestMigratorDryRunSkipsLockAndExecution(t *testing.T) {
	state := newFakeSQLState()
	db := openFakeSQLDB(t, state)
	migrator := NewMigrator(db, MySQLDialect{}, []Migration{
		testMigration{
			name: "m20260613_100000_create_user",
			up: func(ctx context.Context, m *MigrationContext) error {
				return m.Schema().Raw("CREATE TABLE `user` (`id` int)").Exec(ctx)
			},
		},
	})
	migrator.DryRun = true

	applied, err := migrator.Up(context.Background(), 1)
	if err != nil {
		t.Fatalf("Up() error = %v", err)
	}
	if got, want := applied, []string{"m20260613_100000_create_user"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("dry-run applied = %#v, want %#v", got, want)
	}
	if got := state.lockQueries(); len(got) != 0 {
		t.Fatalf("dry-run used locks: %#v", got)
	}
	if got := state.execQueries(); len(got) != 0 {
		t.Fatalf("dry-run executed SQL: %#v", got)
	}
}

func TestMigratorRedoToMarkAndNew(t *testing.T) {
	state := newFakeSQLState()
	state.applied["m20260613_100000_create_user"] = 100
	state.applied["m20260613_110000_create_post"] = 110
	db := openFakeSQLDB(t, state)
	migrator := NewMigrator(db, MySQLDialect{}, []Migration{
		testMigration{name: "m20260613_100000_create_user"},
		testMigration{name: "m20260613_110000_create_post"},
		testMigration{name: "m20260613_120000_create_comment"},
	})

	pending, err := migrator.New(context.Background(), 1)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if got, want := migrationNames(pending), []string{"m20260613_120000_create_comment"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("New() = %#v, want %#v", got, want)
	}

	reverted, applied, err := migrator.Redo(context.Background(), 1)
	if err != nil {
		t.Fatalf("Redo() error = %v", err)
	}
	if !reflect.DeepEqual(reverted, []string{"m20260613_110000_create_post"}) || !reflect.DeepEqual(applied, []string{"m20260613_110000_create_post"}) {
		t.Fatalf("Redo() reverted=%#v applied=%#v", reverted, applied)
	}

	if err := migrator.To(context.Background(), "m20260613_120000_create_comment"); err != nil {
		t.Fatalf("To() error = %v", err)
	}
	if _, ok := state.appliedVersion("m20260613_120000_create_comment"); !ok {
		t.Fatalf("To() did not apply target migration")
	}

	if err := migrator.Mark(context.Background(), "m20260613_100000_create_user"); err != nil {
		t.Fatalf("Mark() error = %v", err)
	}
	if _, ok := state.appliedVersion("m20260613_110000_create_post"); ok {
		t.Fatalf("Mark() kept migration above target applied")
	}
	if _, ok := state.appliedVersion("m20260613_100000_create_user"); !ok {
		t.Fatalf("Mark() removed target migration")
	}
}

func appliedVersions(applied []AppliedMigration) []string {
	versions := make([]string, 0, len(applied))
	for _, migration := range applied {
		versions = append(versions, migration.Version)
	}
	return versions
}

func migrationNames(migrations []Migration) []string {
	names := make([]string, 0, len(migrations))
	for _, migration := range migrations {
		names = append(names, migration.Name())
	}
	return names
}

type testMigration struct {
	name string
	up   func(context.Context, *MigrationContext) error
	down func(context.Context, *MigrationContext) error
}

func (m testMigration) Name() string {
	return m.name
}

func (m testMigration) Up(ctx context.Context, migration *MigrationContext) error {
	if m.up == nil {
		return nil
	}
	return m.up(ctx, migration)
}

func (m testMigration) Down(ctx context.Context, migration *MigrationContext) error {
	if m.down == nil {
		return nil
	}
	return m.down(ctx, migration)
}

var (
	fakeSQLDriverOnce sync.Once
	fakeSQLStates     sync.Map
	fakeSQLStateID    atomic.Int64
)

func openFakeSQLDB(t *testing.T, state *fakeSQLState) *sql.DB {
	t.Helper()
	fakeSQLDriverOnce.Do(func() {
		sql.Register("yiimigrate_fake_sql", fakeSQLDriver{})
	})

	name := fmt.Sprintf("state-%d", fakeSQLStateID.Add(1))
	fakeSQLStates.Store(name, state)
	t.Cleanup(func() {
		fakeSQLStates.Delete(name)
	})

	db, err := sql.Open("yiimigrate_fake_sql", name)
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})
	return db
}

type fakeSQLState struct {
	mu          sync.Mutex
	applied     map[string]int64
	execs       []fakeExecRecord
	lockQuery   []string
	tables      map[string]bool
	columns     map[string]bool
	indexes     map[string]bool
	foreignKeys map[string]bool
	constraints map[string]bool
	rows        map[string]fakeQueryResult
	rowExists   map[string]bool
	countRows   map[string]int64
	queryErr    error
	lockResult  int64
	commits     int
	rollbacks   int
}

func newFakeSQLState() *fakeSQLState {
	return &fakeSQLState{
		applied:     map[string]int64{},
		tables:      map[string]bool{},
		columns:     map[string]bool{},
		indexes:     map[string]bool{},
		foreignKeys: map[string]bool{},
		constraints: map[string]bool{},
		rows:        map[string]fakeQueryResult{},
		rowExists:   map[string]bool{},
		countRows:   map[string]int64{},
		lockResult:  1,
	}
}

func (s *fakeSQLState) appliedVersion(version string) (int64, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	applyTime, ok := s.applied[version]
	return applyTime, ok
}

func (s *fakeSQLState) commitCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.commits
}

func (s *fakeSQLState) rollbackCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.rollbacks
}

func (s *fakeSQLState) execQueries() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	queries := make([]string, 0, len(s.execs))
	for _, record := range s.execs {
		queries = append(queries, record.query)
	}
	return queries
}

func (s *fakeSQLState) lockQueries() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.lockQuery...)
}

type fakeExecRecord struct {
	query string
	args  []any
}

type fakeQueryResult struct {
	columns []string
	values  [][]any
}

type fakeSQLDriver struct{}

func (fakeSQLDriver) Open(name string) (driver.Conn, error) {
	value, ok := fakeSQLStates.Load(name)
	if !ok {
		return nil, fmt.Errorf("unknown fake SQL state %q", name)
	}
	return &fakeSQLConn{state: value.(*fakeSQLState)}, nil
}

type fakeSQLConn struct {
	state *fakeSQLState
	tx    *fakeSQLTx
}

func (c *fakeSQLConn) Prepare(query string) (driver.Stmt, error) {
	return nil, errors.New("prepared statements are not implemented by fakeSQLConn")
}

func (c *fakeSQLConn) Close() error {
	return nil
}

func (c *fakeSQLConn) Begin() (driver.Tx, error) {
	return c.BeginTx(context.Background(), driver.TxOptions{})
}

func (c *fakeSQLConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	c.tx = &fakeSQLTx{
		conn:          c,
		pendingApply:  map[string]int64{},
		pendingDelete: map[string]struct{}{},
	}
	return c.tx, nil
}

func (c *fakeSQLConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	c.state.mu.Lock()
	defer c.state.mu.Unlock()

	c.state.execs = append(c.state.execs, fakeExecRecord{query: query, args: namedValuesToAny(args)})
	if c.tx != nil {
		c.tx.record(query, args)
		return driver.RowsAffected(1), nil
	}
	recordMigrationExec(c.state.applied, query, args)
	recordMigrationDelete(c.state.applied, query, args)
	return driver.RowsAffected(1), nil
}

func (c *fakeSQLConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	c.state.mu.Lock()
	defer c.state.mu.Unlock()

	if c.state.queryErr != nil {
		return nil, c.state.queryErr
	}
	upper := strings.ToUpper(strings.TrimSpace(query))
	if strings.HasPrefix(upper, "SELECT GET_LOCK") {
		c.state.lockQuery = append(c.state.lockQuery, "SELECT GET_LOCK(?, ?)")
		return fakeRows([]string{"GET_LOCK"}, [][]any{{c.state.lockResult}}), nil
	}
	if strings.HasPrefix(upper, "SELECT RELEASE_LOCK") {
		c.state.lockQuery = append(c.state.lockQuery, "SELECT RELEASE_LOCK(?)")
		return fakeRows([]string{"RELEASE_LOCK"}, [][]any{{int64(1)}}), nil
	}
	if strings.Contains(upper, "INFORMATION_SCHEMA.TABLES") {
		return fakeRows([]string{"count"}, [][]any{{boolCount(c.state.tables[fmt.Sprint(args[0].Value)])}}), nil
	}
	if strings.Contains(upper, "INFORMATION_SCHEMA.COLUMNS") {
		key := fmt.Sprint(args[0].Value) + "." + fmt.Sprint(args[1].Value)
		return fakeRows([]string{"count"}, [][]any{{boolCount(c.state.columns[key])}}), nil
	}
	if strings.Contains(upper, "INFORMATION_SCHEMA.STATISTICS") {
		key := fmt.Sprint(args[0].Value) + "." + fmt.Sprint(args[1].Value)
		return fakeRows([]string{"count"}, [][]any{{boolCount(c.state.indexes[key])}}), nil
	}
	if strings.Contains(upper, "INFORMATION_SCHEMA.TABLE_CONSTRAINTS") && strings.Contains(upper, "FOREIGN KEY") {
		key := fmt.Sprint(args[0].Value) + "." + fmt.Sprint(args[1].Value)
		return fakeRows([]string{"count"}, [][]any{{boolCount(c.state.foreignKeys[key])}}), nil
	}
	if strings.Contains(upper, "INFORMATION_SCHEMA.TABLE_CONSTRAINTS") {
		key := fmt.Sprint(args[0].Value) + "." + fmt.Sprint(args[1].Value)
		return fakeRows([]string{"count"}, [][]any{{boolCount(c.state.constraints[key])}}), nil
	}
	if strings.HasPrefix(upper, "SELECT EXISTS(") {
		return fakeRows([]string{"exists"}, [][]any{{boolCount(c.lookupRowExists(query))}}), nil
	}
	if strings.HasPrefix(upper, "SELECT COUNT(*) FROM `") {
		return fakeRows([]string{"count"}, [][]any{{c.lookupCountRows(query)}}), nil
	}
	if result, ok := c.state.rows[query]; ok {
		return fakeRows(result.columns, result.values), nil
	}
	if !strings.Contains(query, "FROM `migration`") {
		return fakeRows(nil, nil), nil
	}

	desc := strings.Contains(strings.ToUpper(query), "DESC")
	values := make([][]driver.Value, 0, len(c.state.applied))
	for version, applyTime := range c.state.applied {
		values = append(values, []driver.Value{version, applyTime})
	}
	sort.Slice(values, func(i, j int) bool {
		if values[i][1].(int64) == values[j][1].(int64) {
			if desc {
				return values[i][0].(string) > values[j][0].(string)
			}
			return values[i][0].(string) < values[j][0].(string)
		}
		if desc {
			return values[i][1].(int64) > values[j][1].(int64)
		}
		return values[i][1].(int64) < values[j][1].(int64)
	})
	return &fakeSQLRows{columns: []string{"version", "apply_time"}, values: values}, nil
}

func (c *fakeSQLConn) lookupRowExists(query string) bool {
	for key, exists := range c.state.rowExists {
		parts := strings.SplitN(key, ".", 2)
		if len(parts) != 2 {
			continue
		}
		if strings.Contains(query, "`"+parts[0]+"`") && strings.Contains(query, parts[1]) {
			return exists
		}
	}
	return false
}

func (c *fakeSQLConn) lookupCountRows(query string) int64 {
	for key, count := range c.state.countRows {
		parts := strings.SplitN(key, ".", 2)
		if len(parts) != 2 {
			continue
		}
		if strings.Contains(query, "`"+parts[0]+"`") && strings.Contains(query, parts[1]) {
			return count
		}
	}
	return 0
}

type fakeSQLTx struct {
	conn          *fakeSQLConn
	pendingApply  map[string]int64
	pendingDelete map[string]struct{}
}

func (tx *fakeSQLTx) record(query string, args []driver.NamedValue) {
	recordMigrationExec(tx.pendingApply, query, args)
	if isDeleteMigrationQuery(query) && len(args) > 0 {
		tx.pendingDelete[fmt.Sprint(args[0].Value)] = struct{}{}
	}
}

func (tx *fakeSQLTx) Commit() error {
	tx.conn.state.mu.Lock()
	defer tx.conn.state.mu.Unlock()

	for version, applyTime := range tx.pendingApply {
		tx.conn.state.applied[version] = applyTime
	}
	for version := range tx.pendingDelete {
		delete(tx.conn.state.applied, version)
	}
	tx.conn.state.commits++
	tx.conn.tx = nil
	return nil
}

func (tx *fakeSQLTx) Rollback() error {
	tx.conn.state.mu.Lock()
	defer tx.conn.state.mu.Unlock()

	tx.conn.state.rollbacks++
	tx.conn.tx = nil
	return nil
}

type fakeSQLRows struct {
	columns []string
	values  [][]driver.Value
	index   int
}

func (r *fakeSQLRows) Columns() []string {
	return r.columns
}

func (r *fakeSQLRows) Close() error {
	return nil
}

func (r *fakeSQLRows) Next(dest []driver.Value) error {
	if r.index >= len(r.values) {
		return io.EOF
	}
	copy(dest, r.values[r.index])
	r.index++
	return nil
}

func fakeRows(columns []string, values [][]any) driver.Rows {
	driverValues := make([][]driver.Value, 0, len(values))
	for _, row := range values {
		driverRow := make([]driver.Value, 0, len(row))
		for _, value := range row {
			driverRow = append(driverRow, driver.Value(value))
		}
		driverValues = append(driverValues, driverRow)
	}
	return &fakeSQLRows{columns: columns, values: driverValues}
}

func recordMigrationExec(applied map[string]int64, query string, args []driver.NamedValue) {
	if !isInsertMigrationQuery(query) || len(args) < 2 {
		return
	}
	version := fmt.Sprint(args[0].Value)
	applyTime, ok := args[1].Value.(int64)
	if !ok {
		applyTime = 0
	}
	applied[version] = applyTime
}

func recordMigrationDelete(applied map[string]int64, query string, args []driver.NamedValue) {
	if !isDeleteMigrationQuery(query) || len(args) == 0 {
		return
	}
	delete(applied, fmt.Sprint(args[0].Value))
}

func isInsertMigrationQuery(query string) bool {
	return strings.HasPrefix(strings.ToUpper(strings.TrimSpace(query)), "INSERT INTO")
}

func isDeleteMigrationQuery(query string) bool {
	return strings.HasPrefix(strings.ToUpper(strings.TrimSpace(query)), "DELETE FROM")
}

func namedValuesToAny(values []driver.NamedValue) []any {
	out := make([]any, 0, len(values))
	for _, value := range values {
		out = append(out, value.Value)
	}
	return out
}

func boolCount(ok bool) int64 {
	if ok {
		return 1
	}
	return 0
}
