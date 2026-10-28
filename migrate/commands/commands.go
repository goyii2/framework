// Package commands provides the goyii migrate command tree: the eight
// subcommands (up, down, redo, history, new, mark, to, create) of the
// standalone yiimigrate CLI this framework's migrate package was merged
// from, here attached below the goyii console root command.
package commands

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strconv"
	"strings"

	_ "github.com/glebarez/go-sqlite"
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	_ "github.com/microsoft/go-mssqldb"
	"github.com/spf13/cobra"

	"github.com/goyii2/framework/base"
	"github.com/goyii2/framework/config"
	"github.com/goyii2/framework/db"
	"github.com/goyii2/framework/migrate"
	"github.com/goyii2/framework/migrate/internal/generator"
)

// New returns the "migrate" parent command with the eight subcommands.
// Database commands resolve their connection from the application's db
// configuration section; the DB_DSN and DB_DIALECT environment variables
// (and MIGRATE_TABLE / MIGRATE_DRY_RUN) keep their effect from the
// standalone yiimigrate CLI. migrations is the application's registered
// migration list, typically migrations.All().
func New(app *base.Application, migrations []migrate.Migration) *cobra.Command {
	root := &cobra.Command{
		Use:   "migrate",
		Short: "Apply and inspect versioned database migrations",
	}
	root.AddCommand(
		newLimitCommand(app, migrations, "up", "Apply pending migrations (default: all pending)", 0, runUp),
		newLimitCommand(app, migrations, "down", "Revert applied migrations (default: the most recent one)", 1, runDown),
		newLimitCommand(app, migrations, "redo", "Revert and re-apply migrations (default: the most recent one)", 1, runRedo),
		newLimitCommand(app, migrations, "history", "Show applied migrations (default: 10)", 10, runHistory),
		newLimitCommand(app, migrations, "new", "Show pending migrations (default: 10)", 10, runNew),
		newMarkCommand(app, migrations),
		newToCommand(app, migrations),
		newCreateCommand(app),
	)
	return root
}

// driverAliases maps configured dialect spellings (including the empty
// default) to canonical database/sql driver names. The dialect itself always
// comes from the migrate package registry, never from this table.
var driverAliases = map[string]string{
	"":           "mysql",
	"mysql":      "mysql",
	"sqlite":     "sqlite",
	"sqlite3":    "sqlite",
	"postgres":   "postgres",
	"postgresql": "postgres",
	"sqlserver":  "sqlserver",
	"mssql":      "sqlserver",
}

// resolveDriver resolves the configured dialect name to the database/sql
// driver name and the migrate dialect. An empty name resolves to mysql.
func resolveDriver(name string) (driverName string, dialect migrate.Dialect, err error) {
	driverName, ok := driverAliases[strings.ToLower(strings.TrimSpace(name))]
	if !ok {
		return "", nil, unsupportedDialectError(name)
	}
	dialect, ok = migrate.DialectByName(driverName)
	if !ok {
		return "", nil, unsupportedDialectError(name)
	}
	return driverName, dialect, nil
}

func unsupportedDialectError(name string) error {
	return fmt.Errorf("unsupported db dialect %q; supported values: %s", name, strings.Join(migrate.DialectNames(), ", "))
}

// resolveTarget turns the application's db configuration plus the
// DB_DSN / DB_DIALECT environment overrides into the driver name, dialect
// and DSN the database commands operate on. Either non-empty environment
// variable wins over the configuration value.
func resolveTarget(cfg config.DB) (driverName string, dialect migrate.Dialect, dsn string, err error) {
	name := cfg.Driver
	if value := strings.TrimSpace(os.Getenv("DB_DIALECT")); value != "" {
		name = value
	}
	driverName, dialect, err = resolveDriver(name)
	if err != nil {
		return "", nil, "", err
	}
	dsn = cfg.DSN
	if value := os.Getenv(db.DSNEnvVar); value != "" {
		dsn = value
	}
	if dsn == "" {
		return "", nil, "", fmt.Errorf("database DSN is required: set db.dsn in the configuration or the %s environment variable", db.DSNEnvVar)
	}
	return driverName, dialect, dsn, nil
}

// openMigrator opens the database/sql connection for cfg and builds the
// migrator over the application's migrations. The caller closes the
// connection. The sqlite driver is the pure-Go glebarez driver — the same
// registration the db component's GORM layer uses — so both layers coexist
// in one binary without duplicate driver registrations.
func openMigrator(cfg config.DB, migrations []migrate.Migration) (*sql.DB, *migrate.Migrator, error) {
	driverName, dialect, dsn, err := resolveTarget(cfg)
	if err != nil {
		return nil, nil, err
	}
	conn, err := sql.Open(driverName, dsn)
	if err != nil {
		return nil, nil, err
	}
	migrator := migrate.NewMigrator(conn, dialect, migrations)
	if table := os.Getenv("MIGRATE_TABLE"); table != "" {
		migrator.MigrationTable = table
	}
	migrator.DryRun = envBool("MIGRATE_DRY_RUN")
	migrator.UseLock = dialectLockable(dialect)
	return conn, migrator, nil
}

// dialectLockable reports whether the dialect supports advisory migration
// locks. SQLite cannot generate a lock statement, so its commands run
// without the lock step instead of failing on every invocation. Probing the
// dialect keeps this correct when a dialect grows or loses lock support.
func dialectLockable(dialect migrate.Dialect) bool {
	_, err := dialect.AcquireLockSQL("probe", 1)
	return err == nil
}

// envBool reads a boolean environment variable with the truthy spellings
// accepted by the standalone yiimigrate CLI.
func envBool(name string) bool {
	value := os.Getenv(name)
	return value == "1" || value == "true" || value == "TRUE" || value == "yes"
}

// limitRunner executes one limit-style subcommand against a freshly opened
// migrator; limit is the parsed optional argument (see optionalInt).
type limitRunner func(ctx context.Context, cmd *cobra.Command, migrator *migrate.Migrator, limit int) error

// newLimitCommand builds a subcommand taking at most one optional numeric
// argument (the limit, defaulting to fallback) over fn.
func newLimitCommand(app *base.Application, migrations []migrate.Migration, use, short string, fallback int, fn limitRunner) *cobra.Command {
	return &cobra.Command{
		Use:   use + " [limit]",
		Short: short,
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			conn, migrator, err := openMigrator(app.Config().DB, migrations)
			if err != nil {
				return err
			}
			defer conn.Close()
			return fn(cmd.Context(), cmd, migrator, optionalInt(args, fallback))
		},
	}
}

func runUp(ctx context.Context, _ *cobra.Command, migrator *migrate.Migrator, limit int) error {
	_, err := migrator.Up(ctx, limit)
	return err
}

func runDown(ctx context.Context, _ *cobra.Command, migrator *migrate.Migrator, limit int) error {
	_, err := migrator.Down(ctx, limit)
	return err
}

func runRedo(ctx context.Context, _ *cobra.Command, migrator *migrate.Migrator, limit int) error {
	_, _, err := migrator.Redo(ctx, limit)
	return err
}

func runHistory(ctx context.Context, cmd *cobra.Command, migrator *migrate.Migrator, limit int) error {
	history, err := migrator.History(ctx, limit)
	if err != nil {
		return err
	}
	for _, item := range history {
		fmt.Fprintf(cmd.OutOrStdout(), "%s %d\n", item.Version, item.ApplyTime)
	}
	return nil
}

func runNew(ctx context.Context, cmd *cobra.Command, migrator *migrate.Migrator, limit int) error {
	pending, err := migrator.New(ctx, limit)
	if err != nil {
		return err
	}
	for _, migration := range pending {
		fmt.Fprintln(cmd.OutOrStdout(), migration.Name())
	}
	return nil
}

// newMarkCommand builds "migrate mark VERSION": VERSION "0" clears the
// migration history, any other value re-marks history up to that version,
// without running migration code.
func newMarkCommand(app *base.Application, migrations []migrate.Migration) *cobra.Command {
	return &cobra.Command{
		Use:   "mark VERSION",
		Short: "Modify the migration history without running migrations",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			conn, migrator, err := openMigrator(app.Config().DB, migrations)
			if err != nil {
				return err
			}
			defer conn.Close()
			return migrator.Mark(cmd.Context(), args[0])
		},
	}
}

// newToCommand builds "migrate to VERSION": migrates up or down until
// VERSION is the latest applied migration.
func newToCommand(app *base.Application, migrations []migrate.Migration) *cobra.Command {
	return &cobra.Command{
		Use:   "to VERSION",
		Short: "Migrate up or down until VERSION is the latest applied migration",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			conn, migrator, err := openMigrator(app.Config().DB, migrations)
			if err != nil {
				return err
			}
			defer conn.Close()
			return migrator.To(cmd.Context(), args[0])
		},
	}
}

// newCreateCommand builds "migrate create NAME": it writes a migration file
// named after the current timestamp plus NAME into the migrations directory
// of the working directory. The generated code imports this framework's
// migrate package; --dialect defaults to the application's configured
// dialect so templates match the target database.
func newCreateCommand(app *base.Application) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create NAME",
		Short: "Generate a new migration file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fields, err := cmd.Flags().GetString("fields")
			if err != nil {
				return err
			}
			dialect, err := cmd.Flags().GetString("dialect")
			if err != nil {
				return err
			}
			path, err := generator.Generate(generator.Options{
				Name:        args[0],
				Dir:         "migrations",
				PackageName: "migrations",
				Fields:      fields,
				Dialect:     dialect,
			})
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "created %s\n", path)
			return nil
		},
	}
	cmd.Flags().String("fields", "", "comma-separated column definitions for the generated migration")
	cmd.Flags().String("dialect", defaultCreateDialect(app), "target dialect for the generated template")
	return cmd
}

// defaultCreateDialect resolves the dialect the create command targets: the
// DB_DIALECT environment variable when set (its behavior in the standalone
// CLI), otherwise the application's configured db driver, otherwise empty —
// which the generator itself defaults to mysql.
func defaultCreateDialect(app *base.Application) string {
	if value := strings.TrimSpace(os.Getenv("DB_DIALECT")); value != "" {
		return value
	}
	return app.Config().DB.Driver
}

// optionalInt parses args[0] as the command limit, falling back to fallback
// when absent or not a number.
func optionalInt(args []string, fallback int) int {
	if len(args) == 0 {
		return fallback
	}
	value, err := strconv.Atoi(args[0])
	if err != nil {
		return fallback
	}
	return value
}
