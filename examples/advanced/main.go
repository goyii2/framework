// The advanced example maps the Yii2 advanced template onto goyii: one
// process, one port, partitions in packages. The frontend application is
// the handler's root scope, the backend application lands at /backend as a
// module, and the console application's commands (serve, migrate — later
// seed and user-create) live in this same binary.
//
// The layout follows the advanced template's partitions:
//
//	config/       this project's own configuration shape and yaml files
//	common/       the shared package: migrations (models join in later slices)
//	frontend/     the frontend application partition (root scope)
//	backend/      the backend application partition (module /backend; a later slice)
//	commands/     the user-defined console commands (a later slice)
//
// Each partition exposes its own assembly function; main only composes.
//
// Run it from this directory:
//
//	go run . migrate up
//	go run . serve
//
// The application lives in the framework's module on purpose: the example
// compiles in every CI run of the module and `go run ./examples/advanced`
// works straight after cloning, with no second go.mod to keep in sync.
package main

import (
	"fmt"
	"os"

	"github.com/goyii2/framework/base"
	// The framework's generic loader: fills whatever struct a project
	// defines, from app.yaml + web-<env>.yaml + environment variables.
	goyii "github.com/goyii2/framework/config"
	"github.com/goyii2/framework/console"
	"github.com/goyii2/framework/examples/advanced/common/migrations"
	"github.com/goyii2/framework/examples/advanced/config"
	"github.com/goyii2/framework/examples/advanced/frontend"
	"github.com/goyii2/framework/log"
	migratecommands "github.com/goyii2/framework/migrate/commands"
	"github.com/goyii2/framework/web"
)

func main() {
	// runtime/ holds the sqlite database. SQLite refuses to open a file
	// inside a missing directory, so the directory exists before anything
	// opens it.
	if err := os.MkdirAll("runtime", 0o755); err != nil {
		fail(err)
	}

	// The project's own configuration: config/ holds the struct and the yaml
	// files. LoadAs runs the three machines — app.yaml, the web-<env>.yaml
	// overlay (GOYII_ENV, dev by default), leaf-field environment overrides
	// (DB_DSN, WEB_LISTEN, APP_DEBUG, ...) — into that struct.
	cfg, err := goyii.LoadAs[config.Config]("config")
	if err != nil {
		fail(err)
	}

	// Assembly: every component is constructed eagerly from the project's
	// configuration and installed on the application; construction errors
	// end the process here, not somewhere inside a request. Constructors
	// validate only — no connection is dialed yet, so boot stays local.
	logger, err := log.New(cfg.Log)
	if err != nil {
		fail(err)
	}
	app := base.New(base.WithApp(cfg.App.Name, goyii.Env(), cfg.App.Debug))
	app.SetLog(logger)
	if err := app.ConnectDB(cfg.DB, cfg.Log.Gorm); err != nil {
		fail(err)
	}
	base.SetDefault(app) // background code without a request context resolves this instance

	handler := web.NewHandler(app)
	// The frontend partition registers itself: its light layout, its views
	// and its root-scope routes all live behind its own assembly function.
	if err := frontend.Register(handler, app); err != nil {
		fail(err)
	}
	// The backend partition mounts its /backend module here once its slice
	// lands: backend.Register(handler, app).

	con := console.New(app, console.WithHandler(handler), console.WithListen(cfg.Web.Listen))
	con.AddCommand(migratecommands.New(app, migrations.All()))
	// The commands partition adds its own commands here (seed, user/create)
	// once their slices land.
	if err := con.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// fail reports a startup problem and ends the process.
func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
