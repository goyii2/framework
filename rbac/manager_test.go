package rbac

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/goyii2/framework/db"
	"github.com/goyii2/framework/migrate"
)

// newTestManager opens a file-backed sqlite database, applies the auth tables
// migration and returns a manager (plus its connection) over it. The second
// returned manager shares nothing but the database — the "fresh process" view
// used to prove persistence and cache freshness.
func newTestManager(t *testing.T, rules ...Rule) (*DbManager, *DbManager, *db.Conn) {
	t.Helper()
	dsn := filepath.Join(t.TempDir(), "rbac.db")

	conn, err := db.New(db.Config{Driver: "sqlite", DSN: dsn})
	if err != nil {
		t.Fatalf("connect sqlite: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	sqlDB, err := conn.DB.DB()
	if err != nil {
		t.Fatalf("raw database handle: %v", err)
	}
	dialect, ok := migrate.DialectByName("sqlite")
	if !ok {
		t.Fatal("sqlite dialect missing")
	}
	mctx := migrate.NewMigrationContext(sqlDB, dialect)
	if err := NewMigration().Up(context.Background(), mctx); err != nil {
		t.Fatalf("apply auth tables migration: %v", err)
	}

	mgr, err := NewDbManager(conn.DB, rules...)
	if err != nil {
		t.Fatalf("NewDbManager: %v", err)
	}
	fresh, err := NewDbManager(conn.DB, rules...)
	if err != nil {
		t.Fatalf("NewDbManager (fresh): %v", err)
	}
	return mgr, fresh, conn
}

// addRole stores a role, failing the test on error.
func addRole(t *testing.T, m *DbManager, name string) {
	t.Helper()
	if err := m.AddItem(Item{Name: name, Type: TypeRole}); err != nil {
		t.Fatalf("add role %s: %v", name, err)
	}
}

// addPermission stores a permission, failing the test on error.
func addPermission(t *testing.T, m *DbManager, name string, ruleName string) {
	t.Helper()
	if err := m.AddItem(Item{Name: name, Type: TypePermission, RuleName: ruleName}); err != nil {
		t.Fatalf("add permission %s: %v", name, err)
	}
}

// grant makes child part of parent, failing the test on error.
func grant(t *testing.T, m *DbManager, parent, child string) {
	t.Helper()
	if err := m.AddChild(parent, child); err != nil {
		t.Fatalf("addChild %s -> %s: %v", parent, child, err)
	}
}

// assign grants itemName to userID, failing the test on error.
func assign(t *testing.T, m *DbManager, itemName, userID string) {
	t.Helper()
	if err := m.Assign(itemName, userID); err != nil {
		t.Fatalf("assign %s to %s: %v", itemName, userID, err)
	}
}

// mustAccess asserts a CheckAccess verdict, failing with both verdicts in the
// message.
func mustAccess(t *testing.T, m *DbManager, userID, itemName string, want bool) {
	t.Helper()
	got, err := m.CheckAccess(userID, itemName, nil)
	if err != nil {
		t.Fatalf("CheckAccess(%s, %s) error: %v", userID, itemName, err)
	}
	if got != want {
		t.Errorf("CheckAccess(%s, %s) = %v, want %v", userID, itemName, got, want)
	}
}

// funcRule is a test rule whose verdict comes from a function, so tests can
// flip it mid-scenario.
type funcRule struct {
	name string
	fn   func(userID string, params Params) bool
}

func (r funcRule) Name() string                              { return r.name }
func (r funcRule) Execute(userID string, params Params) bool { return r.fn(userID, params) }

// nameList extracts the names of items, sorted for assertion-friendly order.
func nameList(items []Item) []string {
	names := make([]string, 0, len(items))
	for _, it := range items {
		names = append(names, it.Name)
	}
	sort.Strings(names)
	return names
}

// --- hierarchy verdicts ------------------------------------------------

// TestCheckAccessDirectAssignment covers the trivial verdicts: a directly
// assigned item is held, anything else is not, and both directions stay clean
// for users without assignments.
func TestCheckAccessDirectAssignment(t *testing.T) {
	m, _, _ := newTestManager(t)
	addRole(t, m, "admin")
	addPermission(t, m, "post/delete", "")
	assign(t, m, "admin", "1")

	mustAccess(t, m, "1", "admin", true)
	mustAccess(t, m, "1", "post/delete", false) // no hierarchy edge yet
	mustAccess(t, m, "2", "admin", false)       // never assigned
	mustAccess(t, m, "1", "nosuchitem", false)  // unknown items deny cleanly

	ok, err := m.CheckAccess("1", "nosuchitem", nil)
	if err != nil {
		t.Errorf("unknown item must deny without error, got %v", err)
	}
	if ok {
		t.Error("unknown item must deny")
	}
}

// TestCheckAccessInheritsThroughHierarchy builds a three-level chain of
// roles plus a permission below and asserts the multi-level inheritance both
// ways: everything below an assignment is held, nothing above it is.
func TestCheckAccessInheritsThroughHierarchy(t *testing.T) {
	m, fresh, _ := newTestManager(t)
	for _, role := range []string{"admin", "editor"} {
		addRole(t, m, role)
	}
	addRole(t, m, "author")
	addPermission(t, m, "post/create", "")
	addPermission(t, m, "post/delete", "")
	// admin contains editor, editor contains author, author grants post/create.
	grant(t, m, "admin", "editor")
	grant(t, m, "editor", "author")
	grant(t, m, "author", "post/create")
	grant(t, m, "admin", "post/delete")
	assign(t, m, "author", "7")

	// Indirect over multiple levels: assigned author, holds post/create.
	mustAccess(t, m, "7", "post/create", true)
	// Downward containment does not leak upward.
	mustAccess(t, m, "7", "editor", false)
	mustAccess(t, m, "7", "admin", false)
	mustAccess(t, m, "7", "post/delete", false)

	// The admin assignment sees through every level at once.
	assign(t, m, "admin", "8")
	mustAccess(t, m, "8", "post/create", true)
	mustAccess(t, m, "8", "post/delete", true)
	mustAccess(t, m, "8", "author", true)

	// A second manager (a fresh process, an empty cache) reads the same
	// verdicts from the database.
	mustAccess(t, fresh, "7", "post/create", true)
	mustAccess(t, fresh, "7", "editor", false)
	mustAccess(t, fresh, "8", "post/delete", true)
}

// TestCheckAccessDiamondInheritance asserts a diamond DAG: the item is
// reachable over two distinct paths and the visited bookkeeping terminates.
func TestCheckAccessDiamondInheritance(t *testing.T) {
	m, _, _ := newTestManager(t)
	for _, name := range []string{"root", "left", "right"} {
		addRole(t, m, name)
	}
	addPermission(t, m, "leaf", "")
	grant(t, m, "root", "left")
	grant(t, m, "root", "right")
	grant(t, m, "left", "leaf")
	grant(t, m, "right", "leaf")

	assign(t, m, "root", "1")
	mustAccess(t, m, "1", "leaf", true)
	assign(t, m, "left", "2")
	mustAccess(t, m, "2", "leaf", true)
	mustAccess(t, m, "2", "root", false)
}

// --- cycle rejection ----------------------------------------------------

// TestAddChildRejectsCycles covers every way an edge could close a loop:
// self-edges, the direct back edge and the transitive one, plus the Yii2 rule
// that a permission may not contain a role and that edges need real items.
func TestAddChildRejectsCycles(t *testing.T) {
	m, _, _ := newTestManager(t)
	addRole(t, m, "admin")
	addRole(t, m, "editor")
	addRole(t, m, "author")
	addPermission(t, m, "post/create", "")
	grant(t, m, "admin", "editor")
	grant(t, m, "editor", "author")

	if err := m.AddChild("admin", "admin"); err == nil {
		t.Error("an item cannot become its own child")
	}
	if err := m.AddChild("editor", "admin"); err == nil {
		t.Error("the back edge editor -> admin closes a direct cycle and must be rejected")
	}
	if err := m.AddChild("author", "admin"); err == nil {
		t.Error("the transitive back edge author -> admin closes a cycle and must be rejected")
	}
	if err := m.AddChild("post/create", "admin"); err == nil {
		t.Error("a permission cannot contain a role")
	}
	if err := m.AddChild("ghost", "admin"); !errors.Is(err, ErrItemNotFound) {
		t.Errorf("edge with missing parent must report ErrItemNotFound, got %v", err)
	}
	if err := m.AddChild("admin", "ghost"); !errors.Is(err, ErrItemNotFound) {
		t.Errorf("edge with missing child must report ErrItemNotFound, got %v", err)
	}

	// Role containing role stays legal, permission containing permission too.
	if err := m.AddChild("post/create", "post/create"); err == nil {
		t.Error("a permission also cannot become its own child")
	}
}

// TestCheckAccessSurvivesTamperedCycle inserts a cycle straight into the
// edges table (bypassing the manager's guard — the database is writable by
// others) and asserts the traversal terminates and denies instead of hanging.
func TestCheckAccessSurvivesTamperedCycle(t *testing.T) {
	m, _, _ := newTestManager(t)
	addRole(t, m, "a")
	addRole(t, m, "b")
	grant(t, m, "a", "b")
	// The illegal back edge: a -> b -> a.
	if err := m.db.Create(&authItemChildModel{Parent: "b", Child: "a"}).Error; err != nil {
		t.Fatalf("tamper edge: %v", err)
	}
	// Drop the cache so the poisoned graph is actually loaded.
	m.mu.Lock()
	m.graph = nil
	m.assignments = nil
	m.mu.Unlock()

	assign(t, m, "a", "1")
	mustAccess(t, m, "1", "a", true)
	mustAccess(t, m, "1", "b", true)
	mustAccess(t, m, "1", "nosuch", false)
}

// --- rules --------------------------------------------------------------

// TestRuleExecutesAndCanVeto grants a permission through a role and flips the
// permission's rule: access follows the rule's verdict even though the
// assignment and the hierarchy are in place.
func TestRuleExecutesAndCanVeto(t *testing.T) {
	verdict := true
	m, _, _ := newTestManager(t, funcRule{name: "strict", fn: func(string, Params) bool { return verdict }})

	addRole(t, m, "editor")
	addPermission(t, m, "post/update", "strict")
	grant(t, m, "editor", "post/update")
	assign(t, m, "editor", "1")

	mustAccess(t, m, "1", "post/update", true)
	verdict = false
	mustAccess(t, m, "1", "post/update", false)
	// The rule vetoes only the item it hangs on; the role itself stays held.
	mustAccess(t, m, "1", "editor", true)
}

// TestRuleOnAncestorVetoesDownstream puts the rule on the role in the middle
// of the chain: when it fails, the permission below is unreachable through
// that ancestor even though the assignment matches.
func TestRuleOnAncestorVetoesDownstream(t *testing.T) {
	verdict := true
	m, _, _ := newTestManager(t, funcRule{name: "office-hours", fn: func(string, Params) bool { return verdict }})

	addRole(t, m, "editor")
	if err := m.AddItem(Item{Name: "editor-in-office", Type: TypeRole, RuleName: "office-hours"}); err != nil {
		t.Fatalf("add conditional role: %v", err)
	}
	// post/update is reachable through both editors; only one is conditional.
	addPermission(t, m, "post/update", "")
	grant(t, m, "editor", "post/update")
	grant(t, m, "editor-in-office", "post/update")
	assign(t, m, "editor-in-office", "1")

	mustAccess(t, m, "1", "post/update", true)
	verdict = false
	mustAccess(t, m, "1", "post/update", false)
	verdict = true
	mustAccess(t, m, "1", "post/update", true)
}

// TestRuleReceivesUserAndParams asserts the rule sees the checked user and
// the params carried by the check.
func TestRuleReceivesUserAndParams(t *testing.T) {
	m, _, _ := newTestManager(t, funcRule{name: "author-only", fn: func(userID string, params Params) bool {
		author, _ := params["author"].(string)
		return author == userID
	}})
	addRole(t, m, "author")
	addPermission(t, m, "post/update-own", "author-only")
	grant(t, m, "author", "post/update-own")
	assign(t, m, "author", "42")

	// The rule passes when the params name the checking user as the author...
	ok, err := m.CheckAccess("42", "post/update-own", Params{"author": "42"})
	if err != nil || !ok {
		t.Errorf("rule must pass when params name the checking user, got ok=%v err=%v", ok, err)
	}
	// ...vetoes when they name somebody else, and vetoes on nil params,
	// where no author is named at all.
	if ok, _ := m.CheckAccess("42", "post/update-own", Params{"author": "13"}); ok {
		t.Error("rule must veto when the params point at another author")
	}
	mustAccess(t, m, "42", "post/update-own", false)
}

// TestRuleRegistryAlignment covers the explicit failure demanded of a
// database whose rule names have no running implementation, and the AddRule
// path that persists a name and thereby resolves the misalignment.
func TestRuleRegistryAlignment(t *testing.T) {
	m, _, conn := newTestManager(t)

	addRole(t, m, "editor")
	if err := m.AddItem(Item{Name: "post/update", Type: TypePermission, RuleName: "ghost-rule"}); err != nil {
		t.Fatalf("add permission: %v", err)
	}
	assign(t, m, "editor", "1")
	grant(t, m, "editor", "post/update")

	// The permission references a rule name nothing implements: loading the
	// graph must fail loudly, naming the rule.
	_, err := m.CheckAccess("1", "post/update", nil)
	if err == nil || !strings.Contains(err.Error(), "ghost-rule") {
		t.Fatalf("unimplemented rule name must surface in a load error, got %v", err)
	}

	// Registering the implementation clears the error without any manager
	// rebuild.
	if err := m.AddRule(funcRule{name: "ghost-rule", fn: func(string, Params) bool { return true }}); err != nil {
		t.Fatalf("AddRule: %v", err)
	}
	mustAccess(t, m, "1", "post/update", true)

	// The name row persisted: a fresh manager construction over the same
	// registry requirement must still validate...
	var rules []authRuleModel
	if err := conn.DB.Find(&rules).Error; err != nil {
		t.Fatalf("read auth_rule: %v", err)
	}
	if len(rules) != 1 || rules[0].Name != "ghost-rule" {
		t.Errorf("auth_rule must hold the persisted name ghost-rule, got %+v", rules)
	}

	// ...while a manager without any implementations again refuses the name.
	bare, err := NewDbManager(conn.DB)
	if err != nil {
		t.Fatalf("NewDbManager: %v", err)
	}
	if _, err := bare.CheckAccess("1", "post/update", nil); err == nil || !strings.Contains(err.Error(), "ghost-rule") {
		t.Errorf("bare manager must reject the unimplemented rule name, got %v", err)
	}
}

// TestRuleStoredNameWithoutItemAlignment inserts a bare name row into
// auth_rule with nothing implementing it — the startup alignment must reject
// it even though no item references it.
func TestRuleStoredNameWithoutItemAlignment(t *testing.T) {
	m, _, conn := newTestManager(t)
	if err := conn.DB.Create(&authRuleModel{Name: "lonely", CreatedAt: 1}).Error; err != nil {
		t.Fatalf("seed rule row: %v", err)
	}
	addRole(t, m, "admin")
	assign(t, m, "admin", "1")

	if _, err := m.CheckAccess("1", "admin", nil); err == nil || !strings.Contains(err.Error(), "lonely") {
		t.Fatalf("stored name without implementation must fail graph load naming it, got %v", err)
	}
}

// TestNewDbManagerRejectsDuplicateRules guards the registry construction.
func TestNewDbManagerRejectsDuplicateRules(t *testing.T) {
	r := funcRule{name: "same", fn: func(string, Params) bool { return true }}
	if _, err := NewDbManager(nil, r, r); err == nil {
		t.Error("duplicate rule names in the registry must be rejected")
	}
	if _, err := NewDbManager(nil); err == nil {
		t.Error("a nil connection must be rejected")
	}
}

// --- assignments --------------------------------------------------------

// TestAssignRevokePersistence walks the assignment lifecycle and asserts
// every state change is visible to a manager with cold caches reading the
// same database (a restarted process).
func TestAssignRevokePersistence(t *testing.T) {
	m, _, conn := newTestManager(t)
	// coldView returns a manager that has read nothing yet — the restarted
	// process, so cached state can never mask what the rows actually say.
	coldView := func() *DbManager {
		fresh, err := NewDbManager(conn.DB)
		if err != nil {
			t.Fatalf("NewDbManager (cold): %v", err)
		}
		return fresh
	}
	addRole(t, m, "admin")
	addRole(t, m, "editor")
	addPermission(t, m, "post/delete", "")

	assign(t, m, "admin", "1")
	assign(t, m, "post/delete", "1") // permissions can be assigned directly

	roles, err := coldView().RolesByUser("1")
	if err != nil {
		t.Fatalf("cold RolesByUser: %v", err)
	}
	if got := nameList(roles); len(got) != 1 || got[0] != "admin" {
		t.Errorf("cold RolesByUser = %v, want [admin] (permissions are not roles)", got)
	}
	mustAccess(t, coldView(), "1", "post/delete", true)

	// Duplicate assignment is an explicit error, matching Yii2.
	if err := m.Assign("admin", "1"); !errors.Is(err, ErrAlreadyAssigned) {
		t.Errorf("re-assign must report ErrAlreadyAssigned, got %v", err)
	}
	// Assigning an unknown item is an explicit error too.
	if err := m.Assign("ghost", "1"); !errors.Is(err, ErrItemNotFound) {
		t.Errorf("assigning an unknown item must report ErrItemNotFound, got %v", err)
	}
	// Blank ids are refused outright.
	if err := m.Assign("", "1"); err == nil {
		t.Error("blank item name must be refused")
	}
	if err := m.Assign("admin", ""); err == nil {
		t.Error("blank user id must be refused")
	}

	if err := m.Revoke("post/delete", "1"); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	mustAccess(t, coldView(), "1", "post/delete", false)
	// Revoking something not held succeeds silently.
	if err := m.Revoke("editor", "1"); err != nil {
		t.Errorf("revoking an absent assignment must succeed, got %v", err)
	}

	assign(t, m, "editor", "2")
	if err := m.RevokeAll("2"); err != nil {
		t.Fatalf("revokeAll: %v", err)
	}
	roles, err = coldView().RolesByUser("2")
	if err != nil {
		t.Fatalf("cold RolesByUser after revokeAll: %v", err)
	}
	if len(roles) != 0 {
		t.Errorf("RolesByUser after revokeAll = %v, want none", nameList(roles))
	}
}

// TestRolesByUserDirectOnly asserts the listing stays direct assignments,
// never inherited roles.
func TestRolesByUserDirectOnly(t *testing.T) {
	m, _, _ := newTestManager(t)
	addRole(t, m, "admin")
	addRole(t, m, "editor")
	grant(t, m, "admin", "editor")
	assign(t, m, "admin", "1")

	roles, err := m.RolesByUser("1")
	if err != nil {
		t.Fatalf("RolesByUser: %v", err)
	}
	if got := nameList(roles); len(got) != 1 || got[0] != "admin" {
		t.Errorf("RolesByUser = %v, want [admin]; inherited editor must not appear", got)
	}
}

// --- children and items -------------------------------------------------

// TestChildrenMaintenance covers Children listing and edge removal, and that
// a removed edge immediately stops granting.
func TestChildrenMaintenance(t *testing.T) {
	m, fresh, _ := newTestManager(t)
	addRole(t, m, "admin")
	addPermission(t, m, "post/delete", "")
	addPermission(t, m, "post/create", "")
	grant(t, m, "admin", "post/delete")
	grant(t, m, "admin", "post/create")

	kids, err := m.Children("admin")
	if err != nil {
		t.Fatalf("Children: %v", err)
	}
	if got := nameList(kids); len(got) != 2 || got[0] != "post/create" || got[1] != "post/delete" {
		t.Errorf("Children(admin) = %v, want [post/create post/delete]", got)
	}

	// Unknown parents report no children rather than an error.
	kids, err = m.Children("ghost")
	if err != nil || len(kids) != 0 {
		t.Errorf("Children(ghost) = %v, %v; want none, nil", kids, err)
	}

	assign(t, m, "admin", "1")
	mustAccess(t, m, "1", "post/delete", true)
	if err := m.RemoveChild("admin", "post/delete"); err != nil {
		t.Fatalf("removeChild: %v", err)
	}
	mustAccess(t, m, "1", "post/delete", false)
	mustAccess(t, m, "1", "post/create", true)
	// Removing an absent edge succeeds silently.
	if err := m.RemoveChild("admin", "post/delete"); err != nil {
		t.Errorf("removing an absent edge must succeed, got %v", err)
	}
	// Same verdict through a freshly loaded cache.
	mustAccess(t, fresh, "1", "post/delete", false)
}

// TestItemLifecycle covers add/get/remove including the cascade an item's
// removal owes its edges and assignments.
func TestItemLifecycle(t *testing.T) {
	m, fresh, _ := newTestManager(t)
	if err := m.AddItem(Item{Name: "admin", Type: TypeRole, Description: "runs the place"}); err != nil {
		t.Fatalf("add role: %v", err)
	}
	if err := m.AddItem(Item{Name: "admin", Type: TypeRole}); !errors.Is(err, ErrItemExists) {
		t.Errorf("duplicate add must report ErrItemExists, got %v", err)
	}
	if err := m.AddItem(Item{Name: "bad", Type: ItemType(9)}); err == nil {
		t.Error("unknown item type must be refused")
	}
	if err := m.AddItem(Item{Name: "", Type: TypeRole}); err == nil {
		t.Error("blank item name must be refused")
	}

	it, err := m.GetItem("admin")
	if err != nil || it.Name != "admin" || it.Type != TypeRole || it.Description != "runs the place" {
		t.Errorf("GetItem = %+v, %v", it, err)
	}
	if _, err := m.GetItem("ghost"); !errors.Is(err, ErrItemNotFound) {
		t.Errorf("GetItem(ghost) must report ErrItemNotFound, got %v", err)
	}

	addPermission(t, m, "post/delete", "")
	grant(t, m, "admin", "post/delete")
	assign(t, m, "admin", "1")
	mustAccess(t, m, "1", "post/delete", true)

	if err := m.RemoveItem("admin"); err != nil {
		t.Fatalf("removeItem: %v", err)
	}
	mustAccess(t, m, "1", "admin", false)
	mustAccess(t, m, "1", "post/delete", false)
	if roles, err := fresh.RolesByUser("1"); err != nil || len(roles) != 0 {
		t.Errorf("assignment must be gone after item removal, got %v, %v", roles, err)
	}
	if kids, err := fresh.Children("admin"); err != nil || len(kids) != 0 {
		t.Errorf("edges must be gone after item removal, got %v, %v", kids, err)
	}
	if err := m.RemoveItem("admin"); !errors.Is(err, ErrItemNotFound) {
		t.Errorf("removing an absent item must report ErrItemNotFound, got %v", err)
	}
}

// --- cache invalidation (acceptance-critical) ---------------------------

// TestCacheInvalidationMakesMutationsImmediatelyVisible is the cache
// consistency acceptance: after the manager has served checks (its graph and
// assignment caches are warm), every mutation — structural or assignment —
// must be reflected by the very next check on the same manager instance.
func TestCacheInvalidationMakesMutationsImmediatelyVisible(t *testing.T) {
	m, fresh, _ := newTestManager(t)

	// Warm both caches with negative checks.
	mustAccess(t, m, "1", "post/delete", false)
	if _, err := m.RolesByUser("1"); err != nil {
		t.Fatalf("warm RolesByUser: %v", err)
	}

	// Structural change: brand-new items, edge and assignment.
	addRole(t, m, "admin")
	addPermission(t, m, "post/delete", "")
	grant(t, m, "admin", "post/delete")
	mustAccess(t, m, "1", "post/delete", false) // still no assignment
	assign(t, m, "admin", "1")
	mustAccess(t, m, "1", "post/delete", true) // assignment change is visible at once
	if roles, err := m.RolesByUser("1"); err != nil || len(nameList(roles)) != 1 {
		t.Errorf("warm cache must list the new assignment, got %v, %v", roles, err)
	}

	// Revoke is visible at once.
	if err := m.Revoke("admin", "1"); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	mustAccess(t, m, "1", "post/delete", false)

	// Re-granting the assignment restores access, then removing the edge
	// (a structural change) takes it away again — both without rebuild.
	assign(t, m, "admin", "1")
	mustAccess(t, m, "1", "post/delete", true)
	if err := m.RemoveChild("admin", "post/delete"); err != nil {
		t.Fatalf("removeChild: %v", err)
	}
	mustAccess(t, m, "1", "post/delete", false)
	grant(t, m, "admin", "post/delete")
	mustAccess(t, m, "1", "post/delete", true)

	// Removing the item clears the assignment's reachability as well.
	if err := m.RemoveItem("post/delete"); err != nil {
		t.Fatalf("removeItem: %v", err)
	}
	mustAccess(t, m, "1", "post/delete", false)

	// At every step the warm manager agreed with a fresh one; assert it for
	// the final state too.
	mustAccess(t, fresh, "1", "post/delete", false)
	if roles, err := fresh.RolesByUser("1"); err != nil || len(nameList(roles)) != 1 {
		t.Errorf("fresh RolesByUser = %v, %v; the admin assignment survives without its item", roles, err)
	}
}

// TestGraphLoadFailsBeforeTablesExist asserts a missing schema surfaces as an
// error (not a silent deny): running the manager before migrating must be
// loud.
func TestGraphLoadFailsBeforeTablesExist(t *testing.T) {
	conn, err := db.New(db.Config{Driver: "sqlite", DSN: filepath.Join(t.TempDir(), "empty.db")})
	if err != nil {
		t.Fatalf("connect sqlite: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	m, err := NewDbManager(conn.DB)
	if err != nil {
		t.Fatalf("NewDbManager: %v", err)
	}
	if _, err := m.CheckAccess("1", "admin", nil); err == nil {
		t.Error("check against a missing schema must error, not deny silently")
	}
}

// --- concurrency --------------------------------------------------------

// TestConcurrentChecksAndMutations runs checks from many goroutines while
// other goroutines keep mutating assignments and the hierarchy. Under -race
// it guards the cache swap and invalidation paths; the verdicts converge on
// the final state once the churn stops.
func TestConcurrentChecksAndMutations(t *testing.T) {
	m, fresh, _ := newTestManager(t)
	addRole(t, m, "admin")
	addRole(t, m, "editor")
	addPermission(t, m, "post/delete", "")
	grant(t, m, "admin", "editor")
	grant(t, m, "editor", "post/delete")
	assign(t, m, "admin", "9")

	stop := make(chan struct{})
	var readers sync.WaitGroup
	for i := 0; i < 8; i++ {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				for _, item := range []string{"admin", "editor", "post/delete"} {
					ok, err := m.CheckAccess("9", item, nil)
					if err != nil {
						t.Errorf("concurrent CheckAccess(%s): %v", item, err)
						return
					}
					_ = ok // any verdict during churn, but never an error
				}
				if _, err := m.RolesByUser("9"); err != nil {
					t.Errorf("concurrent RolesByUser: %v", err)
					return
				}
			}
		}()
	}

	// Each writer churns its own hierarchy edge and its own user, so the
	// remove/add loop never collides with the same edge's unique index.
	churn := []struct{ parent, child string }{
		{"admin", "editor"},
		{"editor", "post/delete"},
	}
	var writers sync.WaitGroup
	for w := 0; w < len(churn); w++ {
		writers.Add(1)
		go func(w int) {
			defer writers.Done()
			user := fmt.Sprintf("user-%d", w)
			for i := 0; i < 50; i++ {
				if err := m.Assign("editor", user); err != nil {
					t.Errorf("concurrent assign: %v", err)
					return
				}
				if err := m.Revoke("editor", user); err != nil {
					t.Errorf("concurrent revoke: %v", err)
					return
				}
				if err := m.RemoveChild(churn[w].parent, churn[w].child); err != nil {
					t.Errorf("concurrent removeChild: %v", err)
					return
				}
				if err := m.AddChild(churn[w].parent, churn[w].child); err != nil {
					t.Errorf("concurrent addChild: %v", err)
					return
				}
			}
		}(w)
	}
	writers.Wait()
	close(stop)
	readers.Wait()

	// After the churn, both managers must agree on the restored world.
	mustAccess(t, m, "9", "post/delete", true)
	mustAccess(t, fresh, "9", "post/delete", true)
}

// TestDbManagerImplementsAuthManager pins the interface at compile time.
func TestDbManagerImplementsAuthManager(t *testing.T) {
	var _ AuthManager = (*DbManager)(nil)
}
