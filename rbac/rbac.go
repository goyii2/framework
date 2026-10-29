package rbac

import "errors"

// ItemType distinguishes the two kinds of authorization items: roles group
// permissions (and other roles) and are assigned to users; permissions name
// what a holder may do. The values match the ones Yii2 stores in the type
// column of auth_item.
type ItemType int

// The two item types. A role may contain permissions and roles; a permission
// may contain permissions only.
const (
	TypeRole       ItemType = 1
	TypePermission ItemType = 2
)

// String renders the item type for logs and error messages.
func (t ItemType) String() string {
	switch t {
	case TypeRole:
		return "role"
	case TypePermission:
		return "permission"
	default:
		return "unknown"
	}
}

// Params carries the request context a Rule evaluates: typically the subject
// resource ("post" -> the post being edited) and anything else the rule wants.
// It is an alias so implementations satisfy consumer-side interfaces declared
// with plain map[string]any signatures.
type Params = map[string]any

// Item is one authorization item (role or permission) — the in-memory shape
// of an auth_item row. CreatedAt/UpdatedAt live in the database only.
type Item struct {
	// Name uniquely identifies the item ("admin", "post/delete").
	Name string
	// Type selects role or permission.
	Type ItemType
	// Description is free-form documentation.
	Description string
	// RuleName names the code-defined rule executed before the item grants
	// anything; empty means the item grants unconditionally.
	RuleName string
}

// Assignment records that a user directly holds an item (a role usually, a
// permission occasionally) — the in-memory shape of an auth_assignment row.
type Assignment struct {
	ItemName string
	UserID   string
}

// Rule is a code-defined condition attached to items. Rules are never
// persisted as behavior: the database stores only their names (auth_rule),
// and the running process supplies the implementations. Execute receives the
// user id being checked and the params of the check; returning false vetoes
// the item the rule is attached to, along with everything that item would
// grant downstream. Params may be nil.
type Rule interface {
	// Name identifies the rule; it must match the name stored on items.
	Name() string
	// Execute reports whether the user passes the rule for this check.
	Execute(userID string, params Params) bool
}

// Sentinel errors of the manager. They are comparable with errors.Is.
var (
	// ErrItemNotFound reports an operation naming an item the database does
	// not know (GetItem, RemoveItem, AddChild, Assign).
	ErrItemNotFound = errors.New("rbac: item not found")
	// ErrItemExists reports AddItem trying to create an existing name.
	ErrItemExists = errors.New("rbac: item already exists")
	// ErrAlreadyAssigned reports Assign on a user already holding the item.
	ErrAlreadyAssigned = errors.New("rbac: item already assigned to this user")
)

// AuthManager is the authorization backend contract: check access along the
// item hierarchy, maintain assignments and the hierarchy itself. DbManager
// implements it against the four auth tables; the interface is the seam other
// implementations (an in-memory manager, a Casbin adapter) plug into.
type AuthManager interface {
	// CheckAccess reports whether userID holds itemName, either by direct
	// assignment or through the item hierarchy: a holder of an item holds
	// everything below it. Rules attached to the items on the matched path
	// execute during the check and can veto the grant. An error means the
	// check could not run (missing tables, misaligned rule registry); false
	// without error is a clean denial.
	CheckAccess(userID string, itemName string, params Params) (bool, error)

	// Assign grants itemName to userID directly.
	Assign(itemName string, userID string) error
	// Revoke removes a direct assignment; revoking something the user does
	// not hold succeeds silently.
	Revoke(itemName string, userID string) error
	// RevokeAll removes every direct assignment of userID.
	RevokeAll(userID string) error
	// RolesByUser returns the roles directly assigned to userID (direct
	// assignments only; inherited roles are not listed).
	RolesByUser(userID string) ([]Item, error)

	// GetItem returns the item stored under name.
	GetItem(name string) (Item, error)
	// AddItem stores a new role or permission.
	AddItem(item Item) error
	// RemoveItem deletes the item together with its hierarchy edges and
	// assignments.
	RemoveItem(name string) error

	// AddChild makes child part of parent: whoever holds parent holds child
	// as well. Cycles, self-edges, missing items and a role below a
	// permission are rejected.
	AddChild(parent string, child string) error
	// RemoveChild dissolves such a relationship; removing an absent edge
	// succeeds silently.
	RemoveChild(parent string, child string) error
	// Children returns the items directly contained in parent.
	Children(parent string) ([]Item, error)

	// AddRule registers a rule implementation and persists its name in the
	// auth_rule table, keeping database and code registry aligned.
	AddRule(rule Rule) error
}
