// The advanced example maps the Yii2 advanced template onto goyii: one
// process, one port, partitions in packages. The frontend application is
// the handler's root scope, the backend application lands at /backend as a
// module guarded by the admin role, and the console application's commands
// (serve, migrate, user/create, seed) live in this same binary.
//
// The layout follows the advanced template's partitions:
//
//	config/       this project's own configuration shape and yaml files
//	common/       the shared package: models and migrations
//	frontend/     the frontend application partition (root scope)
//	backend/      the backend application partition (module /backend)
//	commands/     the user-defined console commands (user/create, seed)
//
// Each partition exposes its own assembly function; main only composes.
//
// Run it from this directory:
//
//	go run . migrate up
//	go run . seed
//	go run . user/create --username demo --password demo123
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
	"github.com/goyii2/framework/examples/advanced/backend"
	"github.com/goyii2/framework/examples/advanced/commands"
	"github.com/goyii2/framework/examples/advanced/common/migrations"
	"github.com/goyii2/framework/examples/advanced/common/models"
	"github.com/goyii2/framework/examples/advanced/config"
	"github.com/goyii2/framework/examples/advanced/frontend"
	"github.com/goyii2/framework/log"
	migratecommands "github.com/goyii2/framework/migrate/commands"
	"github.com/goyii2/framework/rbac"
	"github.com/goyii2/framework/session"
	"github.com/goyii2/framework/url"
	"github.com/goyii2/framework/web"
)

func main() {
	// runtime/ holds the sqlite database and the session records. SQLite
	// refuses to open a file inside a missing directory, so the directories
	// exist before anything opens them.
	for _, dir := range []string{"runtime", "runtime/session"} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			fail(err)
		}
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
	sess, err := session.New(cfg.Session)
	if err != nil {
		fail(err)
	}
	app.SetSession(sess)
	base.SetDefault(app) // background code without a request context resolves this instance

	handler := web.NewHandler(app)
	// Reverse URL generation: the partitions and their views resolve links
	// by route name (url.To), so this handler's named-route table is the
	// package-level default they read.
	url.SetDefault(handler.URLs())
	// The shared user model resolves a session's stored identity id back to
	// the account row — the bridge that turns a session cookie into the
	// logged-in state the frontend navigation shows.
	handler.SessionManager().IdentityFinder = models.FindIdentity(app)
	// The RBAC manager answers the access rules and User.Can against the
	// auth tables the common migrations create; the seed command grants the
	// admin role the backend module's guard requires. Accounts carry no
	// role attributes — assignments in the auth tables are the only source.
	auth, err := rbac.NewDbManager(app.DB().DB)
	if err != nil {
		fail(err)
	}
	handler.SetAuthManager(auth)
	// The frontend partition registers itself: its light layout, its views
	// and its root-scope routes all live behind its own assembly function.
	// It goes first — the backend partition's guard resolves the login
	// page through the frontend's named route.
	if err := frontend.Register(handler, app); err != nil {
		fail(err)
	}
	// The backend partition mounts its /backend module: dark layout, admin
	// guard and dashboard, all behind its own assembly function.
	if err := backend.Register(handler, app, auth); err != nil {
		fail(err)
	}

	con := console.New(app, console.WithHandler(handler), console.WithListen(cfg.Web.Listen))
	con.AddCommand(migratecommands.New(app, migrations.All()))
	con.AddCommand(commands.NewUserCreateCommand())
	con.AddCommand(commands.NewSeedCommand())
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
