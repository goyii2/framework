// Package config holds the advanced example's own configuration shape —
// the counterpart of the Yii2 advanced template's environments/ directory:
// app.yaml carries every environment's values and the web-<env>.yaml
// overlay replaces what one environment changes. The framework's generic
// loader fills the struct (see config.LoadAs in the framework):
//
//	app.yaml            base values, every environment
//	web-<env>.yaml      overlay of the active environment (GOYII_ENV, default dev)
//	environment         leaf-field overrides, path upper-cased: DB_DSN,
//	                    WEB_LISTEN, APP_DEBUG, ...
//
// The values here are data only: behavior is assembled in code, in main.go.
package config

import (
	"github.com/goyii2/framework/db"
	"github.com/goyii2/framework/log"
	"github.com/goyii2/framework/session"
)

// Config is everything this application reads from its configuration
// directory: the identity fields (app, web) and the parameter bundles of
// the components it wires. Sections join as later slices need them.
type Config struct {
	// App is the application's identity. Name shows in the layout; Debug
	// turns on the verbose error pages. The dev overlay flips Debug on, so
	// the base file keeps the terse production setting — the observable
	// environment difference of this template. The environment name itself
	// comes from GOYII_ENV, not a field.
	App struct {
		Name  string `yaml:"name"`
		Debug bool   `yaml:"debug"`
	} `yaml:"app"`

	// Web carries the HTTP server's own values. Listen is where serve binds;
	// it is injected into the console command in main.go. HostInfo is the
	// scheme-and-host prefix absolute URLs are built on (a post page's
	// permalink); main.go installs it on the url manager at boot.
	// BackendHostInfo is the backend partition's own prefix — the deployment
	// shape where the admin face runs on a domain of its own; main.go hands
	// it to the backend partition, whose permalink builds on it explicitly.
	Web struct {
		Listen          string `yaml:"listen"`
		HostInfo        string `yaml:"hostInfo"`
		BackendHostInfo string `yaml:"backendHostInfo"`
	} `yaml:"web"`

	DB      db.Config      `yaml:"db"`
	Session session.Config `yaml:"session"`
	Log     log.Config     `yaml:"log"`
}
