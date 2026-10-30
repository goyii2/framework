package ar

import (
	"context"
	"errors"
	"strings"
	"testing"

	"gorm.io/gorm"

	"github.com/goyii2/framework/base"
)

func TestInsertFindUpdateDeleteReload(t *testing.T) {
	ctx := newTestContext(t)

	created := New[user](ctx).New()
	created.Name = "alice"
	created.Email = "alice@example.com"
	if err := created.Save(); err != nil {
		t.Fatalf("Save(new): %v", err)
	}
	if created.ID == 0 {
		t.Fatal("Save must backfill the primary key")
	}

	loaded, err := FindByID[user](ctx, created.ID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if loaded.Name != "alice" || loaded.Email != "alice@example.com" {
		t.Errorf("FindByID loaded name=%q email=%q", loaded.Name, loaded.Email)
	}

	loaded.Name = "alice2"
	if err := loaded.Save(); err != nil {
		t.Fatalf("Save(update): %v", err)
	}
	fresh, err := FindByID[user](ctx, created.ID)
	if err != nil {
		t.Fatalf("FindByID after update: %v", err)
	}
	if fresh.Name != "alice2" {
		t.Errorf("update did not persist, name = %q", fresh.Name)
	}

	if err := exec(ctx, "UPDATE users SET name = 'alice3' WHERE id = ?", created.ID); err != nil {
		t.Fatalf("raw update: %v", err)
	}
	if err := loaded.Reload(); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if loaded.Name != "alice3" {
		t.Errorf("Reload kept stale name %q", loaded.Name)
	}

	if err := loaded.Delete(); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := FindByID[user](ctx, created.ID); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Errorf("FindByID after Delete: err = %v, want gorm.ErrRecordNotFound", err)
	}
}

func TestQueryChainWhereOrderByLimitOffset(t *testing.T) {
	ctx := newTestContext(t)
	seedUsers(t, ctx, "a", "b", "c", "d", "e")

	got, err := New[user](ctx).Where("id > ?", 1).OrderBy("id DESC").Limit(2).Offset(1).All()
	if err != nil {
		t.Fatalf("All: %v", err)
	}
	ids := make([]int64, 0, len(got))
	for _, u := range got {
		ids = append(ids, u.ID)
	}
	// WHERE id > 1 -> {2,3,4,5}; ORDER BY id DESC -> 5,4,3,2;
	// OFFSET 1 -> 4,3,2; LIMIT 2 -> 4,3.
	if len(ids) != 2 || ids[0] != 4 || ids[1] != 3 {
		t.Errorf("ids = %v, want [4 3]", ids)
	}
}

func TestQueryCount(t *testing.T) {
	ctx := newTestContext(t)
	seedUsers(t, ctx, "a", "b", "c", "d", "e")

	if n, err := New[user](ctx).Count(); err != nil || n != 5 {
		t.Errorf("Count() = %d (err %v), want 5", n, err)
	}
	if n, err := New[user](ctx).Where("id > ?", 1).Count(); err != nil || n != 4 {
		t.Errorf("Count(Where) = %d (err %v), want 4", n, err)
	}
}

func TestQueryOne(t *testing.T) {
	ctx := newTestContext(t)
	seedUsers(t, ctx, "a", "b")

	got, err := New[user](ctx).Where("name = ?", "b").One()
	if err != nil {
		t.Fatalf("One: %v", err)
	}
	if got.Name != "b" {
		t.Errorf("One loaded name = %q, want b", got.Name)
	}
	if _, err := New[user](ctx).Where("name = ?", "zz").One(); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Errorf("One without match: err = %v, want gorm.ErrRecordNotFound", err)
	}
}

func TestQueryBranchesDoNotInterfere(t *testing.T) {
	ctx := newTestContext(t)
	seedUsers(t, ctx, "a", "b", "c", "d", "e")

	core := New[user](ctx).Where("id > ?", 1)
	limited := core.Limit(2)

	all, err := core.All()
	if err != nil {
		t.Fatalf("core All: %v", err)
	}
	if len(all) != 4 {
		t.Errorf("core query was modified by its branch: got %d records, want 4", len(all))
	}
	two, err := limited.All()
	if err != nil {
		t.Fatalf("branch All: %v", err)
	}
	if len(two) != 2 {
		t.Errorf("branch got %d records, want 2", len(two))
	}
}

func TestWithPreloadsAssociations(t *testing.T) {
	ctx := newTestContext(t)
	alice := New[user](ctx).New()
	alice.Name = "alice"
	if err := alice.Save(); err != nil {
		t.Fatalf("save alice: %v", err)
	}
	bob := New[user](ctx).New()
	bob.Name = "bob"
	if err := bob.Save(); err != nil {
		t.Fatalf("save bob: %v", err)
	}
	for _, title := range []string{"a1", "a2"} {
		p := New[post](ctx).New()
		p.UserID, p.Title = alice.ID, title
		if err := p.Save(); err != nil {
			t.Fatalf("save post %q: %v", title, err)
		}
	}
	p := New[post](ctx).New()
	p.UserID, p.Title = bob.ID, "b1"
	if err := p.Save(); err != nil {
		t.Fatalf("save post b1: %v", err)
	}

	users, err := New[user](ctx).With("Posts").OrderBy("id").All()
	if err != nil {
		t.Fatalf("All with preload: %v", err)
	}
	if len(users) != 2 {
		t.Fatalf("got %d users, want 2", len(users))
	}
	if got := users[0].Posts; len(got) != 2 || got[0].Title != "a1" || got[1].Title != "a2" {
		t.Errorf("alice posts = %+v, want [a1 a2]", got)
	}
	if got := users[1].Posts; len(got) != 1 || got[0].Title != "b1" {
		t.Errorf("bob posts = %+v, want [b1]", got)
	}

	plain, err := New[user](ctx).OrderBy("id").All()
	if err != nil {
		t.Fatalf("All without preload: %v", err)
	}
	if len(plain[0].Posts) != 0 {
		t.Errorf("association loaded without With: %+v", plain[0].Posts)
	}
}

func TestJoinFiltersByRelatedTable(t *testing.T) {
	ctx := newTestContext(t)
	seedUsers(t, ctx, "alice", "bob") // ids 1, 2
	for _, insert := range []struct {
		userID int64
		title  string
	}{
		{1, "shared"},
		{2, "shared"},
		{1, "other"},
	} {
		if err := exec(ctx, "INSERT INTO posts (user_id, title) VALUES (?, ?)", insert.userID, insert.title); err != nil {
			t.Fatalf("seed post %q: %v", insert.title, err)
		}
	}

	got, err := New[user](ctx).
		Join("JOIN posts ON posts.user_id = users.id").
		Where("posts.title = ?", "shared").
		All()
	if err != nil {
		t.Fatalf("All with join: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d users, want 2", len(got))
	}
	names := map[int64]string{}
	for _, u := range got {
		names[u.ID] = u.Name
	}
	// The join must not bleed posts columns into the user records.
	if names[1] != "alice" || names[2] != "bob" {
		t.Errorf("joined rows lost the user columns: %v", names)
	}
}

func TestDetachedStructQueriesWithoutModel(t *testing.T) {
	ctx := newTestContext(t)
	seedUsers(t, ctx, "a", "b")

	got, err := New[detachedUser](ctx).Where("id = ?", 1).One()
	if err != nil {
		t.Fatalf("One on a plain struct: %v", err)
	}
	if got.Name != "a" {
		t.Errorf("got name = %q, want a", got.Name)
	}
}

func TestUnboundModelRejectsPersistence(t *testing.T) {
	u := &user{ID: 1}
	calls := map[string]func() error{
		"Save":   func() error { return u.Save() },
		"Delete": func() error { return u.Delete() },
		"Reload": func() error { return u.Reload() },
	}
	for name, call := range calls {
		if err := call(); err == nil || !strings.Contains(err.Error(), "not bound") {
			t.Errorf("%s on unbound model: err = %v, want the not-bound error", name, err)
		}
	}
}

func TestReloadWithoutPrimaryKeyFails(t *testing.T) {
	ctx := newTestContext(t)
	fresh := New[user](ctx).New()
	if err := fresh.Reload(); err == nil || !strings.Contains(err.Error(), "primary key") {
		t.Errorf("Reload before save: err = %v, want the primary key error", err)
	}
}

func TestFindByIDRejectsModelWithoutPrimaryKey(t *testing.T) {
	type noPK struct {
		Name string `gorm:"column:name"`
	}
	ctx := newTestContext(t)
	handle, err := session(ctx)
	if err != nil {
		t.Fatalf("session: %v", err)
	}
	if _, err := primaryKey(handle, &noPK{}); err == nil {
		t.Error("primaryKey on a model without a primary key must fail")
	}
}

func TestUseConnBypassesApplication(t *testing.T) {
	ctx := newTestContext(t)
	seedUsers(t, ctx, "a", "b", "c")

	conn := base.FromContext(ctx).DB()
	bare := UseConn(context.Background(), conn)
	if n, err := New[user](bare).Count(); err != nil || n != 3 {
		t.Errorf("Count via UseConn = %d (err %v), want 3", n, err)
	}
}
