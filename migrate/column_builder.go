package migrate

// ColumnBuilder describes a database column using an immutable chainable API.
type ColumnBuilder struct {
	typeName      string
	size          []int
	values        []string
	nullSet       bool
	nullable      bool
	unsigned      bool
	primaryKey    bool
	autoIncrement bool
	unique        bool
	defaultSet    bool
	defaultValue  any
	defaultExpr   string
	comment       string
	check         string
	after         string
	first         bool
	appendSQL     string
	charset       string
	collation     string
	generatedAs   string
	generatedKind string
}

func newColumnBuilder(typeName string, size ...int) *ColumnBuilder {
	return &ColumnBuilder{typeName: typeName, size: append([]int(nil), size...), nullable: true}
}

func (c *ColumnBuilder) clone() *ColumnBuilder {
	next := *c
	next.size = append([]int(nil), c.size...)
	next.values = append([]string(nil), c.values...)
	return &next
}

// Nullable reports whether the column is currently nullable.
func (c *ColumnBuilder) Nullable() bool {
	return c.nullable
}

// NotNull returns a copy of the builder that emits NOT NULL.
func (c *ColumnBuilder) NotNull() *ColumnBuilder {
	next := c.clone()
	next.nullSet = true
	next.nullable = false
	return next
}

// Null returns a copy of the builder that emits NULL.
func (c *ColumnBuilder) Null() *ColumnBuilder {
	next := c.clone()
	next.nullSet = true
	next.nullable = true
	return next
}

// Unsigned returns a copy of the builder marked as unsigned.
func (c *ColumnBuilder) Unsigned() *ColumnBuilder {
	next := c.clone()
	next.unsigned = true
	return next
}

// PrimaryKey returns a copy of the builder marked as a primary key.
func (c *ColumnBuilder) PrimaryKey() *ColumnBuilder {
	next := c.clone()
	next.primaryKey = true
	next.nullSet = true
	next.nullable = false
	return next
}

// AutoIncrement returns a copy of the builder marked as auto-incrementing.
func (c *ColumnBuilder) AutoIncrement() *ColumnBuilder {
	next := c.clone()
	next.autoIncrement = true
	return next
}

// Unique returns a copy of the builder marked as unique.
func (c *ColumnBuilder) Unique() *ColumnBuilder {
	next := c.clone()
	next.unique = true
	return next
}

// DefaultValue returns a copy of the builder with a literal default value.
func (c *ColumnBuilder) DefaultValue(v any) *ColumnBuilder {
	next := c.clone()
	next.defaultSet = true
	next.defaultValue = v
	next.defaultExpr = ""
	return next
}

// DefaultExpression returns a copy of the builder with a raw SQL default.
func (c *ColumnBuilder) DefaultExpression(sql string) *ColumnBuilder {
	next := c.clone()
	next.defaultSet = false
	next.defaultValue = nil
	next.defaultExpr = sql
	return next
}

// Comment returns a copy of the builder with a column comment.
func (c *ColumnBuilder) Comment(comment string) *ColumnBuilder {
	next := c.clone()
	next.comment = comment
	return next
}

// Check returns a copy of the builder with a CHECK constraint expression.
func (c *ColumnBuilder) Check(expr string) *ColumnBuilder {
	next := c.clone()
	next.check = expr
	return next
}

// After returns a copy of the builder positioned after another column.
func (c *ColumnBuilder) After(column string) *ColumnBuilder {
	next := c.clone()
	next.after = column
	next.first = false
	return next
}

// First returns a copy of the builder positioned first in the table.
func (c *ColumnBuilder) First() *ColumnBuilder {
	next := c.clone()
	next.first = true
	next.after = ""
	return next
}

// Append returns a copy of the builder with raw SQL appended.
func (c *ColumnBuilder) Append(sql string) *ColumnBuilder {
	next := c.clone()
	next.appendSQL = sql
	return next
}

// Charset returns a copy of the builder with a character set.
func (c *ColumnBuilder) Charset(charset string) *ColumnBuilder {
	next := c.clone()
	next.charset = charset
	return next
}

// Collate returns a copy of the builder with a collation.
func (c *ColumnBuilder) Collate(collation string) *ColumnBuilder {
	next := c.clone()
	next.collation = collation
	return next
}

// GeneratedAs returns a copy of the builder with a generated-column expression.
func (c *ColumnBuilder) GeneratedAs(expr string) *ColumnBuilder {
	next := c.clone()
	next.generatedAs = expr
	return next
}

// Stored returns a copy of the builder marked as a stored generated column.
func (c *ColumnBuilder) Stored() *ColumnBuilder {
	next := c.clone()
	next.generatedKind = "STORED"
	return next
}

// Virtual returns a copy of the builder marked as a virtual generated column.
func (c *ColumnBuilder) Virtual() *ColumnBuilder {
	next := c.clone()
	next.generatedKind = "VIRTUAL"
	return next
}

// PrimaryKey creates an integer primary key column.
func (m *MigrationContext) PrimaryKey() *ColumnBuilder {
	return newColumnBuilder("integer").PrimaryKey().AutoIncrement()
}

// BigPrimaryKey creates a big integer primary key column.
func (m *MigrationContext) BigPrimaryKey() *ColumnBuilder {
	return newColumnBuilder("bigInteger").PrimaryKey().AutoIncrement()
}

// UnsignedPrimaryKey creates an unsigned integer primary key column.
func (m *MigrationContext) UnsignedPrimaryKey() *ColumnBuilder {
	return m.PrimaryKey().Unsigned()
}

// UnsignedBigPrimaryKey creates an unsigned big integer primary key column.
func (m *MigrationContext) UnsignedBigPrimaryKey() *ColumnBuilder {
	return m.BigPrimaryKey().Unsigned()
}

// TinyInteger creates a tiny integer column.
func (m *MigrationContext) TinyInteger(length ...int) *ColumnBuilder {
	return newColumnBuilder("tinyInteger", length...)
}

// SmallInteger creates a small integer column.
func (m *MigrationContext) SmallInteger(length ...int) *ColumnBuilder {
	return newColumnBuilder("smallInteger", length...)
}

// Integer creates an integer column.
func (m *MigrationContext) Integer(length ...int) *ColumnBuilder {
	return newColumnBuilder("integer", length...)
}

// BigInteger creates a big integer column.
func (m *MigrationContext) BigInteger(length ...int) *ColumnBuilder {
	return newColumnBuilder("bigInteger", length...)
}

// UnsignedTinyInteger creates an unsigned tiny integer column.
func (m *MigrationContext) UnsignedTinyInteger(length ...int) *ColumnBuilder {
	return m.TinyInteger(length...).Unsigned()
}

// UnsignedSmallInteger creates an unsigned small integer column.
func (m *MigrationContext) UnsignedSmallInteger(length ...int) *ColumnBuilder {
	return m.SmallInteger(length...).Unsigned()
}

// UnsignedInteger creates an unsigned integer column.
func (m *MigrationContext) UnsignedInteger(length ...int) *ColumnBuilder {
	return m.Integer(length...).Unsigned()
}

// UnsignedBigInteger creates an unsigned big integer column.
func (m *MigrationContext) UnsignedBigInteger(length ...int) *ColumnBuilder {
	return m.BigInteger(length...).Unsigned()
}

// String creates a variable-length string column.
func (m *MigrationContext) String(size ...int) *ColumnBuilder {
	return newColumnBuilder("string", size...)
}

// Char creates a fixed-length string column.
func (m *MigrationContext) Char(size int) *ColumnBuilder {
	return newColumnBuilder("char", size)
}

// Text creates a text column.
func (m *MigrationContext) Text() *ColumnBuilder {
	return newColumnBuilder("text")
}

// TinyText creates a tiny text column.
func (m *MigrationContext) TinyText() *ColumnBuilder {
	return newColumnBuilder("tinyText")
}

// MediumText creates a medium text column.
func (m *MigrationContext) MediumText() *ColumnBuilder {
	return newColumnBuilder("mediumText")
}

// LongText creates a long text column.
func (m *MigrationContext) LongText() *ColumnBuilder {
	return newColumnBuilder("longText")
}

// Binary creates a binary column.
func (m *MigrationContext) Binary(length ...int) *ColumnBuilder {
	return newColumnBuilder("binary", length...)
}

// TinyBlob creates a tiny blob column.
func (m *MigrationContext) TinyBlob() *ColumnBuilder {
	return newColumnBuilder("tinyBlob")
}

// MediumBlob creates a medium blob column.
func (m *MigrationContext) MediumBlob() *ColumnBuilder {
	return newColumnBuilder("mediumBlob")
}

// LongBlob creates a long blob column.
func (m *MigrationContext) LongBlob() *ColumnBuilder {
	return newColumnBuilder("longBlob")
}

// Boolean creates a boolean column.
func (m *MigrationContext) Boolean() *ColumnBuilder {
	return newColumnBuilder("boolean")
}

// Float creates a float column.
func (m *MigrationContext) Float(precision ...int) *ColumnBuilder {
	return newColumnBuilder("float", precision...)
}

// Double creates a double column.
func (m *MigrationContext) Double(precision ...int) *ColumnBuilder {
	return newColumnBuilder("double", precision...)
}

// Decimal creates a decimal column.
func (m *MigrationContext) Decimal(precision, scale int) *ColumnBuilder {
	return newColumnBuilder("decimal", precision, scale)
}

// Money creates a money column.
func (m *MigrationContext) Money(precision, scale int) *ColumnBuilder {
	return newColumnBuilder("money", precision, scale)
}

// Date creates a date column.
func (m *MigrationContext) Date() *ColumnBuilder {
	return newColumnBuilder("date")
}

// DateTime creates a datetime column.
func (m *MigrationContext) DateTime(precision ...int) *ColumnBuilder {
	return newColumnBuilder("dateTime", precision...)
}

// Time creates a time column.
func (m *MigrationContext) Time(precision ...int) *ColumnBuilder {
	return newColumnBuilder("time", precision...)
}

// Timestamp creates a timestamp column.
func (m *MigrationContext) Timestamp(precision ...int) *ColumnBuilder {
	return newColumnBuilder("timestamp", precision...)
}

// Json creates a JSON column.
func (m *MigrationContext) Json() *ColumnBuilder {
	return newColumnBuilder("json")
}

// UUID creates a UUID column.
func (m *MigrationContext) UUID() *ColumnBuilder {
	return newColumnBuilder("uuid")
}

// Enum creates an enum column.
func (m *MigrationContext) Enum(values ...string) *ColumnBuilder {
	c := newColumnBuilder("enum")
	c.values = append([]string(nil), values...)
	return c
}

// Set creates a set column.
func (m *MigrationContext) Set(values ...string) *ColumnBuilder {
	c := newColumnBuilder("set")
	c.values = append([]string(nil), values...)
	return c
}
