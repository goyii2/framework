// Tests of the user/create console command: account creation through the
// model's insert validation, and the duplicate refusal that validation
// produces.
package commands

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/goyii2/framework/base"
	"github.com/goyii2/framework/db"
	"github.com/goyii2/framework/db/ar"
	"github.com/goyii2/framework/examples/advanced/common/migrations"
	"github.com/goyii2/framework/examples/advanced/common/models"
	"github.com/goyii2/framework/migrate"
)

// newTestApp assembles the application the command runs against: an
// in-memory sqlite database the real migrations shaped. The pool is capped
// at one connection because an in-memory database exists per connection.
func newTestApp(t *testing.T) *base.Application {
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
	mig := migrate.NewMigrator(sqlDB, migrate.SQLiteDialect{}, migrations.All())
	mig.UseLock = false
	ctx := context.Background()
	if err := mig.EnsureMigrationTable(ctx); err != nil {
		t.Fatalf("ensure migration table: %v", err)
	}
	if _, err := mig.Up(ctx, 0); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	app := base.New()
	app.SetDB(conn)
	return app
}

// runUserCreate executes the command once with args and returns its stdout.
func runUserCreate(t *testing.T, app *base.Application, args ...string) (string, error) {
	t.Helper()
	cmd := NewUserCreateCommand()
	cmd.SetContext(app.WithContext(context.Background()))
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

func TestUserCreateCommandCreatesAccount(t *testing.T) {
	app := newTestApp(t)

	out, err := runUserCreate(t, app, "--username", "admin", "--password", "admin123", "--role", "admin")
	if err != nil {
		t.Fatalf("user/create: %v", err)
	}
	if !strings.Contains(out, "created user admin") || !strings.Contains(out, "role admin") {
		t.Fatalf("output %q lacks the creation confirmation", out)
	}

	user, err := ar.New[models.User](app.WithContext(context.Background())).Where("username = ?", "admin").One()
	if err != nil {
		t.Fatalf("loading the created account: %v", err)
	}
	if user.Role != "admin" {
		t.Errorf("role = %q, want admin", user.Role)
	}
	if !user.CheckPassword("admin123") {
		t.Error("the created account does not accept its password")
	}
	if user.CheckPassword("admin124") {
		t.Error("the created account accepts a wrong password")
	}
}

func TestUserCreateCommandRefusesDuplicateUsername(t *testing.T) {
	app := newTestApp(t)

	if _, err := runUserCreate(t, app, "--username", "demo", "--password", "demo123"); err != nil {
		t.Fatalf("first user/create: %v", err)
	}
	_, err := runUserCreate(t, app, "--username", "demo", "--password", "other123")
	if err == nil {
		t.Fatal("duplicate username must fail the command")
	}
	if !strings.Contains(err.Error(), "username is already taken") {
		t.Errorf("error = %v, want the uniqueness message", err)
	}
}
