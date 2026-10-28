package migrate

// ColumnDef is one ordered column definition in a table.
type ColumnDef struct {
	Name   string
	Column *ColumnBuilder
}

// ColumnList stores table columns in declaration order.
type ColumnList struct {
	items []ColumnDef
}

// Columns creates an empty ordered column list.
func Columns() *ColumnList {
	return &ColumnList{}
}

// Add appends a named column definition and returns the list.
func (c *ColumnList) Add(name string, column *ColumnBuilder) *ColumnList {
	c.items = append(c.items, ColumnDef{Name: name, Column: column})
	return c
}

// Items returns a copy of the ordered column definitions.
func (c *ColumnList) Items() []ColumnDef {
	if c == nil {
		return nil
	}
	return append([]ColumnDef(nil), c.items...)
}
