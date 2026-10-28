package db

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlserver"
	"gorm.io/gorm"

	"github.com/goyii2/framework/config"
)

// DSNEnvVar is the environment variable overriding the configured DSN. It
// takes precedence over the db.dsn configuration value so deployment
// platforms can inject the DSN without touching configuration files.
const DSNEnvVar = "DB_DSN"

// Conn is the application's database connection: a GORM handle for data
// access (DML) plus the driver metadata. Schema changes belong to the migrate
// package, which opens its own database/sql connection from the same DSN.
type Conn struct {
	*gorm.DB

	// driver is the canonical driver name ("mysql", "sqlite", ...).
	driver string
}

// Driver returns the canonical driver name the connection was opened with.
func (c *Conn) Driver() string { return c.driver }

// Close releases the underlying connection pool.
func (c *Conn) Close() error {
	sqlDB, err := c.DB.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

// Connect opens a database connection for cfg. An empty driver resolves to
// mysql, matching the migration commands' default dialect; driver aliases
// (sqlite3, postgresql, mssql) are accepted alongside the canonical names.
// The DB_DSN environment variable overrides the configured DSN when set.
func Connect(cfg config.DB) (*Conn, error) {
	dsn := cfg.DSN
	if fromEnv := os.Getenv(DSNEnvVar); fromEnv != "" {
		dsn = fromEnv
	}
	if strings.TrimSpace(dsn) == "" {
		return nil, fmt.Errorf("db: DSN is required: set db.dsn in the configuration or the %s environment variable", DSNEnvVar)
	}
	driver, err := canonicalDriver(cfg.Driver)
	if err != nil {
		return nil, err
	}

	gormDB, err := gorm.Open(dialectorFor(driver, dsn), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("db: open %s: %w", driver, err)
	}
	return &Conn{DB: gormDB, driver: driver}, nil
}

// driverAliases maps configured driver spellings (including the empty
// default) to canonical driver names.
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

// canonicalDriver resolves the configured driver name to its canonical form.
func canonicalDriver(name string) (string, error) {
	driver, ok := driverAliases[strings.ToLower(strings.TrimSpace(name))]
	if !ok {
		return "", fmt.Errorf("db: unsupported driver %q; supported values: %s", name, strings.Join(sortedDrivers(), ", "))
	}
	return driver, nil
}

// sortedDrivers lists the canonical driver names for error messages.
func sortedDrivers() []string {
	seen := make(map[string]struct{}, len(driverAliases))
	for _, driver := range driverAliases {
		seen[driver] = struct{}{}
	}
	drivers := make([]string, 0, len(seen))
	for driver := range seen {
		drivers = append(drivers, driver)
	}
	sort.Strings(drivers)
	return drivers
}

// dialectorFor returns the GORM dialector for the canonical driver name and
// dsn. The sqlite driver is the pure-Go glebarez driver, so no CGO toolchain
// is required on any platform.
func dialectorFor(driver, dsn string) gorm.Dialector {
	switch driver {
	case "sqlite":
		return sqlite.Open(dsn)
	case "postgres":
		return postgres.Open(dsn)
	case "sqlserver":
		return sqlserver.Open(dsn)
	default:
		return mysql.Open(dsn)
	}
}
