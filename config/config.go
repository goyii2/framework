package config

import (
	"os"
	"path/filepath"
)

// Config carries all framework configuration values. Sections grow with the
// subsystems that consume them; unknown or absent values fall back to the
// framework defaults (see WithDefaults).
type Config struct {
	// Env is the resolved runtime environment name (dev, test, prod, ...).
	// It is set by the loader from GOYII_ENV / the explicit load argument.
	Env     string  `yaml:"env"`
	App     App     `yaml:"app"`
	Web     Web     `yaml:"web"`
	DB      DB      `yaml:"db"`
	Cache   Cache   `yaml:"cache"`
	Log     Log     `yaml:"log"`
	Session Session `yaml:"session"`
}

// App holds identity and runtime-mode values.
type App struct {
	Name     string `yaml:"name"`
	Language string `yaml:"language"` // source language for i18n, e.g. en-US
	Timezone string `yaml:"timezone"` // IANA name, e.g. UTC
	Debug    bool   `yaml:"debug"`
}

// Web holds HTTP server values.
type Web struct {
	Listen string `yaml:"listen"` // host:port the server listens on

	// DisableConventionFallback turns off /{controller}/{action} resolution
	// for paths no explicit route matched. The fallback is on by default;
	// disable it when every route is bound explicitly.
	DisableConventionFallback bool `yaml:"disableConventionFallback"`

	// DisableCsrf turns off the CSRF validation middleware the HTTP entry
	// point installs by default (double-submit cookie mode). Form-facing
	// applications keep it on; pure API applications that authenticate per
	// request (token/bearer) disable it.
	DisableCsrf bool `yaml:"disableCsrf"`
}

// DB holds the database connection values shared by DML and migrations.
type DB struct {
	Driver string `yaml:"driver"`
	DSN    string `yaml:"dsn"`
}

// Cache holds the cache component values.
type Cache struct {
	Driver    string `yaml:"driver"`    // memory | file | redis
	KeyPrefix string `yaml:"keyPrefix"` // key namespace for the file and redis backends (memory is instance-local)
	TTL       int    `yaml:"ttl"`       // seconds; default lifetime when a caller passes ttl <= 0
	Path      string `yaml:"path"`      // file backend storage directory
	Redis     Redis  `yaml:"redis"`
}

// Log holds the logging component values.
type Log struct {
	Level   string      `yaml:"level"` // debug | info | warn | error
	Targets []LogTarget `yaml:"targets"`
	Gorm    GormLog     `yaml:"gorm"`
}

// LogTarget configures one log target. Type selects the built-in sink
// (console | file); the remaining values are delivery rules applied by the
// dispatcher before the sink sees a record.
type LogTarget struct {
	Type       string   `yaml:"type"`       // console | file
	MinLevel   string   `yaml:"minLevel"`   // debug | info | warn | error
	Categories []string `yaml:"categories"` // include patterns ("web", "db*", "*"); empty = all
	Except     []string `yaml:"except"`     // exclude patterns; win over categories

	// File target values.
	Path      string `yaml:"path"`      // log file path
	MaxSizeMB int    `yaml:"maxSizeMB"` // rotate when the file exceeds this size
	MaxFiles  int    `yaml:"maxFiles"`  // rotated files kept on disk
}

// GormLog holds the values of the database query logging adapter, which
// routes GORM's logger callbacks into the framework log component.
type GormLog struct {
	Disable         bool `yaml:"disable"`         // true turns the adapter off (GORM default stdout logger)
	SlowThresholdMs int  `yaml:"slowThresholdMs"` // queries at or above this duration log as slow
}

// Session holds the session component values.
type Session struct {
	Name    string `yaml:"name"`    // session cookie name
	Backend string `yaml:"backend"` // file | redis
	Path    string `yaml:"path"`    // file backend storage directory
	Timeout int    `yaml:"timeout"` // seconds
	Redis   Redis  `yaml:"redis"`
}

// Redis holds connection values for redis-backed components.
type Redis struct {
	Addr     string `yaml:"addr"`
	Password string `yaml:"password"`
	DB       int    `yaml:"db"`
}

// Default returns a configuration with framework defaults and no file or
// environment values applied.
func Default() Config {
	return WithDefaults(Config{})
}

// WithDefaults fills zero values of cfg with framework defaults and returns
// the result. Explicitly set values are kept as-is.
func WithDefaults(cfg Config) Config {
	if cfg.Env == "" {
		cfg.Env = "dev"
	}
	if cfg.App.Name == "" {
		cfg.App.Name = "myapp"
	}
	if cfg.App.Language == "" {
		cfg.App.Language = "en-US"
	}
	if cfg.App.Timezone == "" {
		cfg.App.Timezone = "UTC"
	}
	if cfg.Web.Listen == "" {
		cfg.Web.Listen = ":8080"
	}
	if cfg.Log.Level == "" {
		cfg.Log.Level = "info"
	}
	if cfg.Log.Gorm.SlowThresholdMs == 0 {
		cfg.Log.Gorm.SlowThresholdMs = 200
	}
	if cfg.Cache.Driver == "" {
		cfg.Cache.Driver = "memory"
	}
	if cfg.Cache.TTL == 0 {
		cfg.Cache.TTL = 3600
	}
	if cfg.Cache.Path == "" {
		cfg.Cache.Path = filepath.Join(os.TempDir(), "goyii-cache")
	}
	if cfg.Session.Name == "" {
		cfg.Session.Name = "goyii-session"
	}
	if cfg.Session.Backend == "" {
		cfg.Session.Backend = "file"
	}
	if cfg.Session.Path == "" {
		cfg.Session.Path = filepath.Join(os.TempDir(), "goyii-session")
	}
	if cfg.Session.Timeout == 0 {
		cfg.Session.Timeout = 1440
	}
	return cfg
}
