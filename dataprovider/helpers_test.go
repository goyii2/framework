package dataprovider

import (
	"context"
	"sync"
	"testing"
	"time"

	gormlogger "gorm.io/gorm/logger"

	"github.com/goyii2/framework/base"
	"github.com/goyii2/framework/config"
	"github.com/goyii2/framework/db"
	"github.com/goyii2/framework/db/ar"
)

// member is the Active Record test model: table members, plain columns the
// sort whitelist names.
type member struct {
	ar.Model
	ID     int64  `gorm:"column:id;primaryKey"`
	Name   string `gorm:"column:name"`
	Status int    `gorm:"column:status"`
}

func (member) TableName() string { return "members" }

// sqlRecorder captures every statement GORM executes, so tests can assert
// what actually went down to SQL (one count, one paged select).
type sqlRecorder struct {
	mu         sync.Mutex
	statements []string
}

func (r *sqlRecorder) LogMode(gormlogger.LogLevel) gormlogger.Interface { return r }
func (r *sqlRecorder) Info(context.Context, string, ...any)             {}
func (r *sqlRecorder) Warn(context.Context, string, ...any)             {}
func (r *sqlRecorder) Error(context.Context, string, ...any)            {}

func (r *sqlRecorder) Trace(_ context.Context, _ time.Time, fc func() (string, int64), _ error) {
	sql, _ := fc()
	r.mu.Lock()
	r.statements = append(r.statements, sql)
	r.mu.Unlock()
}

func (r *sqlRecorder) recorded() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.statements...)
}

// reset drops the statements recorded so far (connect-time housekeeping
// queries included), so assertions see only what one Prepare executed.
func (r *sqlRecorder) reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.statements = nil
}

// newActiveTestContext opens an in-memory sqlite database with the members
// table, wiring the statement recorder into the connection and the
// connection into an application context, exactly the way ar's own tests
// do. The pool is capped at one connection because an in-memory database
// exists per connection.
func newActiveTestContext(t *testing.T) (context.Context, *sqlRecorder) {
	t.Helper()
	rec := &sqlRecorder{}
	conn, err := db.Connect(config.DB{Driver: "sqlite", DSN: ":memory:"}, db.WithGormLogger(rec))
	if err != nil {
		t.Fatalf("connect in-memory sqlite: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	sqlDB, err := conn.DB.DB()
	if err != nil {
		t.Fatalf("raw connection: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	if err := conn.Exec("CREATE TABLE members (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL, status INTEGER NOT NULL DEFAULT 1)").Error; err != nil {
		t.Fatalf("create table: %v", err)
	}
	app := base.New(config.Config{})
	app.SetDB(conn)
	return app.WithContext(context.Background()), rec
}

// seedMembers inserts n members named m01..mNN with ids 1..n and alternating
// status, straight through SQL so provider tests never depend on the insert
// path.
func seedMembers(t *testing.T, ctx context.Context, n int) {
	t.Helper()
	for i := 1; i <= n; i++ {
		status := 1
		if i%2 == 0 {
			status = 2
		}
		if err := execSQL(ctx, "INSERT INTO members (id, name, status) VALUES (?, ?, ?)", i, "m", status); err != nil {
			t.Fatalf("seed member %d: %v", i, err)
		}
	}
}

func execSQL(ctx context.Context, query string, args ...any) error {
	conn, err := base.FromContext(ctx).DB()
	if err != nil {
		return err
	}
	return conn.Exec(query, args...).Error
}
