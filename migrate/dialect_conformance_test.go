package migrate

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

// The conformance matrix is a table-driven suite that walks every adapter for
// each "operation × input → expected SQL" row. Dialect-specific test files
// keep only real divergence (existence probes, locks, quirks); everything the
// adapters share in structure lives here, so a new case automatically covers
// all four adapters at once.

// conformanceDialects lists every adapter the matrix walks. Adding a dialect
// to this slice pulls it into every matrix case.
var conformanceDialects = []Dialect{
	MySQLDialect{},
	SQLiteDialect{},
	PostgreSQLDialect{},
	SQLServerDialect{},
}

// sqlCase is one matrix row whose operation returns a plain SQL string.
// Every dialect must be covered by want or wantUnsupported (wantError applies
// to all dialects), so a case that forgets a dialect fails loudly.
type sqlCase struct {
	name string
	op   func(d Dialect) (string, error)
	// want maps a dialect name to the exact SQL the operation must produce.
	want map[string]string
	// wantUnsupported maps a dialect name to the operation name of the
	// UnsupportedOperationError that dialect must return.
	wantUnsupported map[string]string
	// wantError is a substring that must appear in the error for every dialect.
	wantError string
}

// stmtCase is one matrix row whose operation returns a statement with bound
// arguments. Expected arguments are shared because the adapters only differ
// in quoting and placeholder style, never in argument order.
type stmtCase struct {
	name      string
	op        func(d Dialect) (SQLStatement, error)
	wantQuery map[string]string
	wantArgs  []any
	// wantError is a substring that must appear in the error for every dialect.
	wantError string
}

// wantAll builds the per-dialect SQL expectations in the fixed order
// mysql, sqlite, postgres, sqlserver.
func wantAll(mysql, sqlite, postgres, sqlserver string) map[string]string {
	return map[string]string{
		"mysql":     mysql,
		"sqlite":    sqlite,
		"postgres":  postgres,
		"sqlserver": sqlserver,
	}
}

// wantUnsupported marks the named dialects as expecting an
// UnsupportedOperationError carrying the given operation name.
func wantUnsupported(operation string, dialects ...string) map[string]string {
	expectations := make(map[string]string, len(dialects))
	for _, name := range dialects {
		expectations[name] = operation
	}
	return expectations
}

// colCase turns a column builder into a matrix row by building the column
// through the dialect under test.
func colCase(name string, build func(m *MigrationContext) *ColumnBuilder, want map[string]string, wantUnsupported map[string]string) sqlCase {
	return sqlCase{
		name: name,
		op: func(d Dialect) (string, error) {
			return d.BuildColumn(build(NewMigrationContext(nil, d)))
		},
		want:            want,
		wantUnsupported: wantUnsupported,
	}
}

func runSQLMatrix(t *testing.T, cases []sqlCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, d := range conformanceDialects {
				t.Run(d.Name(), func(t *testing.T) {
					got, err := tc.op(d)
					switch {
					case tc.wantError != "":
						if err == nil || !strings.Contains(err.Error(), tc.wantError) {
							t.Fatalf("error = %v, want error containing %q", err, tc.wantError)
						}
					case tc.wantUnsupported[d.Name()] != "":
						operation := tc.wantUnsupported[d.Name()]
						var unsupported *UnsupportedOperationError
						if !errors.As(err, &unsupported) {
							t.Fatalf("error = %v, want UnsupportedOperationError(%s)", err, operation)
						}
						if unsupported.Dialect != d.Name() || unsupported.Operation != operation {
							t.Fatalf("unsupported error = %s/%s, want %s/%s", unsupported.Dialect, unsupported.Operation, d.Name(), operation)
						}
					default:
						wantSQL, ok := tc.want[d.Name()]
						if !ok {
							t.Fatalf("case has no expectation for dialect %s", d.Name())
						}
						if err != nil {
							t.Fatalf("unexpected error: %v", err)
						}
						if got != wantSQL {
							t.Fatalf("SQL:\n got: %s\nwant: %s", got, wantSQL)
						}
					}
				})
			}
		})
	}
}

func runStmtMatrix(t *testing.T, cases []stmtCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, d := range conformanceDialects {
				t.Run(d.Name(), func(t *testing.T) {
					stmt, err := tc.op(d)
					if tc.wantError != "" {
						if err == nil || !strings.Contains(err.Error(), tc.wantError) {
							t.Fatalf("error = %v, want error containing %q", err, tc.wantError)
						}
						return
					}
					wantQuery, ok := tc.wantQuery[d.Name()]
					if !ok {
						t.Fatalf("case has no expectation for dialect %s", d.Name())
					}
					if err != nil {
						t.Fatalf("unexpected error: %v", err)
					}
					if stmt.Query != wantQuery {
						t.Fatalf("query:\n got: %s\nwant: %s", stmt.Query, wantQuery)
					}
					if len(stmt.Args) != len(tc.wantArgs) {
						t.Fatalf("args = %#v, want %#v", stmt.Args, tc.wantArgs)
					}
					for i, arg := range tc.wantArgs {
						if !reflect.DeepEqual(stmt.Args[i], arg) {
							t.Fatalf("args = %#v, want %#v", stmt.Args, tc.wantArgs)
						}
					}
				})
			}
		})
	}
}

func TestDialectConformanceNaming(t *testing.T) {
	runSQLMatrix(t, []sqlCase{
		{
			name: "dialect name",
			op:   func(d Dialect) (string, error) { return d.Name(), nil },
			want: wantAll("mysql", "sqlite", "postgres", "sqlserver"),
		},
		{
			name: "quote table",
			op:   func(d Dialect) (string, error) { return d.QuoteTable("article"), nil },
			want: wantAll("`article`", `"article"`, `"article"`, "[article]"),
		},
		{
			name: "quote table with schema",
			op:   func(d Dialect) (string, error) { return d.QuoteTable("app.article"), nil },
			want: wantAll("`app`.`article`", `"app"."article"`, `"app"."article"`, "[app].[article]"),
		},
		{
			name: "quote table star stays raw",
			op:   func(d Dialect) (string, error) { return d.QuoteTable("*"), nil },
			want: wantAll("*", "*", "*", "*"),
		},
		{
			name: "quote column",
			op:   func(d Dialect) (string, error) { return d.QuoteColumn("title"), nil },
			want: wantAll("`title`", `"title"`, `"title"`, "[title]"),
		},
		{
			name: "quote index column",
			op:   func(d Dialect) (string, error) { return d.QuoteIndexColumn("user_id"), nil },
			want: wantAll("`user_id`", `"user_id"`, `"user_id"`, "[user_id]"),
		},
		{
			name: "quote index column leaves function expression raw",
			op:   func(d Dialect) (string, error) { return d.QuoteIndexColumn("LOWER(email)"), nil },
			want: wantAll("LOWER(email)", "LOWER(email)", "LOWER(email)", "LOWER(email)"),
		},
		{
			name: "quote index column leaves json path raw",
			op:   func(d Dialect) (string, error) { return d.QuoteIndexColumn("data->>'$.user'"), nil },
			want: wantAll("data->>'$.user'", "data->>'$.user'", "data->>'$.user'", "data->>'$.user'"),
		},
		{
			name: "placeholder first",
			op:   func(d Dialect) (string, error) { return d.Placeholder(1), nil },
			want: wantAll("?", "?", "$1", "@p1"),
		},
		{
			name: "placeholder double digit",
			op:   func(d Dialect) (string, error) { return d.Placeholder(12), nil },
			want: wantAll("?", "?", "$12", "@p12"),
		},
	})
}

func TestDialectConformanceBuildColumn(t *testing.T) {
	runSQLMatrix(t, []sqlCase{
		// Type mappings.
		colCase("tiny integer", func(m *MigrationContext) *ColumnBuilder { return m.TinyInteger() },
			wantAll("TINYINT", "INTEGER", "SMALLINT", "TINYINT"), nil),
		colCase("small integer", func(m *MigrationContext) *ColumnBuilder { return m.SmallInteger() },
			wantAll("SMALLINT", "INTEGER", "SMALLINT", "SMALLINT"), nil),
		colCase("integer", func(m *MigrationContext) *ColumnBuilder { return m.Integer() },
			wantAll("INT", "INTEGER", "INTEGER", "INT"), nil),
		colCase("integer with display size", func(m *MigrationContext) *ColumnBuilder { return m.Integer(11) },
			wantAll("INT(11)", "INTEGER", "INTEGER", "INT"), nil),
		colCase("big integer", func(m *MigrationContext) *ColumnBuilder { return m.BigInteger() },
			wantAll("BIGINT", "INTEGER", "BIGINT", "BIGINT"), nil),
		colCase("string default size", func(m *MigrationContext) *ColumnBuilder { return m.String() },
			wantAll("VARCHAR(255)", "VARCHAR(255)", "VARCHAR(255)", "NVARCHAR(255)"), nil),
		colCase("string sized", func(m *MigrationContext) *ColumnBuilder { return m.String(128) },
			wantAll("VARCHAR(128)", "VARCHAR(128)", "VARCHAR(128)", "NVARCHAR(128)"), nil),
		colCase("char", func(m *MigrationContext) *ColumnBuilder { return m.Char(10) },
			wantAll("CHAR(10)", "CHAR(10)", "CHAR(10)", "NCHAR(10)"), nil),
		colCase("text", func(m *MigrationContext) *ColumnBuilder { return m.Text() },
			wantAll("TEXT", "TEXT", "TEXT", "NVARCHAR(MAX)"), nil),
		colCase("tiny text", func(m *MigrationContext) *ColumnBuilder { return m.TinyText() },
			wantAll("TINYTEXT", "TEXT", "TEXT", "NVARCHAR(MAX)"), nil),
		colCase("medium text", func(m *MigrationContext) *ColumnBuilder { return m.MediumText() },
			wantAll("MEDIUMTEXT", "TEXT", "TEXT", "NVARCHAR(MAX)"), nil),
		colCase("long text", func(m *MigrationContext) *ColumnBuilder { return m.LongText() },
			wantAll("LONGTEXT", "TEXT", "TEXT", "NVARCHAR(MAX)"), nil),
		colCase("binary default size", func(m *MigrationContext) *ColumnBuilder { return m.Binary() },
			wantAll("VARBINARY", "BLOB", "BYTEA", "VARBINARY"), nil),
		colCase("binary sized", func(m *MigrationContext) *ColumnBuilder { return m.Binary(255) },
			wantAll("VARBINARY(255)", "BLOB", "BYTEA", "VARBINARY(255)"), nil),
		colCase("tiny blob", func(m *MigrationContext) *ColumnBuilder { return m.TinyBlob() },
			wantAll("TINYBLOB", "BLOB", "BYTEA", "VARBINARY(MAX)"), nil),
		colCase("medium blob", func(m *MigrationContext) *ColumnBuilder { return m.MediumBlob() },
			wantAll("MEDIUMBLOB", "BLOB", "BYTEA", "VARBINARY(MAX)"), nil),
		colCase("long blob", func(m *MigrationContext) *ColumnBuilder { return m.LongBlob() },
			wantAll("LONGBLOB", "BLOB", "BYTEA", "VARBINARY(MAX)"), nil),
		colCase("boolean", func(m *MigrationContext) *ColumnBuilder { return m.Boolean() },
			wantAll("TINYINT(1)", "INTEGER", "BOOLEAN", "BIT"), nil),
		colCase("float", func(m *MigrationContext) *ColumnBuilder { return m.Float() },
			wantAll("FLOAT", "REAL", "REAL", "FLOAT"), nil),
		colCase("float with precision", func(m *MigrationContext) *ColumnBuilder { return m.Float(53) },
			wantAll("FLOAT(53)", "REAL", "REAL", "FLOAT(53)"), nil),
		colCase("double", func(m *MigrationContext) *ColumnBuilder { return m.Double() },
			wantAll("DOUBLE", "REAL", "DOUBLE PRECISION", "DOUBLE PRECISION"), nil),
		colCase("decimal", func(m *MigrationContext) *ColumnBuilder { return m.Decimal(10, 2) },
			wantAll("DECIMAL(10, 2)", "NUMERIC(10, 2)", "DECIMAL(10, 2)", "DECIMAL(10, 2)"), nil),
		colCase("money", func(m *MigrationContext) *ColumnBuilder { return m.Money(10, 2) },
			wantAll("DECIMAL(10, 2)", "NUMERIC(10, 2)", "MONEY", "MONEY"), nil),
		colCase("date", func(m *MigrationContext) *ColumnBuilder { return m.Date() },
			wantAll("DATE", "TEXT", "DATE", "DATE"), nil),
		colCase("datetime", func(m *MigrationContext) *ColumnBuilder { return m.DateTime() },
			wantAll("DATETIME", "TEXT", "TIMESTAMP", "DATETIME2"), nil),
		colCase("datetime with precision", func(m *MigrationContext) *ColumnBuilder { return m.DateTime(3) },
			wantAll("DATETIME(3)", "TEXT", "TIMESTAMP(3)", "DATETIME2(3)"), nil),
		colCase("time", func(m *MigrationContext) *ColumnBuilder { return m.Time() },
			wantAll("TIME", "TEXT", "TIME", "TIME"), nil),
		colCase("time with precision", func(m *MigrationContext) *ColumnBuilder { return m.Time(6) },
			wantAll("TIME(6)", "TEXT", "TIME(6)", "TIME(6)"), nil),
		colCase("timestamp", func(m *MigrationContext) *ColumnBuilder { return m.Timestamp() },
			wantAll("TIMESTAMP", "TEXT", "TIMESTAMP", "DATETIME2"), nil),
		colCase("timestamp with precision", func(m *MigrationContext) *ColumnBuilder { return m.Timestamp(0) },
			wantAll("TIMESTAMP(0)", "TEXT", "TIMESTAMP(0)", "DATETIME2(0)"), nil),
		colCase("json", func(m *MigrationContext) *ColumnBuilder { return m.Json() },
			wantAll("JSON", "TEXT", "JSONB", "NVARCHAR(MAX)"), nil),
		colCase("uuid", func(m *MigrationContext) *ColumnBuilder { return m.UUID() },
			wantAll("CHAR(36)", "TEXT", "UUID", "UNIQUEIDENTIFIER"), nil),
		colCase("enum", func(m *MigrationContext) *ColumnBuilder { return m.Enum("a", "b") },
			map[string]string{"mysql": "ENUM('a', 'b')", "sqlite": "TEXT", "postgres": "TEXT"},
			wantUnsupported("enum", "sqlserver")),
		colCase("set", func(m *MigrationContext) *ColumnBuilder { return m.Set("x", "y") },
			map[string]string{"mysql": "SET('x', 'y')", "sqlite": "TEXT", "postgres": "TEXT"},
			wantUnsupported("set", "sqlserver")),
		// A type name no adapter maps must never be spliced into SQL
		// verbatim: every adapter rejects it with the same unsupported error.
		colCase("unmapped column type", func(m *MigrationContext) *ColumnBuilder { return newColumnBuilder("geometry") },
			nil,
			wantUnsupported("geometry", "mysql", "sqlite", "postgres", "sqlserver")),

		// Primary key and auto-increment identities.
		colCase("integer primary key", func(m *MigrationContext) *ColumnBuilder { return m.PrimaryKey() },
			wantAll(
				"INT NOT NULL AUTO_INCREMENT PRIMARY KEY",
				"INTEGER PRIMARY KEY AUTOINCREMENT",
				"SERIAL PRIMARY KEY",
				"INT IDENTITY(1,1) PRIMARY KEY NOT NULL",
			), nil),
		colCase("big primary key", func(m *MigrationContext) *ColumnBuilder { return m.BigPrimaryKey() },
			wantAll(
				"BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY",
				"INTEGER PRIMARY KEY AUTOINCREMENT",
				"BIGSERIAL PRIMARY KEY",
				"BIGINT IDENTITY(1,1) PRIMARY KEY NOT NULL",
			), nil),
		colCase("unsigned integer", func(m *MigrationContext) *ColumnBuilder { return m.UnsignedInteger() },
			map[string]string{"mysql": "INT UNSIGNED", "sqlite": "INTEGER"},
			wantUnsupported("UNSIGNED", "postgres", "sqlserver")),
		colCase("unsigned big primary key", func(m *MigrationContext) *ColumnBuilder { return m.UnsignedBigPrimaryKey() },
			map[string]string{
				"mysql":  "BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY",
				"sqlite": "INTEGER PRIMARY KEY AUTOINCREMENT",
			},
			wantUnsupported("UNSIGNED", "postgres", "sqlserver")),

		// Modifiers.
		colCase("not null", func(m *MigrationContext) *ColumnBuilder { return m.String(64).NotNull() },
			wantAll("VARCHAR(64) NOT NULL", "VARCHAR(64) NOT NULL", "VARCHAR(64) NOT NULL", "NVARCHAR(64) NOT NULL"), nil),
		colCase("null explicit", func(m *MigrationContext) *ColumnBuilder { return m.String(64).Null() },
			wantAll("VARCHAR(64) NULL", "VARCHAR(64) NULL", "VARCHAR(64) NULL", "NVARCHAR(64) NULL"), nil),
		colCase("default value", func(m *MigrationContext) *ColumnBuilder { return m.Integer().DefaultValue(42) },
			wantAll("INT DEFAULT 42", "INTEGER DEFAULT 42", "INTEGER DEFAULT 42", "INT DEFAULT 42"), nil),
		colCase("default expression", func(m *MigrationContext) *ColumnBuilder {
			return m.Timestamp(0).DefaultExpression("CURRENT_TIMESTAMP")
		},
			wantAll(
				"TIMESTAMP(0) DEFAULT CURRENT_TIMESTAMP",
				"TEXT DEFAULT CURRENT_TIMESTAMP",
				"TIMESTAMP(0) DEFAULT CURRENT_TIMESTAMP",
				"DATETIME2(0) DEFAULT CURRENT_TIMESTAMP",
			), nil),
		colCase("unique", func(m *MigrationContext) *ColumnBuilder { return m.String(64).Unique() },
			wantAll("VARCHAR(64) UNIQUE", "VARCHAR(64) UNIQUE", "VARCHAR(64) UNIQUE", "NVARCHAR(64) UNIQUE"), nil),
		colCase("check", func(m *MigrationContext) *ColumnBuilder { return m.Integer().Check("id > 0") },
			wantAll("INT CHECK (id > 0)", "INTEGER CHECK (id > 0)", "INTEGER CHECK (id > 0)", "INT CHECK (id > 0)"), nil),
		colCase("generated as", func(m *MigrationContext) *ColumnBuilder { return m.Integer().GeneratedAs("id * 2") },
			map[string]string{
				"mysql":    "INT GENERATED ALWAYS AS (id * 2)",
				"sqlite":   "INTEGER GENERATED ALWAYS AS (id * 2)",
				"postgres": "INTEGER GENERATED ALWAYS AS (id * 2)",
			},
			wantUnsupported("GENERATED AS", "sqlserver")),
		colCase("generated stored", func(m *MigrationContext) *ColumnBuilder { return m.Integer().GeneratedAs("id * 2").Stored() },
			map[string]string{
				"mysql":    "INT GENERATED ALWAYS AS (id * 2) STORED",
				"sqlite":   "INTEGER GENERATED ALWAYS AS (id * 2) STORED",
				"postgres": "INTEGER GENERATED ALWAYS AS (id * 2) STORED",
			},
			wantUnsupported("GENERATED AS", "sqlserver")),
		colCase("append raw sql", func(m *MigrationContext) *ColumnBuilder { return m.Integer().Append("USING id::integer") },
			wantAll("INT USING id::integer", "INTEGER USING id::integer", "INTEGER USING id::integer", "INT USING id::integer"), nil),
		colCase("charset", func(m *MigrationContext) *ColumnBuilder { return m.String(64).Charset("utf8mb4") },
			map[string]string{"mysql": "VARCHAR(64) CHARACTER SET utf8mb4"},
			wantUnsupported("CHARACTER SET", "sqlite", "postgres", "sqlserver")),
		colCase("collation", func(m *MigrationContext) *ColumnBuilder { return m.String(64).Collate("utf8mb4_unicode_ci") },
			map[string]string{"mysql": "VARCHAR(64) COLLATE utf8mb4_unicode_ci"},
			wantUnsupported("COLLATE", "sqlite", "postgres", "sqlserver")),
		colCase("after column", func(m *MigrationContext) *ColumnBuilder { return m.String(64).After("id") },
			map[string]string{"mysql": "VARCHAR(64) AFTER `id`"},
			wantUnsupported("AFTER", "sqlite", "postgres", "sqlserver")),
		colCase("first column", func(m *MigrationContext) *ColumnBuilder { return m.String(64).First() },
			map[string]string{"mysql": "VARCHAR(64) FIRST"},
			wantUnsupported("FIRST", "sqlite", "postgres", "sqlserver")),
		// Inline column comments only exist on MySQL; PostgreSQL and SQL Server
		// emit comments through separate statements, so the modifier renders
		// nothing there, while SQLite rejects it.
		colCase("column comment", func(m *MigrationContext) *ColumnBuilder { return m.String(64).Comment("The title") },
			map[string]string{"mysql": "VARCHAR(64) COMMENT 'The title'", "postgres": "VARCHAR(64)", "sqlserver": "NVARCHAR(64)"},
			wantUnsupported("COLUMN COMMENT", "sqlite")),
		{
			name:      "nil column builder",
			op:        func(d Dialect) (string, error) { return d.BuildColumn(nil) },
			wantError: "column builder is nil",
		},
	})
}

func TestDialectConformanceSchema(t *testing.T) {
	articleColumns := func(m *MigrationContext) *ColumnList {
		return Columns().
			Add("id", m.PrimaryKey()).
			Add("title", m.String(128).NotNull()).
			Add("metadata", m.Json().Null()).
			Add("created_at", m.Timestamp(0).NotNull().DefaultExpression("CURRENT_TIMESTAMP"))
	}

	runSQLMatrix(t, []sqlCase{
		{
			name: "create table",
			op: func(d Dialect) (string, error) {
				return d.CreateTable("article", articleColumns(NewMigrationContext(nil, d)), "")
			},
			want: wantAll(
				"CREATE TABLE `article` (`id` INT NOT NULL AUTO_INCREMENT PRIMARY KEY, `title` VARCHAR(128) NOT NULL, `metadata` JSON NULL, `created_at` TIMESTAMP(0) NOT NULL DEFAULT CURRENT_TIMESTAMP)",
				`CREATE TABLE "article" ("id" INTEGER PRIMARY KEY AUTOINCREMENT, "title" VARCHAR(128) NOT NULL, "metadata" TEXT NULL, "created_at" TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP)`,
				`CREATE TABLE "article" ("id" SERIAL PRIMARY KEY, "title" VARCHAR(128) NOT NULL, "metadata" JSONB NULL, "created_at" TIMESTAMP(0) NOT NULL DEFAULT CURRENT_TIMESTAMP)`,
				"CREATE TABLE [article] ([id] INT IDENTITY(1,1) PRIMARY KEY NOT NULL, [title] NVARCHAR(128) NOT NULL, [metadata] NVARCHAR(MAX) NULL, [created_at] DATETIME2(0) NOT NULL DEFAULT CURRENT_TIMESTAMP)",
			),
		},
		{
			name: "create table with options",
			op: func(d Dialect) (string, error) {
				return d.CreateTable("article", Columns().Add("id", NewMigrationContext(nil, d).PrimaryKey()), "WITH (fillfactor = 90)")
			},
			want: map[string]string{
				"mysql":    "CREATE TABLE `article` (`id` INT NOT NULL AUTO_INCREMENT PRIMARY KEY) WITH (fillfactor = 90)",
				"postgres": `CREATE TABLE "article" ("id" SERIAL PRIMARY KEY) WITH (fillfactor = 90)`,
			},
			wantUnsupported: wantUnsupported("TABLE OPTIONS", "sqlite", "sqlserver"),
		},
		{
			name: "create migration table",
			op:   func(d Dialect) (string, error) { return d.CreateMigrationTableSQL("migration") },
			want: wantAll(
				"CREATE TABLE IF NOT EXISTS `migration` (`version` varchar(180) NOT NULL PRIMARY KEY, `apply_time` int NOT NULL)",
				`CREATE TABLE IF NOT EXISTS "migration" ("version" varchar(180) NOT NULL PRIMARY KEY, "apply_time" int NOT NULL)`,
				`CREATE TABLE IF NOT EXISTS "migration" ("version" varchar(180) NOT NULL PRIMARY KEY, "apply_time" int NOT NULL)`,
				"IF OBJECT_ID(N'[migration]', 'U') IS NULL BEGIN CREATE TABLE [migration] ([version] varchar(180) NOT NULL PRIMARY KEY, [apply_time] int NOT NULL) END",
			),
		},
		{
			// Every adapter drops strictly: no IF EXISTS, so a missing object
			// surfaces as the database's own error. Idempotent drops are the
			// plan layer's job (DropTableIfExists probes first).
			name: "drop table",
			op:   func(d Dialect) (string, error) { return d.DropTable("article") },
			want: wantAll("DROP TABLE `article`", `DROP TABLE "article"`, `DROP TABLE "article"`, "DROP TABLE [article]"),
		},
		{
			name: "rename table",
			op:   func(d Dialect) (string, error) { return d.RenameTable("article", "post") },
			want: wantAll(
				"RENAME TABLE `article` TO `post`",
				`ALTER TABLE "article" RENAME TO "post"`,
				`ALTER TABLE "article" RENAME TO "post"`,
				"EXEC sp_rename 'article', 'post'",
			),
		},
		{
			name:            "truncate table",
			op:              func(d Dialect) (string, error) { return d.TruncateTable("article") },
			want:            map[string]string{"mysql": "TRUNCATE TABLE `article`", "postgres": `TRUNCATE TABLE "article"`, "sqlserver": "TRUNCATE TABLE [article]"},
			wantUnsupported: wantUnsupported("TRUNCATE TABLE", "sqlite"),
		},
		{
			name: "add column text null",
			op: func(d Dialect) (string, error) {
				return d.AddColumn("article", "summary", NewMigrationContext(nil, d).Text().Null())
			},
			want: wantAll(
				"ALTER TABLE `article` ADD COLUMN `summary` TEXT NULL",
				`ALTER TABLE "article" ADD COLUMN "summary" TEXT NULL`,
				`ALTER TABLE "article" ADD COLUMN "summary" TEXT NULL`,
				"ALTER TABLE [article] ADD [summary] NVARCHAR(MAX) NULL",
			),
		},
		{
			name: "add column string not null",
			op: func(d Dialect) (string, error) {
				return d.AddColumn("article", "summary", NewMigrationContext(nil, d).String(255).NotNull())
			},
			want: wantAll(
				"ALTER TABLE `article` ADD COLUMN `summary` VARCHAR(255) NOT NULL",
				`ALTER TABLE "article" ADD COLUMN "summary" VARCHAR(255) NOT NULL`,
				`ALTER TABLE "article" ADD COLUMN "summary" VARCHAR(255) NOT NULL`,
				"ALTER TABLE [article] ADD [summary] NVARCHAR(255) NOT NULL",
			),
		},
		{
			name: "alter column",
			op: func(d Dialect) (string, error) {
				return d.AlterColumn("article", "title", NewMigrationContext(nil, d).String(256))
			},
			want: map[string]string{
				"mysql":     "ALTER TABLE `article` MODIFY COLUMN `title` VARCHAR(256)",
				"postgres":  `ALTER TABLE "article" ALTER COLUMN "title" TYPE VARCHAR(256)`,
				"sqlserver": "ALTER TABLE [article] ALTER COLUMN [title] NVARCHAR(256)",
			},
			wantUnsupported: wantUnsupported("ALTER COLUMN", "sqlite"),
		},
		{
			// PostgreSQL rewrites the column type only, so nullability
			// modifiers do not survive an ALTER COLUMN there.
			name: "alter column not null",
			op: func(d Dialect) (string, error) {
				return d.AlterColumn("article", "title", NewMigrationContext(nil, d).String(256).NotNull())
			},
			want: map[string]string{
				"mysql":     "ALTER TABLE `article` MODIFY COLUMN `title` VARCHAR(256) NOT NULL",
				"postgres":  `ALTER TABLE "article" ALTER COLUMN "title" TYPE VARCHAR(256)`,
				"sqlserver": "ALTER TABLE [article] ALTER COLUMN [title] NVARCHAR(256) NOT NULL",
			},
			wantUnsupported: wantUnsupported("ALTER COLUMN", "sqlite"),
		},
		{
			// An unmapped type surfaces through ALTER COLUMN as well: the
			// three adapters that can alter reject the type itself, while
			// SQLite rejects the whole operation first.
			name: "alter column unmapped type",
			op: func(d Dialect) (string, error) {
				return d.AlterColumn("article", "shape", newColumnBuilder("geometry"))
			},
			wantUnsupported: map[string]string{
				"mysql":     "geometry",
				"postgres":  "geometry",
				"sqlserver": "geometry",
				"sqlite":    "ALTER COLUMN",
			},
		},
		{
			name:            "drop column",
			op:              func(d Dialect) (string, error) { return d.DropColumn("article", "summary") },
			want:            map[string]string{"mysql": "ALTER TABLE `article` DROP COLUMN `summary`", "postgres": `ALTER TABLE "article" DROP COLUMN "summary"`, "sqlserver": "ALTER TABLE [article] DROP COLUMN [summary]"},
			wantUnsupported: wantUnsupported("DROP COLUMN", "sqlite"),
		},
		{
			name:            "rename column",
			op:              func(d Dialect) (string, error) { return d.RenameColumn("article", "title", "heading") },
			want:            map[string]string{"mysql": "ALTER TABLE `article` RENAME COLUMN `title` TO `heading`", "postgres": `ALTER TABLE "article" RENAME COLUMN "title" TO "heading"`, "sqlserver": "EXEC sp_rename 'article.title', 'heading', 'COLUMN'"},
			wantUnsupported: wantUnsupported("RENAME COLUMN", "sqlite"),
		},
		{
			name: "create index",
			op: func(d Dialect) (string, error) {
				return d.CreateIndex("idx_article_title", "article", []string{"title"}, false)
			},
			want: wantAll(
				"CREATE INDEX `idx_article_title` ON `article` (`title`)",
				`CREATE INDEX "idx_article_title" ON "article" ("title")`,
				`CREATE INDEX "idx_article_title" ON "article" ("title")`,
				"CREATE INDEX [idx_article_title] ON [article] ([title])",
			),
		},
		{
			name: "create unique index",
			op: func(d Dialect) (string, error) {
				return d.CreateIndex("idx_article_slug", "article", []string{"slug"}, true)
			},
			want: wantAll(
				"CREATE UNIQUE INDEX `idx_article_slug` ON `article` (`slug`)",
				`CREATE UNIQUE INDEX "idx_article_slug" ON "article" ("slug")`,
				`CREATE UNIQUE INDEX "idx_article_slug" ON "article" ("slug")`,
				"CREATE UNIQUE INDEX [idx_article_slug] ON [article] ([slug])",
			),
		},
		{
			name: "create composite index",
			op: func(d Dialect) (string, error) {
				return d.CreateIndex("idx_article_user", "article", []string{"user_id", "created_at"}, false)
			},
			want: wantAll(
				"CREATE INDEX `idx_article_user` ON `article` (`user_id`, `created_at`)",
				`CREATE INDEX "idx_article_user" ON "article" ("user_id", "created_at")`,
				`CREATE INDEX "idx_article_user" ON "article" ("user_id", "created_at")`,
				"CREATE INDEX [idx_article_user] ON [article] ([user_id], [created_at])",
			),
		},
		{
			name: "create index on expression",
			op: func(d Dialect) (string, error) {
				return d.CreateIndex("idx_article_lower", "article", []string{"LOWER(title)"}, false)
			},
			want: wantAll(
				"CREATE INDEX `idx_article_lower` ON `article` (LOWER(title))",
				`CREATE INDEX "idx_article_lower" ON "article" (LOWER(title))`,
				`CREATE INDEX "idx_article_lower" ON "article" (LOWER(title))`,
				"CREATE INDEX [idx_article_lower] ON [article] (LOWER(title))",
			),
		},
		{
			// Every adapter drops strictly: no IF EXISTS, a missing index is
			// the database's own error, idempotency lives in the plan layer.
			// The table argument is used only where the engine's DROP INDEX
			// syntax needs it: MySQL and SQL Server emit "ON table", while
			// PostgreSQL and SQLite drop the index by name alone.
			name: "drop index",
			op:   func(d Dialect) (string, error) { return d.DropIndex("idx_article_title", "article") },
			want: wantAll(
				"DROP INDEX `idx_article_title` ON `article`",
				`DROP INDEX "idx_article_title"`,
				`DROP INDEX "idx_article_title"`,
				"DROP INDEX [idx_article_title] ON [article]",
			),
		},
		{
			name:            "add primary key",
			op:              func(d Dialect) (string, error) { return d.AddPrimaryKey("pk_article", "article", []string{"id"}) },
			want:            map[string]string{"mysql": "ALTER TABLE `article` ADD CONSTRAINT `pk_article` PRIMARY KEY (`id`)", "postgres": `ALTER TABLE "article" ADD CONSTRAINT "pk_article" PRIMARY KEY ("id")`, "sqlserver": "ALTER TABLE [article] ADD CONSTRAINT [pk_article] PRIMARY KEY ([id])"},
			wantUnsupported: wantUnsupported("ADD PRIMARY KEY", "sqlite"),
		},
		{
			name:            "drop primary key",
			op:              func(d Dialect) (string, error) { return d.DropPrimaryKey("pk_article", "article") },
			want:            map[string]string{"mysql": "ALTER TABLE `article` DROP PRIMARY KEY", "postgres": `ALTER TABLE "article" DROP CONSTRAINT "pk_article"`, "sqlserver": "ALTER TABLE [article] DROP CONSTRAINT [pk_article]"},
			wantUnsupported: wantUnsupported("DROP PRIMARY KEY", "sqlite"),
		},
		{
			name: "add foreign key",
			op: func(d Dialect) (string, error) {
				return d.AddForeignKey("fk_article_user", "article", []string{"user_id"}, "user", []string{"id"}, Cascade, NoAction)
			},
			want: map[string]string{
				"mysql":     "ALTER TABLE `article` ADD CONSTRAINT `fk_article_user` FOREIGN KEY (`user_id`) REFERENCES `user` (`id`) ON DELETE CASCADE ON UPDATE NO ACTION",
				"postgres":  `ALTER TABLE "article" ADD CONSTRAINT "fk_article_user" FOREIGN KEY ("user_id") REFERENCES "user" ("id") ON DELETE CASCADE ON UPDATE NO ACTION`,
				"sqlserver": "ALTER TABLE [article] ADD CONSTRAINT [fk_article_user] FOREIGN KEY ([user_id]) REFERENCES [user] ([id]) ON DELETE CASCADE ON UPDATE NO ACTION",
			},
			wantUnsupported: wantUnsupported("ADD FOREIGN KEY", "sqlite"),
		},
		{
			name: "add foreign key without actions",
			op: func(d Dialect) (string, error) {
				return d.AddForeignKey("fk_article_user", "article", []string{"user_id"}, "user", []string{"id"}, "", "")
			},
			want: map[string]string{
				"mysql":     "ALTER TABLE `article` ADD CONSTRAINT `fk_article_user` FOREIGN KEY (`user_id`) REFERENCES `user` (`id`)",
				"postgres":  `ALTER TABLE "article" ADD CONSTRAINT "fk_article_user" FOREIGN KEY ("user_id") REFERENCES "user" ("id")`,
				"sqlserver": "ALTER TABLE [article] ADD CONSTRAINT [fk_article_user] FOREIGN KEY ([user_id]) REFERENCES [user] ([id])",
			},
			wantUnsupported: wantUnsupported("ADD FOREIGN KEY", "sqlite"),
		},
		{
			name:            "drop foreign key",
			op:              func(d Dialect) (string, error) { return d.DropForeignKey("fk_article_user", "article") },
			want:            map[string]string{"mysql": "ALTER TABLE `article` DROP FOREIGN KEY `fk_article_user`", "postgres": `ALTER TABLE "article" DROP CONSTRAINT "fk_article_user"`, "sqlserver": "ALTER TABLE [article] DROP CONSTRAINT [fk_article_user]"},
			wantUnsupported: wantUnsupported("DROP FOREIGN KEY", "sqlite"),
		},
		{
			name: "add comment on column",
			op:   func(d Dialect) (string, error) { return d.AddCommentOnColumn("article", "title", "The title") },
			want: map[string]string{
				"mysql":     "ALTER TABLE `article` MODIFY COLUMN `title` COMMENT 'The title'",
				"postgres":  `COMMENT ON COLUMN "article"."title" IS 'The title'`,
				"sqlserver": "EXEC sp_addextendedproperty 'MS_Description', 'The title', 'SCHEMA', SCHEMA_NAME(), 'TABLE', 'article', 'COLUMN', 'title'",
			},
			wantUnsupported: wantUnsupported("COLUMN COMMENT", "sqlite"),
		},
		{
			name: "drop comment from column",
			op:   func(d Dialect) (string, error) { return d.DropCommentFromColumn("article", "title") },
			want: map[string]string{
				"mysql":     "ALTER TABLE `article` MODIFY COLUMN `title` COMMENT ''",
				"postgres":  `COMMENT ON COLUMN "article"."title" IS NULL`,
				"sqlserver": "EXEC sp_dropextendedproperty 'MS_Description', 'SCHEMA', SCHEMA_NAME(), 'TABLE', 'article', 'COLUMN', 'title'",
			},
			wantUnsupported: wantUnsupported("COLUMN COMMENT", "sqlite"),
		},
		{
			name: "add comment on table",
			op:   func(d Dialect) (string, error) { return d.AddCommentOnTable("article", "Articles table") },
			want: map[string]string{
				"mysql":     "ALTER TABLE `article` COMMENT = 'Articles table'",
				"postgres":  `COMMENT ON TABLE "article" IS 'Articles table'`,
				"sqlserver": "EXEC sp_addextendedproperty 'MS_Description', 'Articles table', 'SCHEMA', SCHEMA_NAME(), 'TABLE', 'article'",
			},
			wantUnsupported: wantUnsupported("TABLE COMMENT", "sqlite"),
		},
		{
			name: "drop comment from table",
			op:   func(d Dialect) (string, error) { return d.DropCommentFromTable("article") },
			want: map[string]string{
				"mysql":     "ALTER TABLE `article` COMMENT = ''",
				"postgres":  `COMMENT ON TABLE "article" IS NULL`,
				"sqlserver": "EXEC sp_dropextendedproperty 'MS_Description', 'SCHEMA', SCHEMA_NAME(), 'TABLE', 'article'",
			},
			wantUnsupported: wantUnsupported("TABLE COMMENT", "sqlite"),
		},

		// Query helpers used by the migration context.
		{
			name: "row exists",
			op:   func(d Dialect) (string, error) { return d.BuildRowExistsSQL("user", "username = 'admin'"), nil },
			want: wantAll(
				"SELECT EXISTS(SELECT 1 FROM `user` WHERE username = 'admin')",
				`SELECT EXISTS(SELECT 1 FROM "user" WHERE username = 'admin')`,
				`SELECT EXISTS(SELECT 1 FROM "user" WHERE username = 'admin')`,
				"SELECT CASE WHEN EXISTS(SELECT 1 FROM [user] WHERE username = 'admin') THEN 1 ELSE 0 END",
			),
		},
		{
			name: "row exists without condition",
			op:   func(d Dialect) (string, error) { return d.BuildRowExistsSQL("user", ""), nil },
			want: wantAll(
				"SELECT EXISTS(SELECT 1 FROM `user`)",
				`SELECT EXISTS(SELECT 1 FROM "user")`,
				`SELECT EXISTS(SELECT 1 FROM "user")`,
				"SELECT CASE WHEN EXISTS(SELECT 1 FROM [user]) THEN 1 ELSE 0 END",
			),
		},
		{
			name: "count rows",
			op:   func(d Dialect) (string, error) { return d.BuildCountRowsSQL("article", "status = 'draft'"), nil },
			want: wantAll(
				"SELECT COUNT(*) FROM `article` WHERE status = 'draft'",
				`SELECT COUNT(*) FROM "article" WHERE status = 'draft'`,
				`SELECT COUNT(*) FROM "article" WHERE status = 'draft'`,
				"SELECT COUNT(*) FROM [article] WHERE status = 'draft'",
			),
		},
	})
}

func TestDialectConformanceDML(t *testing.T) {
	runStmtMatrix(t, []stmtCase{
		{
			name: "insert with expression and value",
			op: func(d Dialect) (SQLStatement, error) {
				return d.Insert("article", Row{
					"title":      "Hello",
					"created_at": Expr("CURRENT_TIMESTAMP"),
				})
			},
			wantQuery: wantAll(
				"INSERT INTO `article` (`created_at`, `title`) VALUES (CURRENT_TIMESTAMP, ?)",
				`INSERT INTO "article" ("created_at", "title") VALUES (CURRENT_TIMESTAMP, ?)`,
				`INSERT INTO "article" ("created_at", "title") VALUES (CURRENT_TIMESTAMP, $1)`,
				"INSERT INTO [article] ([created_at], [title]) VALUES (CURRENT_TIMESTAMP, @p1)",
			),
			wantArgs: []any{"Hello"},
		},
		{
			name:      "insert empty row",
			op:        func(d Dialect) (SQLStatement, error) { return d.Insert("article", Row{}) },
			wantError: "insert row is empty",
		},
		{
			name: "batch insert",
			op: func(d Dialect) (SQLStatement, error) {
				return d.BatchInsert("article", []string{"title", "status"}, [][]any{
					{"Hello", "draft"},
					{"World", "published"},
				})
			},
			wantQuery: wantAll(
				"INSERT INTO `article` (`title`, `status`) VALUES (?, ?), (?, ?)",
				`INSERT INTO "article" ("title", "status") VALUES (?, ?), (?, ?)`,
				`INSERT INTO "article" ("title", "status") VALUES ($1, $2), ($3, $4)`,
				"INSERT INTO [article] ([title], [status]) VALUES (@p1, @p2), (@p3, @p4)",
			),
			wantArgs: []any{"Hello", "draft", "World", "published"},
		},
		{
			name:      "batch insert empty columns",
			op:        func(d Dialect) (SQLStatement, error) { return d.BatchInsert("article", nil, [][]any{{"Hello"}}) },
			wantError: "batch insert columns are empty",
		},
		{
			name:      "batch insert empty rows",
			op:        func(d Dialect) (SQLStatement, error) { return d.BatchInsert("article", []string{"title"}, nil) },
			wantError: "batch insert rows are empty",
		},
		{
			name: "batch insert row width mismatch",
			op: func(d Dialect) (SQLStatement, error) {
				return d.BatchInsert("article", []string{"title", "status", "created_at"}, [][]any{{"Hello", "draft"}})
			},
			wantError: "2 values for 3 columns",
		},
		{
			name: "update with condition",
			op: func(d Dialect) (SQLStatement, error) {
				return d.Update("article", Row{"title": "Updated"}, "status = 'draft'")
			},
			wantQuery: wantAll(
				"UPDATE `article` SET `title` = ? WHERE status = 'draft'",
				`UPDATE "article" SET "title" = ? WHERE status = 'draft'`,
				`UPDATE "article" SET "title" = $1 WHERE status = 'draft'`,
				"UPDATE [article] SET [title] = @p1 WHERE status = 'draft'",
			),
			wantArgs: []any{"Updated"},
		},
		{
			name: "update without condition",
			op: func(d Dialect) (SQLStatement, error) {
				return d.Update("article", Row{"title": "All"}, "")
			},
			wantQuery: wantAll(
				"UPDATE `article` SET `title` = ?",
				`UPDATE "article" SET "title" = ?`,
				`UPDATE "article" SET "title" = $1`,
				"UPDATE [article] SET [title] = @p1",
			),
			wantArgs: []any{"All"},
		},
		{
			name: "update with expression value",
			op: func(d Dialect) (SQLStatement, error) {
				return d.Update("article", Row{"views": Expr("views + 1")}, "")
			},
			wantQuery: wantAll(
				"UPDATE `article` SET `views` = views + 1",
				`UPDATE "article" SET "views" = views + 1`,
				`UPDATE "article" SET "views" = views + 1`,
				"UPDATE [article] SET [views] = views + 1",
			),
		},
		{
			name:      "update empty row",
			op:        func(d Dialect) (SQLStatement, error) { return d.Update("article", Row{}, "status = 'draft'") },
			wantError: "update row is empty",
		},
		{
			name:      "delete with condition",
			op:        func(d Dialect) (SQLStatement, error) { return d.Delete("article", "status = 'draft'") },
			wantQuery: wantAll("DELETE FROM `article` WHERE status = 'draft'", `DELETE FROM "article" WHERE status = 'draft'`, `DELETE FROM "article" WHERE status = 'draft'`, "DELETE FROM [article] WHERE status = 'draft'"),
		},
		{
			name:      "delete without condition",
			op:        func(d Dialect) (SQLStatement, error) { return d.Delete("article", "") },
			wantQuery: wantAll("DELETE FROM `article`", `DELETE FROM "article"`, `DELETE FROM "article"`, "DELETE FROM [article]"),
		},
	})
}
