package db

import (
	"fmt"
	"os"
	"strings"

	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// Config carries the database connection values shared by DML and
// migrations. The zero driver resolves to mysql; the DSN comes from the
// configuration or the DB_DSN environment variable — db has no fallback
// connection.
type Config struct {
	Driver string `yaml:"driver"`
	DSN    string `yaml:"dsn"`
}

// Option customizes connection construction; it is how the application
// injects the query logging adapter without callers of New knowing about it.
type Option func(*options)

// options carries the accumulated New customizations.
type options struct {
	logger gormlogger.Interface
}

// WithGormLogger routes GORM's logger callbacks (queries, slow queries,
// errors) to the given logger instead of GORM's default stdout logger.
func WithGormLogger(l gormlogger.Interface) Option {
	return func(o *options) { o.logger = l }
}

// New opens the database connection for cfg. An empty driver resolves to
// mysql, matching the migration commands' default dialect; driver aliases
// (sqlite3, postgresql, mssql) are accepted alongside the canonical names.
// The DB_DSN environment variable overrides the configured DSN when set.
func New(cfg Config, opts ...Option) (*Conn, error) {
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

	var o options
	for _, opt := range opts {
		opt(&o)
	}
	gormDB, err := gorm.Open(dialectorFor(driver, dsn), &gorm.Config{Logger: o.logger})
	if err != nil {
		return nil, fmt.Errorf("db: open %s: %w", driver, err)
	}
	return &Conn{DB: gormDB, driver: driver}, nil
}
