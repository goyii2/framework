package ar

import (
	"context"
	"testing"

	"github.com/goyii2/framework/base"
	"github.com/goyii2/framework/config"
	"github.com/goyii2/framework/db"
)

// user is the base test model: table users, one-to-many posts.
type user struct {
	Model
	ID    int64   `gorm:"column:id;primaryKey"`
	Name  string  `gorm:"column:name"`
	Email string  `gorm:"column:email"`
	Posts []*post `gorm:"foreignKey:UserID"`
}

func (user) TableName() string { return "users" }

// post belongs to user via user_id.
type post struct {
	Model
	ID     int64  `gorm:"column:id;primaryKey"`
	UserID int64  `gorm:"column:user_id"`
	Title  string `gorm:"column:title"`
}

func (post) TableName() string { return "posts" }

// detachedUser proves the layer queries plain structs that do not embed
// Model; such records simply have no persistence methods.
type detachedUser struct {
	ID   int64  `gorm:"column:id;primaryKey"`
	Name string `gorm:"column:name"`
}

func (detachedUser) TableName() string { return "users" }

// newTestContext opens an in-memory sqlite database, creates the test tables
// and returns a context whose application carries that connection as its db
// component. The pool is capped at one connection on purpose: an in-memory
// database exists per connection, so a second pooled connection would see an
// empty database.
func newTestContext(t *testing.T) context.Context {
	t.Helper()
	conn, err := db.New(db.Config{Driver: "sqlite", DSN: ":memory:"})
	if err != nil {
		t.Fatalf("connect in-memory sqlite: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	sqlDB, err := conn.DB.DB()
	if err != nil {
		t.Fatalf("raw connection: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	for _, ddl := range []string{
		"CREATE TABLE users (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL, email TEXT NOT NULL DEFAULT '')",
		"CREATE TABLE posts (id INTEGER PRIMARY KEY AUTOINCREMENT, user_id INTEGER NOT NULL, title TEXT NOT NULL)",
		"CREATE TABLE hook_users (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL)",
	} {
		if err := conn.Exec(ddl).Error; err != nil {
			t.Fatalf("create table: %v", err)
		}
	}
	app := base.New(config.Config{})
	app.SetDB(conn)
	return app.WithContext(context.Background())
}

// seedUsers inserts users named after names with ids 1..n directly through
// SQL, so query tests do not depend on the insert path under test.
func seedUsers(t *testing.T, ctx context.Context, names ...string) {
	t.Helper()
	for i, name := range names {
		if err := exec(ctx, "INSERT INTO users (id, name) VALUES (?, ?)", i+1, name); err != nil {
			t.Fatalf("seed user %q: %v", name, err)
		}
	}
}

// exec runs a raw statement on the connection backing ctx. Never call it
// with a transaction context: the application connection is a separate
// session and, with a one-connection pool, would deadlock against the open
// transaction.
func exec(ctx context.Context, query string, args ...any) error {
	conn, err := base.FromContext(ctx).DB()
	if err != nil {
		return err
	}
	return conn.Exec(query, args...).Error
}
