package rbac

// The GORM row shapes of the four authorization tables. They mirror the Yii2
// schema (auth_item / auth_item_child / auth_assignment and auth_rule reduced
// to the stored names) with unexported types: everything outside the manager
// speaks in Item, Assignment and Rule values, never rows.

// authItemModel is one row of auth_item.
type authItemModel struct {
	Name        string `gorm:"column:name;primaryKey;size:64"`
	Type        int    `gorm:"column:type"`
	Description string `gorm:"column:description"`
	RuleName    string `gorm:"column:rule_name;size:64"`
	CreatedAt   int64  `gorm:"column:created_at"`
	UpdatedAt   int64  `gorm:"column:updated_at"`
}

func (authItemModel) TableName() string { return "auth_item" }

// authItemChildModel is one edge of auth_item_child: holding Parent implies
// holding Child.
type authItemChildModel struct {
	Parent string `gorm:"column:parent;primaryKey;size:64"`
	Child  string `gorm:"column:child;primaryKey;size:64"`
}

func (authItemChildModel) TableName() string { return "auth_item_child" }

// authRuleModel is one row of auth_rule. Only the name is meaningful: rule
// behavior lives exclusively in code, the table records which names the
// system knows.
type authRuleModel struct {
	Name      string `gorm:"column:name;primaryKey;size:64"`
	CreatedAt int64  `gorm:"column:created_at"`
}

func (authRuleModel) TableName() string { return "auth_rule" }

// authAssignmentModel is one row of auth_assignment: userID directly holds
// ItemName. The user id is a string because identities are string-keyed.
type authAssignmentModel struct {
	ItemName  string `gorm:"column:item_name;primaryKey;size:64"`
	UserID    string `gorm:"column:user_id;primaryKey;size:64"`
	CreatedAt int64  `gorm:"column:created_at"`
}

func (authAssignmentModel) TableName() string { return "auth_assignment" }
