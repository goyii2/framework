package rbac

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
)

// DbManager is the database-backed AuthManager: authorization items, their
// hierarchy and user assignments live in the four auth tables (created by
// NewMigration), accessed through GORM.
//
// The item graph — every item plus both directions of the hierarchy — is
// loaded into memory on first use and reused by every access check; direct
// assignments are cached per user the same way. Mutations write through to
// the database and invalidate exactly the cache they affect, so a check
// issued after a mutation returns reflects it, on the same manager instance.
// The caches are process-local: deployments where several processes share
// one database see each other's structural changes only after their own next
// invalidation or restart — the price of checking against memory, paid
// deliberately.
//
// All database access is serialized under the manager lock, which also keeps
// single-writer engines such as SQLite happy under concurrency.
type DbManager struct {
	db *gorm.DB

	mu sync.RWMutex
	// graph is the loaded item graph; nil means not loaded yet. A loaded
	// graph is immutable — invalidation swaps in a new one, in-flight checks
	// finish on their own snapshot.
	graph *itemGraph
	// assignments caches each user's directly assigned item names by user
	// id; a missing entry is re-read from the database on demand. Stored
	// slices are never mutated after insertion.
	assignments map[string][]string
	// rules is the code registry of rule implementations. The map is
	// rebuilt and swapped whole by AddRule; readers copy the header out
	// under the read lock and never see a mutation.
	rules map[string]Rule
}

// NewDbManager builds the manager over an opened GORM connection. The rules
// argument is the process's rule registry: implementations for every rule
// name the database references must be present there — graph loading fails
// loudly naming any stored or referenced rule without an implementation.
// Construction performs no database access; the tables must exist by the
// time operations run (see NewMigration).
func NewDbManager(db *gorm.DB, rules ...Rule) (*DbManager, error) {
	registry := make(map[string]Rule, len(rules))
	for _, rule := range rules {
		if rule == nil {
			return nil, fmt.Errorf("rbac: rule registry contains a nil rule")
		}
		name := rule.Name()
		if name == "" {
			return nil, fmt.Errorf("rbac: rule %T has an empty name", rule)
		}
		if _, dup := registry[name]; dup {
			return nil, fmt.Errorf("rbac: rule %q registered twice", name)
		}
		registry[name] = rule
	}
	if db == nil {
		return nil, fmt.Errorf("rbac: DbManager requires a database connection")
	}
	return &DbManager{db: db, rules: registry}, nil
}

// itemGraph is the immutable in-memory snapshot of auth_item and
// auth_item_child.
type itemGraph struct {
	items map[string]*itemNode
}

// itemNode is one item with its place in the hierarchy: children are the
// items this one grants; parents are the ones granting it. Access checks
// walk parents (toward the user's assignments); the child direction feeds
// Children listings.
type itemNode struct {
	item     Item
	children []string
	parents  []string
}

// query returns a clean statement chain off the shared connection so
// conditions never leak between operations.
func (m *DbManager) query() *gorm.DB {
	return m.db.Session(&gorm.Session{NewDB: true})
}

// --- the check ----------------------------------------------------------

// CheckAccess reports whether userID holds itemName. The check walks the
// hierarchy upward from the requested item toward the user's direct
// assignments: the item is held when it is assigned directly, or when any
// item that contains it (transitively, through any number of levels and
// paths) is. Rules attached to the items along the matched path execute as
// part of the walk; a rule returning false vetoes its item and everything
// that item would grant. A false without error is a clean denial; an error
// means the check could not run.
func (m *DbManager) CheckAccess(userID string, itemName string, params Params) (bool, error) {
	g, err := m.loadGraph()
	if err != nil {
		return false, err
	}
	names, err := m.loadAssignments(userID)
	if err != nil {
		return false, err
	}
	if len(names) == 0 {
		return false, nil
	}
	assigned := make(map[string]struct{}, len(names))
	for _, name := range names {
		assigned[name] = struct{}{}
	}
	rules := m.ruleSet()
	return checkAccess(g, rules, userID, itemName, params, assigned, make(map[string]struct{})), nil
}

// checkAccess is the lock-free recursive verdict over immutable snapshots:
// rule lookups and executions never need the manager lock, so rules may call
// back into the manager without deadlocking. The visited set bounds the walk
// even against a hierarchy someone managed to make cyclic behind the
// manager's back.
func checkAccess(g *itemGraph, rules map[string]Rule, userID, itemName string, params Params, assigned map[string]struct{}, visited map[string]struct{}) bool {
	if _, seen := visited[itemName]; seen {
		return false
	}
	visited[itemName] = struct{}{}
	node, ok := g.items[itemName]
	if !ok {
		return false
	}
	if node.item.RuleName != "" {
		rule := rules[node.item.RuleName]
		if rule == nil {
			// Unreachable past load-time registry validation; refusing beats
			// granting when a rule is missing anyway.
			return false
		}
		if !rule.Execute(userID, params) {
			return false
		}
	}
	if _, ok := assigned[itemName]; ok {
		return true
	}
	for _, parent := range node.parents {
		if checkAccess(g, rules, userID, parent, params, assigned, visited) {
			return true
		}
	}
	return false
}

// ruleSet copies the current registry map header out under the read lock.
func (m *DbManager) ruleSet() map[string]Rule {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.rules
}

// --- assignments --------------------------------------------------------

// Assign grants itemName to userID directly. Granting twice is an explicit
// error, as is assigning an unknown item; blank names are refused outright.
func (m *DbManager) Assign(itemName string, userID string) error {
	if itemName == "" || userID == "" {
		return fmt.Errorf("rbac: assign requires an item name and a user id")
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	// Existence via count: Take would log its not-found as a query error on
	// what is a routine denial here.
	var known int64
	if err := m.query().Model(&authItemModel{}).Where("name = ?", itemName).
		Count(&known).Error; err != nil {
		return fmt.Errorf("rbac: assign %q: %w", itemName, err)
	}
	if known == 0 {
		return fmt.Errorf("rbac: assign %q: %w", itemName, ErrItemNotFound)
	}

	var existing int64
	if err := m.query().Model(&authAssignmentModel{}).
		Where("item_name = ? AND user_id = ?", itemName, userID).
		Count(&existing).Error; err != nil {
		return fmt.Errorf("rbac: assign %q: %w", itemName, err)
	}
	if existing > 0 {
		return fmt.Errorf("rbac: assign %q to %q: %w", itemName, userID, ErrAlreadyAssigned)
	}

	row := authAssignmentModel{ItemName: itemName, UserID: userID, CreatedAt: time.Now().Unix()}
	if err := m.query().Create(&row).Error; err != nil {
		return fmt.Errorf("rbac: assign %q to %q: %w", itemName, userID, err)
	}
	delete(m.assignments, userID)
	return nil
}

// Revoke removes one direct assignment. Revoking something the user does not
// hold succeeds silently.
func (m *DbManager) Revoke(itemName string, userID string) error {
	if itemName == "" || userID == "" {
		return fmt.Errorf("rbac: revoke requires an item name and a user id")
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.query().
		Where("item_name = ? AND user_id = ?", itemName, userID).
		Delete(&authAssignmentModel{}).Error; err != nil {
		return fmt.Errorf("rbac: revoke %q from %q: %w", itemName, userID, err)
	}
	delete(m.assignments, userID)
	return nil
}

// RevokeAll removes every direct assignment of userID.
func (m *DbManager) RevokeAll(userID string) error {
	if userID == "" {
		return fmt.Errorf("rbac: revokeAll requires a user id")
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.query().Where("user_id = ?", userID).
		Delete(&authAssignmentModel{}).Error; err != nil {
		return fmt.Errorf("rbac: revoke all from %q: %w", userID, err)
	}
	delete(m.assignments, userID)
	return nil
}

// RolesByUser returns the roles directly assigned to userID. Direct
// assignments only — roles reachable only through the hierarchy are not
// listed, and directly assigned permissions are not roles.
func (m *DbManager) RolesByUser(userID string) ([]Item, error) {
	names, err := m.loadAssignments(userID)
	if err != nil {
		return nil, err
	}
	g, err := m.loadGraph()
	if err != nil {
		return nil, err
	}
	roles := make([]Item, 0, len(names))
	for _, name := range names {
		if node, ok := g.items[name]; ok && node.item.Type == TypeRole {
			roles = append(roles, node.item)
		}
	}
	sort.Slice(roles, func(i, j int) bool { return roles[i].Name < roles[j].Name })
	return roles, nil
}

// --- items --------------------------------------------------------------

// GetItem returns the item stored under name.
func (m *DbManager) GetItem(name string) (Item, error) {
	if name == "" {
		return Item{}, fmt.Errorf("rbac: get item: %w", ErrItemNotFound)
	}
	var row authItemModel
	err := m.query().Where("name = ?", name).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Item{}, fmt.Errorf("rbac: get item %q: %w", name, ErrItemNotFound)
	}
	if err != nil {
		return Item{}, fmt.Errorf("rbac: get item %q: %w", name, err)
	}
	return Item{Name: row.Name, Type: ItemType(row.Type), Description: row.Description, RuleName: row.RuleName}, nil
}

// AddItem stores a new role or permission. The name must be unique and the
// type one of the two known kinds.
func (m *DbManager) AddItem(item Item) error {
	if item.Name == "" {
		return fmt.Errorf("rbac: add item: the name is blank")
	}
	if item.Type != TypeRole && item.Type != TypePermission {
		return fmt.Errorf("rbac: add item %q: unknown item type %d", item.Name, item.Type)
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	var existing int64
	if err := m.query().Model(&authItemModel{}).Where("name = ?", item.Name).
		Count(&existing).Error; err != nil {
		return fmt.Errorf("rbac: add item %q: %w", item.Name, err)
	}
	if existing > 0 {
		return fmt.Errorf("rbac: add item %q: %w", item.Name, ErrItemExists)
	}

	now := time.Now().Unix()
	row := authItemModel{
		Name:        item.Name,
		Type:        int(item.Type),
		Description: item.Description,
		RuleName:    item.RuleName,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := m.query().Create(&row).Error; err != nil {
		return fmt.Errorf("rbac: add item %q: %w", item.Name, err)
	}
	m.invalidateLocked()
	return nil
}

// RemoveItem deletes the item together with the hierarchy edges touching it
// and the assignments of it, then drops the caches.
func (m *DbManager) RemoveItem(name string) error {
	if name == "" {
		return fmt.Errorf("rbac: remove item: %w", ErrItemNotFound)
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	var known int64
	if err := m.query().Model(&authItemModel{}).Where("name = ?", name).
		Count(&known).Error; err != nil {
		return fmt.Errorf("rbac: remove item %q: %w", name, err)
	}
	if known == 0 {
		return fmt.Errorf("rbac: remove item %q: %w", name, ErrItemNotFound)
	}
	if err := m.query().Where("name = ?", name).Delete(&authItemModel{}).Error; err != nil {
		return fmt.Errorf("rbac: remove item %q: %w", name, err)
	}
	if err := m.query().Where("parent = ? OR child = ?", name, name).
		Delete(&authItemChildModel{}).Error; err != nil {
		return fmt.Errorf("rbac: remove item %q edges: %w", name, err)
	}
	if err := m.query().Where("item_name = ?", name).
		Delete(&authAssignmentModel{}).Error; err != nil {
		return fmt.Errorf("rbac: remove item %q assignments: %w", name, err)
	}
	m.invalidateLocked()
	return nil
}

// --- hierarchy ----------------------------------------------------------

// AddChild makes child part of parent: whoever holds parent — directly or
// through the hierarchy — holds child as well. The edge is refused when
// either item is missing, when parent and child are the same, when closing
// the edge would create a cycle (detected over the edges currently in the
// database, including ones other processes wrote), and when a permission
// would contain a role, the one combination Yii2 forbids because it makes
// permission checks grant roles.
func (m *DbManager) AddChild(parent string, child string) error {
	if parent == "" || child == "" {
		return fmt.Errorf("rbac: addChild requires a parent and a child name")
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	var rows []authItemModel
	if err := m.query().Where("name IN ?", []string{parent, child}).Find(&rows).Error; err != nil {
		return fmt.Errorf("rbac: addChild %s -> %s: %w", parent, child, err)
	}
	kinds := make(map[string]ItemType, len(rows))
	for _, row := range rows {
		kinds[row.Name] = ItemType(row.Type)
	}
	if _, ok := kinds[parent]; !ok {
		return fmt.Errorf("rbac: addChild %s -> %s: parent: %w", parent, child, ErrItemNotFound)
	}
	if _, ok := kinds[child]; !ok {
		return fmt.Errorf("rbac: addChild %s -> %s: child: %w", parent, child, ErrItemNotFound)
	}
	if parent == child {
		return fmt.Errorf("rbac: an item cannot become its own child: %q", parent)
	}
	if kinds[parent] == TypePermission && kinds[child] == TypeRole {
		return fmt.Errorf("rbac: addChild %s -> %s: a permission cannot contain a role", parent, child)
	}
	reaches, err := m.reachesOverDB(child, parent)
	if err != nil {
		return err
	}
	if reaches {
		return fmt.Errorf("rbac: addChild %s -> %s: the edge would close a cycle", parent, child)
	}

	if err := m.query().Create(&authItemChildModel{Parent: parent, Child: child}).Error; err != nil {
		return fmt.Errorf("rbac: addChild %s -> %s: %w", parent, child, err)
	}
	m.invalidateLocked()
	return nil
}

// RemoveChild dissolves the parent-child relationship. Removing an absent
// edge succeeds silently.
func (m *DbManager) RemoveChild(parent string, child string) error {
	if parent == "" || child == "" {
		return fmt.Errorf("rbac: removeChild requires a parent and a child name")
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.query().Where("parent = ? AND child = ?", parent, child).
		Delete(&authItemChildModel{}).Error; err != nil {
		return fmt.Errorf("rbac: removeChild %s -> %s: %w", parent, child, err)
	}
	m.invalidateLocked()
	return nil
}

// Children returns the items directly contained in parent, ordered by name.
func (m *DbManager) Children(parent string) ([]Item, error) {
	if parent == "" {
		return nil, nil
	}
	var rows []authItemModel
	err := m.query().Table("auth_item_child").
		Select("auth_item.name AS name, auth_item.type AS type, auth_item.description AS description, auth_item.rule_name AS rule_name").
		Joins("JOIN auth_item ON auth_item.name = auth_item_child.child").
		Where("auth_item_child.parent = ?", parent).
		Order("auth_item.name").
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("rbac: children of %q: %w", parent, err)
	}
	items := make([]Item, 0, len(rows))
	for _, row := range rows {
		items = append(items, Item{Name: row.Name, Type: ItemType(row.Type), Description: row.Description, RuleName: row.RuleName})
	}
	return items, nil
}

// reachesOverDB reports whether target is reachable from start along child
// edges as they stand in the database right now — the live-edge cycle guard
// AddChild checks against. The walk is breadth-first over the frontier, so
// deep hierarchies cost one query per level.
func (m *DbManager) reachesOverDB(start, target string) (bool, error) {
	if start == target {
		return true, nil
	}
	seen := map[string]struct{}{start: {}}
	frontier := []string{start}
	for len(frontier) > 0 {
		var edges []authItemChildModel
		if err := m.query().Where("parent IN ?", frontier).Find(&edges).Error; err != nil {
			return false, fmt.Errorf("rbac: walk item children: %w", err)
		}
		next := make([]string, 0, len(edges))
		for _, edge := range edges {
			if edge.Child == target {
				return true, nil
			}
			if _, dup := seen[edge.Child]; !dup {
				seen[edge.Child] = struct{}{}
				next = append(next, edge.Child)
			}
		}
		frontier = next
	}
	return false, nil
}

// --- rules --------------------------------------------------------------

// AddRule registers rule's implementation in the process registry and
// persists its name in auth_rule when the table does not carry it yet — the
// write side of keeping database names and code implementations aligned.
// Registering a rule of a name already present replaces the implementation
// and leaves the row alone.
func (m *DbManager) AddRule(rule Rule) error {
	if rule == nil {
		return fmt.Errorf("rbac: AddRule requires a rule")
	}
	name := rule.Name()
	if name == "" {
		return fmt.Errorf("rbac: rule %T has an empty name", rule)
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	next := make(map[string]Rule, len(m.rules)+1)
	for key, value := range m.rules {
		next[key] = value
	}
	next[name] = rule

	var existing int64
	if err := m.query().Model(&authRuleModel{}).Where("name = ?", name).
		Count(&existing).Error; err != nil {
		return fmt.Errorf("rbac: add rule %q: %w", name, err)
	}
	if existing == 0 {
		if err := m.query().Create(&authRuleModel{Name: name, CreatedAt: time.Now().Unix()}).Error; err != nil {
			return fmt.Errorf("rbac: add rule %q: %w", name, err)
		}
	}
	m.rules = next
	return nil
}

// --- cache loading and invalidation --------------------------------------

// loadGraph returns the item graph, loading and validating it on first use.
func (m *DbManager) loadGraph() (*itemGraph, error) {
	m.mu.RLock()
	g := m.graph
	m.mu.RUnlock()
	if g != nil {
		return g, nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.graphLocked()
}

// graphLocked loads the graph when needed; the caller holds the write lock.
func (m *DbManager) graphLocked() (*itemGraph, error) {
	if g := m.graph; g != nil {
		return g, nil
	}
	g, err := m.readGraph()
	if err != nil {
		return nil, err
	}
	m.graph = g
	return g, nil
}

// loadAssignments returns userID's directly assigned item names, reading and
// caching them on first use. The write lock covers the database read: every
// statement the manager issues runs under it, serializing database access
// in-process.
func (m *DbManager) loadAssignments(userID string) ([]string, error) {
	m.mu.RLock()
	names, ok := m.assignments[userID]
	m.mu.RUnlock()
	if ok {
		return names, nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if names, ok := m.assignments[userID]; ok {
		return names, nil
	}

	var rows []authAssignmentModel
	if err := m.query().Where("user_id = ?", userID).Order("item_name").
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("rbac: load assignments of %q: %w", userID, err)
	}
	names = make([]string, 0, len(rows))
	for _, row := range rows {
		names = append(names, row.ItemName)
	}
	if m.assignments == nil {
		m.assignments = make(map[string][]string)
	}
	m.assignments[userID] = names
	return names, nil
}

// invalidateLocked drops both caches entirely; the caller holds the write
// lock. Structural changes go through it because they can affect any user's
// verdict; assignment changes only evict the one user's entry.
func (m *DbManager) invalidateLocked() {
	m.graph = nil
	m.assignments = nil
}

// readGraph builds the graph from the database and enforces the rule
// registry alignment: every rule name auth_rule stores and every rule name
// an item references must have a running implementation. Anything else fails
// the load, naming the offenders — a deployment whose database and code
// disagree about rules must not degrade into silent grants or denials.
func (m *DbManager) readGraph() (*itemGraph, error) {
	var itemRows []authItemModel
	if err := m.query().Order("name").Find(&itemRows).Error; err != nil {
		return nil, fmt.Errorf("rbac: load items: %w", err)
	}
	var edgeRows []authItemChildModel
	if err := m.query().Order("parent, child").Find(&edgeRows).Error; err != nil {
		return nil, fmt.Errorf("rbac: load item children: %w", err)
	}
	var ruleRows []authRuleModel
	if err := m.query().Order("name").Find(&ruleRows).Error; err != nil {
		return nil, fmt.Errorf("rbac: load rule names: %w", err)
	}

	var unimplemented []string
	for _, row := range ruleRows {
		if m.rules[row.Name] == nil {
			unimplemented = append(unimplemented, row.Name)
		}
	}
	if len(unimplemented) > 0 {
		return nil, fmt.Errorf("rbac: auth_rule stores rules without an implementation: %s", strings.Join(unimplemented, ", "))
	}
	unimplemented = nil
	for _, row := range itemRows {
		if row.RuleName != "" && m.rules[row.RuleName] == nil {
			unimplemented = append(unimplemented, row.RuleName)
		}
	}
	if len(unimplemented) > 0 {
		sort.Strings(unimplemented)
		return nil, fmt.Errorf("rbac: items reference rules without an implementation: %s", strings.Join(dedup(unimplemented), ", "))
	}

	g := &itemGraph{items: make(map[string]*itemNode, len(itemRows))}
	for _, row := range itemRows {
		g.items[row.Name] = &itemNode{
			item: Item{Name: row.Name, Type: ItemType(row.Type), Description: row.Description, RuleName: row.RuleName},
		}
	}
	for _, edge := range edgeRows {
		parent, ok := g.items[edge.Parent]
		if !ok {
			return nil, fmt.Errorf("rbac: auth_item_child references missing item %q", edge.Parent)
		}
		child, ok := g.items[edge.Child]
		if !ok {
			return nil, fmt.Errorf("rbac: auth_item_child references missing item %q", edge.Child)
		}
		parent.children = append(parent.children, edge.Child)
		child.parents = append(child.parents, edge.Parent)
	}
	return g, nil
}

// dedup removes adjacent duplicates of a sorted list.
func dedup(sorted []string) []string {
	out := sorted[:0]
	var prev string
	for i, value := range sorted {
		if i == 0 || value != prev {
			out = append(out, value)
		}
		prev = value
	}
	return out
}
