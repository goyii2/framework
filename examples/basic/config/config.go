// Package config holds this project's own configuration shape. The project
// decides what its configuration contains — identity and server values of its
// own, the parameter bundles of the component packages it uses, and fully
// custom sections no framework package knows about — and the framework's
// generic loader fills it (see config.LoadAs in the framework):
//
//	app.yaml            base values, every environment
//	web-<env>.yaml      overlay of the active environment (GOYII_ENV, default dev)
//	environment         leaf-field overrides, path upper-cased: DB_DSN,
//	                    APP_DEBUG, EMAIL_APIKEY, ...
//
// The values here are data only: behavior is assembled in code, in main.go.
package config

import (
	"github.com/goyii2/framework/cache"
	"github.com/goyii2/framework/db"
	"github.com/goyii2/framework/log"
	"github.com/goyii2/framework/mail"
	"github.com/goyii2/framework/queue"
	"github.com/goyii2/framework/session"
)

// Config is everything this application reads from its configuration
// directory. Identity fields (app, web) and the custom email section belong to
// the project; the remaining sections embed the component packages' parameter
// bundles, so a section's fields are defined next to the component that
// consumes them.
type Config struct {
	// App is the application's identity. Name shows in the layout and the
	// debug panel; Debug turns on the verbose error pages and the debug
	// panel. The environment name itself comes from GOYII_ENV, not a field.
	App struct {
		Name  string `yaml:"name"`
		Debug bool   `yaml:"debug"`
	} `yaml:"app"`

	// Web carries the HTTP server's own values. Listen is where serve binds;
	// it is injected into the console command in main.go.
	Web struct {
		Listen string `yaml:"listen"`
	} `yaml:"web"`

	DB      db.Config      `yaml:"db"`
	Queue   queue.Config   `yaml:"queue"`
	Cache   cache.Config   `yaml:"cache"`
	Session session.Config `yaml:"session"`
	Mail    mail.Config    `yaml:"mail"`
	Log     log.Config     `yaml:"log"`

	// Stats is the secondary database of the multi-database walkthrough: a
	// second db.Config under its own yaml key, so its env overrides carry the
	// STATS_ prefix (STATS_DRIVER, STATS_DSN). The connection is a project
	// asset assembled in main.go (not the application's primary db slot) and
	// registers as the migrate target "stats" (migrate --db=stats up).
	Stats db.Config `yaml:"stats"`

	// I18n picks the display language for requests that signal no
	// preference; the message trees themselves are embedded in the binary.
	I18n struct {
		Language string `yaml:"language"`
	} `yaml:"i18n"`

	// Email is the project's fully custom section: the credentials of a
	// transactional-mail provider. No framework package knows these fields;
	// the notifier component (this project's own) consumes them, and the
	// generic env machine can override each one (EMAIL_APIKEY,
	// EMAIL_SENDER). apiKey is a placeholder for the walkthrough — a real
	// deployment feeds it through the environment, never through app.yaml.
	Email EmailConfig `yaml:"email"`
}

// EmailConfig is the custom section of Config (see there).
type EmailConfig struct {
	APIKey string `yaml:"apiKey"`
	Sender string `yaml:"sender"`
}
