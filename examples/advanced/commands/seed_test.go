// Tests of the seed command: the accounts and the RBAC grant it creates,
// and the idempotency that lets it run against an already-seeded database.
package commands

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/goyii2/framework/base"
	"github.com/goyii2/framework/db/ar"
	"github.com/goyii2/framework/examples/advanced/common/models"
	"github.com/goyii2/framework/rbac"
)

// runSeed executes the seed command once and returns its stdout.
func runSeed(t *testing.T, app *base.Application, args ...string) (string, error) {
	t.Helper()
	cmd := NewSeedCommand()
	cmd.SetContext(app.WithContext(context.Background()))
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

func TestSeedCreatesAccountsAndAdminGrant(t *testing.T) {
	app := newTestApp(t)

	if _, err := runSeed(t, app); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Both demo accounts exist and accept their passwords.
	for _, acc := range []struct{ username, password string }{
		{"admin", "admin123"},
		{"demo", "demo123"},
	} {
		user, err := ar.New[models.User](app.WithContext(context.Background())).Where("username = ?", acc.username).One()
		if err != nil {
			t.Fatalf("loading %s: %v", acc.username, err)
		}
		if !user.CheckPassword(acc.password) {
			t.Errorf("%s does not accept the seeded password", acc.username)
		}
	}

	// The admin role resolves through the auth manager — the same check
	// the backend guard runs — while demo holds nothing.
	auth, err := rbac.NewDbManager(app.DB().DB)
	if err != nil {
		t.Fatalf("rbac.NewDbManager: %v", err)
	}
	admin, err := ar.New[models.User](app.WithContext(context.Background())).Where("username = ?", "admin").One()
	if err != nil {
		t.Fatalf("loading admin: %v", err)
	}
	if ok, err := auth.CheckAccess(admin.GetID(), "admin", nil); err != nil || !ok {
		t.Errorf("CheckAccess(admin, admin) = %v, %v; want true, nil", ok, err)
	}
	demo, err := ar.New[models.User](app.WithContext(context.Background())).Where("username = ?", "demo").One()
	if err != nil {
		t.Fatalf("loading demo: %v", err)
	}
	if ok, err := auth.CheckAccess(demo.GetID(), "admin", nil); err != nil || ok {
		t.Errorf("CheckAccess(demo, admin) = %v, %v; want false, nil", ok, err)
	}
}

func TestSeedIsRepeatable(t *testing.T) {
	app := newTestApp(t)

	if _, err := runSeed(t, app); err != nil {
		t.Fatalf("first seed: %v", err)
	}
	out, err := runSeed(t, app)
	if err != nil {
		t.Fatalf("second seed must not fail: %v", err)
	}
	if !strings.Contains(out, "user admin already exists") || !strings.Contains(out, "user demo already exists") {
		t.Fatalf("second run output %q lacks the already-exists notes", out)
	}

	// The grant is not duplicated: one admin assignment row, two accounts
	// — the semantic no-op a re-run promises.
	assertCount(t, app, "user", 2)
	assertCount(t, app, "auth_assignment", 1)
}

// assertCount asserts the row count of one table.
func assertCount(t *testing.T, app *base.Application, table string, want int) {
	t.Helper()
	var n int64
	if err := app.DB().DB.Table(table).Count(&n).Error; err != nil {
		t.Fatalf("counting %s: %v", table, err)
	}
	if n != int64(want) {
		t.Errorf("%s count = %d, want %d", table, n, want)
	}
}
