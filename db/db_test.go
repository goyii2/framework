package db

import (
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// sqliteDSN returns a file-backed sqlite DSN; file-backed rather than
// :memory: so every pooled connection sees the same database.
func sqliteDSN(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "test.db")
}

// tableNames returns the tables visible through a fresh connection to dsn.
func tableNames(t *testing.T, dsn string) []string {
	t.Helper()
	conn, err := New(Config{Driver: "sqlite", DSN: dsn})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	var names []string
	if err := conn.Raw("SELECT name FROM sqlite_master WHERE type = 'table' ORDER BY name").Scan(&names).Error; err != nil {
		t.Fatalf("listing tables: %v", err)
	}
	return names
}

func contains(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}

func TestNewOpensSQLiteAndRunsQueries(t *testing.T) {
	conn, err := New(Config{Driver: "sqlite", DSN: sqliteDSN(t)})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if got := conn.Driver(); got != "sqlite" {
		t.Errorf("Driver() = %q, want sqlite", got)
	}
	if err := conn.Exec("CREATE TABLE demo (id INTEGER PRIMARY KEY, name TEXT NOT NULL)").Error; err != nil {
		t.Fatalf("create table: %v", err)
	}
	if err := conn.Exec("INSERT INTO demo (name) VALUES (?)", "alice").Error; err != nil {
		t.Fatalf("insert: %v", err)
	}
	var count int64
	if err := conn.Raw("SELECT COUNT(*) FROM demo WHERE name = ?", "alice").Scan(&count).Error; err != nil {
		t.Fatalf("select: %v", err)
	}
	if count != 1 {
		t.Errorf("row count = %d, want 1", count)
	}
}

func TestCanonicalDriverDefaultsToMySQL(t *testing.T) {
	for _, name := range []string{"", "   ", "MySQL"} {
		got, err := canonicalDriver(name)
		if err != nil {
			t.Fatalf("canonicalDriver(%q) error = %v", name, err)
		}
		if got != "mysql" {
			t.Errorf("canonicalDriver(%q) = %q, want mysql", name, got)
		}
	}
}

func TestNewAcceptsDriverAliases(t *testing.T) {
	for _, name := range []string{"sqlite", "sqlite3", " SQLite3 "} {
		conn, err := New(Config{Driver: name, DSN: sqliteDSN(t)})
		if err != nil {
			t.Fatalf("New(driver=%q) error = %v", name, err)
		}
		t.Cleanup(func() { _ = conn.Close() })
		if got := conn.Driver(); got != "sqlite" {
			t.Errorf("New(driver=%q).Driver() = %q, want sqlite", name, got)
		}
	}
}

func TestNewRejectsUnknownDriver(t *testing.T) {
	_, err := New(Config{Driver: "oracle", DSN: "host=localhost"})
	if err == nil {
		t.Fatal("New(oracle) returned nil error")
	}
	for _, want := range []string{"unsupported driver", "mysql", "postgres", "sqlserver", "sqlite"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q missing %q", err, want)
		}
	}
}

func TestNewRequiresDSN(t *testing.T) {
	_, err := New(Config{Driver: "sqlite"})
	if err == nil {
		t.Fatal("New without DSN returned nil error")
	}
	if !strings.Contains(err.Error(), DSNEnvVar) {
		t.Errorf("error %q must name the %s fallback", err, DSNEnvVar)
	}
}

func TestNewDSNEnvOverridesConfiguredDSN(t *testing.T) {
	configured := sqliteDSN(t)
	fromEnv := sqliteDSN(t)

	conn, err := New(Config{Driver: "sqlite", DSN: configured})
	if err != nil {
		t.Fatalf("seeding configured database: %v", err)
	}
	if err := conn.Exec("CREATE TABLE configured_marker (id INTEGER)").Error; err != nil {
		t.Fatalf("seeding configured database: %v", err)
	}
	_ = conn.Close()

	t.Setenv(DSNEnvVar, fromEnv)
	conn, err = New(Config{Driver: "sqlite", DSN: configured})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if tables := tableNames(t, fromEnv); contains(tables, "configured_marker") {
		t.Error("connection must use the DSN from the environment, not the configured one")
	}
}

func TestComponentOpensLazilyAndCaches(t *testing.T) {
	dsn := sqliteDSN(t)
	component := NewComponent(Config{Driver: "sqlite", DSN: dsn})

	first, err := component.Get()
	if err != nil {
		t.Fatalf("first Get() error = %v", err)
	}
	// The component keeps the connection open for the application's
	// lifetime; release it here so Windows can remove the temp database.
	t.Cleanup(func() { _ = first.Close() })
	if err := first.Exec("CREATE TABLE lazy (id INTEGER)").Error; err != nil {
		t.Fatalf("create table through first connection: %v", err)
	}

	second, err := component.Get()
	if err != nil {
		t.Fatalf("second Get() error = %v", err)
	}
	if second != first {
		t.Error("repeated Get() returned a different connection")
	}
	if tables := tableNames(t, dsn); !contains(tables, "lazy") {
		t.Error("connection must point at the configured database file")
	}
}

func TestComponentRetriesAfterFailedOpen(t *testing.T) {
	component := NewComponent(Config{Driver: "sqlite"}) // no DSN

	if _, err := component.Get(); err == nil {
		t.Fatal("Get() without DSN returned nil error")
	}

	dsn := sqliteDSN(t)
	conn, err := New(Config{Driver: "sqlite", DSN: dsn})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	component.Set(conn)

	got, err := component.Get()
	if err != nil {
		t.Fatalf("Get() after Set() error = %v", err)
	}
	if got != conn {
		t.Error("Get() after Set() returned a different connection")
	}

	component.Set(nil)
	if _, err := component.Get(); err == nil {
		t.Error("Set(nil) must restore lazy initialization, which fails without a DSN")
	}
	_ = conn.Close()
}

func TestComponentConcurrentGetOpensOnce(t *testing.T) {
	component := NewComponent(Config{Driver: "sqlite", DSN: sqliteDSN(t)})

	conns := make([]*Conn, 16)
	var wg sync.WaitGroup
	for i := range conns {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			conn, err := component.Get()
			if err != nil {
				t.Errorf("Get() error = %v", err)
				return
			}
			conns[i] = conn
		}(i)
	}
	wg.Wait()

	for i, conn := range conns {
		if conn != conns[0] {
			t.Fatalf("goroutine %d saw a different connection; the open must happen once", i)
		}
	}
	_ = conns[0].Close()
}
