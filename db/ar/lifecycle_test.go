package ar

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"gorm.io/gorm"
)

// hookUser implements every GORM lifecycle hook, recording the call order in
// hooks; failCreate makes BeforeCreate refuse, so failure paths can be
// exercised. Both fields are unexported, so they map to no columns.
type hookUser struct {
	Model
	ID         int64  `gorm:"column:id;primaryKey"`
	Name       string `gorm:"column:name"`
	failCreate bool
	hooks      []string
}

func (hookUser) TableName() string { return "hook_users" }

func (u *hookUser) BeforeSave(tx *gorm.DB) error { u.hooks = append(u.hooks, "BeforeSave"); return nil }
func (u *hookUser) BeforeCreate(tx *gorm.DB) error {
	u.hooks = append(u.hooks, "BeforeCreate")
	if u.failCreate {
		return errors.New("hookUser: BeforeCreate refused")
	}
	return nil
}
func (u *hookUser) AfterCreate(tx *gorm.DB) error {
	u.hooks = append(u.hooks, "AfterCreate")
	return nil
}
func (u *hookUser) BeforeUpdate(tx *gorm.DB) error {
	u.hooks = append(u.hooks, "BeforeUpdate")
	return nil
}
func (u *hookUser) AfterUpdate(tx *gorm.DB) error {
	u.hooks = append(u.hooks, "AfterUpdate")
	return nil
}
func (u *hookUser) AfterSave(tx *gorm.DB) error { u.hooks = append(u.hooks, "AfterSave"); return nil }
func (u *hookUser) BeforeDelete(tx *gorm.DB) error {
	u.hooks = append(u.hooks, "BeforeDelete")
	return nil
}
func (u *hookUser) AfterDelete(tx *gorm.DB) error {
	u.hooks = append(u.hooks, "AfterDelete")
	return nil
}
func (u *hookUser) AfterFind(tx *gorm.DB) error { u.hooks = append(u.hooks, "AfterFind"); return nil }

func assertHooks(t *testing.T, got []string, want ...string) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("hooks = %v, want %v", got, want)
	}
}

func TestLifecycleHooksOnInsertFindUpdateDelete(t *testing.T) {
	ctx := newTestContext(t)

	created := New[hookUser](ctx).New()
	created.Name = "hooks"
	if err := created.Save(); err != nil {
		t.Fatalf("insert: %v", err)
	}
	assertHooks(t, created.hooks, "BeforeSave", "BeforeCreate", "AfterCreate", "AfterSave")

	loaded, err := FindByID[hookUser](ctx, created.ID)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	assertHooks(t, loaded.hooks, "AfterFind")

	loaded.Name = "renamed"
	if err := loaded.Save(); err != nil {
		t.Fatalf("update: %v", err)
	}
	assertHooks(t, loaded.hooks, "AfterFind", "BeforeSave", "BeforeUpdate", "AfterUpdate", "AfterSave")

	if err := loaded.Delete(); err != nil {
		t.Fatalf("delete: %v", err)
	}
	assertHooks(t, loaded.hooks,
		"AfterFind", "BeforeSave", "BeforeUpdate", "AfterUpdate", "AfterSave", "BeforeDelete", "AfterDelete")
}

func TestHookErrorAbortsInsert(t *testing.T) {
	ctx := newTestContext(t)
	u := New[hookUser](ctx).New()
	u.Name = "never"
	u.failCreate = true
	if err := u.Save(); err == nil {
		t.Fatal("Save with a failing BeforeCreate returned nil error")
	}
	if n, err := New[hookUser](ctx).Count(); err != nil || n != 0 {
		t.Errorf("row count = %d (err %v), want 0: a failed hook must abort the insert", n, err)
	}
}

// insertUser saves a new bound record named name — the transaction tests'
// unit of work.
func insertUser(ctx context.Context, name string) error {
	u := New[user](ctx).New()
	u.Name = name
	return u.Save()
}

func TestTransactionCommitsOnSuccess(t *testing.T) {
	ctx := newTestContext(t)
	err := Transaction(ctx, func(tctx context.Context) error {
		return insertUser(tctx, "kept")
	})
	if err != nil {
		t.Fatalf("Transaction: %v", err)
	}
	got, err := New[user](ctx).One()
	if err != nil || got.Name != "kept" {
		t.Fatalf("row missing after commit: %v %+v", err, got)
	}
}

func TestTransactionRollsBackEverythingOnError(t *testing.T) {
	ctx := newTestContext(t)
	boom := errors.New("boom")
	err := Transaction(ctx, func(tctx context.Context) error {
		if err := insertUser(tctx, "one"); err != nil {
			return err
		}
		if err := insertUser(tctx, "two"); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("Transaction error = %v, want %v", err, boom)
	}
	if n, err := New[user](ctx).Count(); err != nil || n != 0 {
		t.Errorf("count after rollback = %d (err %v), want 0", n, err)
	}
}

func TestTransactionRollsBackWhenHookFails(t *testing.T) {
	ctx := newTestContext(t)
	err := Transaction(ctx, func(tctx context.Context) error {
		if err := insertUser(tctx, "first"); err != nil {
			return err
		}
		u := New[hookUser](tctx).New()
		u.Name = "doomed"
		u.failCreate = true
		return u.Save()
	})
	if err == nil {
		t.Fatal("Transaction with a failing hook returned nil error")
	}
	if n, err := New[user](ctx).Count(); err != nil || n != 0 {
		t.Errorf("count = %d (err %v), want 0: the earlier insert must roll back with the failed one", n, err)
	}
}

func TestInstanceStaysBoundToItsTransaction(t *testing.T) {
	ctx := newTestContext(t)
	var bound *user
	err := Transaction(ctx, func(tctx context.Context) error {
		bound = New[user](tctx).New()
		bound.Name = "tx"
		if err := bound.Save(); err != nil {
			return err
		}
		return errors.New("rollback")
	})
	if err == nil {
		t.Fatal("Transaction error = nil, want rollback")
	}
	if _, err := FindByID[user](ctx, bound.ID); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Errorf("row visible after rollback: err = %v", err)
	}
	// The instance keeps its transaction context; after the rollback that
	// handle is finished, so persistence must refuse rather than silently
	// fall back to the outer connection.
	if err := bound.Reload(); err == nil {
		t.Error("Reload through the finished transaction returned nil error")
	}
}

func TestNestedTransactionRollsBackToSavepoint(t *testing.T) {
	ctx := newTestContext(t)
	err := Transaction(ctx, func(octx context.Context) error {
		if err := insertUser(octx, "outer"); err != nil {
			return err
		}
		innerErr := Transaction(octx, func(ictx context.Context) error {
			if err := insertUser(ictx, "inner"); err != nil {
				return err
			}
			return errors.New("inner rollback")
		})
		if innerErr == nil {
			return errors.New("inner transaction unexpectedly committed")
		}
		return nil // the outer unit commits despite the inner rollback
	})
	if err != nil {
		t.Fatalf("outer transaction: %v", err)
	}
	got, err := New[user](ctx).OrderBy("id").All()
	if err != nil {
		t.Fatalf("All: %v", err)
	}
	if len(got) != 1 || got[0].Name != "outer" {
		t.Errorf("rows after nested rollback: %+v, want only [outer]", got)
	}
}

func TestInstanceMethodsInsideTransactionParticipate(t *testing.T) {
	ctx := newTestContext(t)
	err := Transaction(ctx, func(tctx context.Context) error {
		u := New[user](tctx).New()
		u.Name = "original"
		if err := u.Save(); err != nil {
			return err
		}
		// A second bound instance updates the row inside the same
		// transaction; the first one's Reload must observe the uncommitted
		// write.
		other, err := FindByID[user](tctx, u.ID)
		if err != nil {
			return err
		}
		other.Name = "changed"
		if err := other.Save(); err != nil {
			return err
		}
		if err := u.Reload(); err != nil {
			return err
		}
		if u.Name != "changed" {
			t.Errorf("Reload inside the transaction saw %q, want changed", u.Name)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Transaction: %v", err)
	}
}
