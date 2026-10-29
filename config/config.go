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
	Redis   Redis   `yaml:"redis"`
	Cache   Cache   `yaml:"cache"`
	Mutex   Mutex   `yaml:"mutex"`
	Queue   Queue   `yaml:"queue"`
	Log     Log     `yaml:"log"`
	Session Session `yaml:"session"`
	I18n    I18n    `yaml:"i18n"`
	Mail    Mail    `yaml:"mail"`
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

// Mutex holds the mutex component values. The redis driver draws the
// application's shared redis connection (the top-level redis section); it
// takes no values of its own here.
type Mutex struct {
	Driver string `yaml:"driver"` // process | redis; default process
}

// Queue holds the queue component values. The db driver stores messages in
// TableName (create it with queue.NewMigration); the redis driver shares the
// application's redis connection (the top-level redis section) and takes no
// values of its own here.
type Queue struct {
	Driver    string `yaml:"driver"`    // sync | db | redis; default sync
	TableName string `yaml:"tableName"` // db driver table; default queue
	// PollIntervalMs is how long the listen worker waits between drain
	// passes when idle; a short value trades CPU for latency in tests.
	PollIntervalMs int `yaml:"pollIntervalMs"`
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

// Redis holds connection values for redis-backed components. The top-level
// section feeds the application's shared connection; the cache and session
// sections of the same shape are optional overrides — empty inherits the
// top-level values, non-empty replaces them (a different server cannot share
// one connection).
type Redis struct {
	Addr     string `yaml:"addr"`
	Password string `yaml:"password"`
	DB       int    `yaml:"db"`
}

// I18n holds the internationalization component values. The source language
// — the language message ids are written in — is the application's own
// App.Language; this section only steers the display side.
type I18n struct {
	// Language is the display language for requests that signal no
	// preference; empty falls back to the application's source language.
	Language string `yaml:"language"`

	// MessagesDir is the directory holding the <lang>/<category>.json
	// translation trees the default message source loads from disk.
	// Applications embedding their translations install the embedded
	// filesystem directly (Application.SetI18n) and ignore this value.
	MessagesDir string `yaml:"messagesDir"`
}

// Mail holds the outgoing-mail component values. Driver selects the
// implementation: "log" records every message in the application log without
// sending (the development default), "smtp" delivers through a real server.
type Mail struct {
	Driver string `yaml:"driver"` // log | smtp; default log
	From   string `yaml:"from"`   // default sender for messages that set none, e.g. "Ada <noreply@example.com>"
	SMTP   SMTP   `yaml:"smtp"`
}

// SMTP holds the values of the smtp driver. Encryption selects the transport
// security: "none" sends plaintext, "starttls" upgrades the connection with
// STARTTLS after connect (the default), "smtps" connects with implicit TLS.
// The Yii2 swiftmailer spellings "tls" and "ssl" are accepted as aliases of
// starttls and smtps.
type SMTP struct {
	Host       string `yaml:"host"`
	Port       int    `yaml:"port"`     // default: 587 (465 with encryption smtps)
	Username   string `yaml:"username"` // empty disables authentication
	Password   string `yaml:"password"`
	Encryption string `yaml:"encryption"` // none | starttls | smtps (aliases tls/ssl); default starttls
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
	if cfg.Mutex.Driver == "" {
		cfg.Mutex.Driver = "process"
	}
	if cfg.Queue.Driver == "" {
		cfg.Queue.Driver = "sync"
	}
	if cfg.Queue.TableName == "" {
		cfg.Queue.TableName = "queue"
	}
	if cfg.Queue.PollIntervalMs == 0 {
		cfg.Queue.PollIntervalMs = 1000
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
	if cfg.I18n.MessagesDir == "" {
		cfg.I18n.MessagesDir = "messages"
	}
	if cfg.Mail.Driver == "" {
		cfg.Mail.Driver = "log"
	}
	return cfg
}
