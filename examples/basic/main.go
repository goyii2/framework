// The basic example is the P0 walkthrough application: configuration
// assembly, migrations, seeding, session login and a validated CRUD API with
// access control, CSRF, negotiated errors, logging and caching all live.
//
// Run it from this directory:
//
//	go run . migrate up
//	go run . seed
//	go run . serve
//
// The application lives in the framework's module on purpose: the example
// compiles in every CI run of the module and `go run ./examples/basic` works
// straight after cloning, with no second go.mod to keep in sync.
package main

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/goyii2/framework/base"
	"github.com/goyii2/framework/config"
	"github.com/goyii2/framework/console"
	"github.com/goyii2/framework/db/ar"
	migratecommands "github.com/goyii2/framework/migrate/commands"
	"github.com/goyii2/framework/url"
	"github.com/goyii2/framework/web"

	"github.com/goyii2/framework/examples/basic/migrations"
)

func main() {
	// runtime/ holds the sqlite database, the log file, session records and
	// cache entries. SQLite refuses to open a file inside a missing
	// directory, so the directories exist before anything opens them.
	for _, dir := range []string{"runtime", "runtime/session", "runtime/cache"} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			fail(err)
		}
	}

	cfg, err := config.Load("config")
	if err != nil {
		fail(err)
	}
	app := base.New(cfg)
	base.SetDefault(app) // background code without a request context resolves this instance

	handler := web.NewHandler(app)
	handler.SessionManager().IdentityFinder = findIdentity(app)
	registerRoutes(handler, app)
	url.SetDefault(handler.URLs()) // url.To resolves this handler's named routes

	con := console.New(app, console.WithHandler(handler))
	con.AddCommand(migratecommands.New(app, migrations.All()))
	con.AddCommand(newSeedCommand())
	if err := con.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// registerRoutes wires the URL space. Explicit bindings cover the API;
// /site/index stays unbound on purpose so it resolves through the
// convention fallback (/{controller}/{action} over the root registry).
func registerRoutes(h *web.Handler, app *base.Application) {
	// Error responses negotiate JSON/HTML from the Accept header, and write
	// requests must carry a JSON body (anything else answers 415).
	h.Use(web.ContentNegotiator{
		ResponseFormats: []web.Format{
			{Type: "application/json", Format: "json"},
			{Type: "text/html", Format: "html"},
		},
		RequestFormats: []web.Format{{Type: "application/json", Format: "json"}},
	}.Middleware())

	site := &SiteController{}
	post := &PostController{}

	sc := h.Ctrl(site)
	sc.Get("/site/csrf", site.ActionCsrf).Name("site/csrf")
	sc.Post("/login", site.ActionLogin).Name("site/login")
	sc.Post("/logout", site.ActionLogout).Name("site/logout")
	sc.Get("/me", site.ActionMe).Name("site/me")

	pc := h.Ctrl(post)
	pc.Get("/post", post.ActionIndex).Name("post/index")
	pc.Get("/post/{id:[0-9]+}", post.ActionView).Name("post/view")
	pc.Post("/post", post.ActionCreate).Name("post/create")
	pc.Put("/post/{id:[0-9]+}", post.ActionUpdate).Name("post/update")
	pc.Delete("/post/{id:[0-9]+}", post.ActionDelete).Name("post/delete")

	if err := app.RegisterController("site", site); err != nil {
		fail(err)
	}
}

// findIdentity resolves the identity id stored in a session back to the
// account row. It is the bridge between the session layer and the user
// table; an id that no longer matches an account degrades to a guest.
func findIdentity(app *base.Application) func(string) (web.Identity, bool) {
	return func(id string) (web.Identity, bool) {
		uid, err := strconv.ParseInt(id, 10, 64)
		if err != nil {
			return nil, false
		}
		user, err := ar.FindByID[User](app.WithContext(context.Background()), uid)
		if err != nil {
			return nil, false
		}
		return user, true
	}
}

// fail reports a startup problem and ends the process.
func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
