// The basic example is the P0+P1 walkthrough application: configuration
// assembly, migrations, seeding, session login, a validated CRUD API with
// access control, CSRF, negotiated errors, logging and caching — and the
// browser face on top: templ pages with layout and widgets, RBAC menus,
// translated interface and validation messages, a paginated sortable list,
// detail and edit forms, automatic timestamps and the mailer.
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
	"embed"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/a-h/templ"

	"github.com/goyii2/framework/base"
	"github.com/goyii2/framework/config"
	"github.com/goyii2/framework/console"
	"github.com/goyii2/framework/db/ar"
	"github.com/goyii2/framework/debug"
	"github.com/goyii2/framework/i18n"
	migratecommands "github.com/goyii2/framework/migrate/commands"
	queuecommands "github.com/goyii2/framework/queue/commands"
	"github.com/goyii2/framework/rbac"
	"github.com/goyii2/framework/url"
	"github.com/goyii2/framework/vite"
	"github.com/goyii2/framework/web"

	"github.com/goyii2/framework/examples/basic/migrations"
)

// messages holds the translation trees the i18n middleware resolves:
// messages/<lang>/<category>.json, embedded so the single binary carries
// every language.
//
//go:embed messages
var messages embed.FS

// headAssets is the Vite entry of the frontend (script and stylesheet tags),
// resolved once at startup and rendered into every page's <head> by the
// application layout.
var headAssets templ.Component

func main() {
	// runtime/ holds the sqlite database, the log file, session records,
	// cache entries and the debug panel's request records. SQLite refuses to
	// open a file inside a missing directory, so the directories exist before
	// anything opens them.
	for _, dir := range []string{"runtime", "runtime/session", "runtime/cache", "runtime/debug"} {
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
	app.SetI18n(i18n.NewMessageSource(messages))
	// The queue job registry is filled before any command runs: serve pushes
	// onto it, seed plants the demo jobs, and the queue workers resolve the
	// same names.
	if err := app.Queue().Jobs().Register(&EmailJob{}); err != nil {
		fail(err)
	}

	handler := web.NewHandler(app)
	// Static assets answer before anything else: they are not application
	// requests, so neither the debug capture nor the negotiator sees them.
	mountFrontend(handler)
	// The debug panel's capture must sit outside every response-writing
	// middleware, so it mounts before the application's own global chain;
	// with app.debug=false the call is a no-op.
	debug.New(app).Mount(handler)

	handler.SessionManager().IdentityFinder = findIdentity(app)
	// The RBAC manager answers user.Can and the access-control rules. It
	// shares the application's connection and carries no code-defined rules
	// (this example's items are unconditional), which is why NewDbManager
	// takes none.
	authConn, err := app.DB()
	if err != nil {
		fail(err)
	}
	auth, err := rbac.NewDbManager(authConn.DB)
	if err != nil {
		fail(err)
	}
	handler.SetAuthManager(auth)
	registerRoutes(handler, app, auth)
	registerViews(handler.Views())
	url.SetDefault(handler.URLs()) // url.To resolves this handler's named routes

	con := console.New(app, console.WithHandler(handler))
	con.AddCommand(migratecommands.New(app, migrations.All()))
	con.AddCommand(queuecommands.New(app))
	con.AddCommand(newSeedCommand())
	if err := con.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// mountFrontend wires the Vite frontend. The entry resolves once at startup —
// a wrong entry name or a broken manifest fails the process instead of
// shipping broken pages. Build mode (the committed web/dist manifest present)
// serves the hashed assets from disk; dev mode (no manifest yet, or a fresh
// checkout before the first build) proxies the static paths to the Vite dev
// server, so the Go side never needs Node at runtime.
func mountFrontend(h *web.Handler) {
	front := &vite.Vite{Manifest: "web/dist/.vite/manifest.json"}
	entry, err := front.Entry("src/main.ts")
	if err != nil {
		fail(err)
	}
	headAssets = entry
	if front.IsDev() {
		h.Use(front.DevProxy("/assets"))
		return
	}
	h.Use(distAssets("web/dist/assets", "/assets"))
}

// distAssets serves the committed build outputs below one path prefix. The
// file server confines lookups to dir; unknown files answer the plain 404
// page, never the convention fallback.
func distAssets(dir, prefix string) web.Middleware {
	files := http.StripPrefix(prefix, http.FileServer(http.Dir(dir)))
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if p := r.URL.Path; p == prefix || strings.HasPrefix(p, prefix+"/") {
				files.ServeHTTP(w, r)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// registerRoutes wires the URL space. Two families live side by side: the
// /post JSON API of the P0 walkthrough and the /posts browser pages of the
// P1 face; /site/index stays unbound on purpose so it resolves through the
// convention fallback (/{controller}/{action} over the root registry). The
// /admin module tree demos nesting: one module-level access rule guards
// the whole subtree, the /admin/report submodule adds a second prefix
// level with module-relative route names.
func registerRoutes(h *web.Handler, app *base.Application, auth *rbac.DbManager) {
	// Error responses negotiate JSON/HTML from the Accept header; write
	// requests carry a JSON body (API) or a form-encoded one (browser
	// pages), anything else answers 415.
	h.Use(web.ContentNegotiator{
		ResponseFormats: []web.Format{
			{Type: "application/json", Format: "json"},
			{Type: "text/html", Format: "html"},
		},
		RequestFormats: []web.Format{
			{Type: "application/json", Format: "json"},
			{Type: "application/x-www-form-urlencoded", Format: "form"},
		},
	}.Middleware())
	// The application frame wraps every HTML render (pages and error
	// pages alike); JSON replies never render, so it stays dormant there.
	h.Use(pageLayout())

	site := &SiteController{}
	post := &PostController{Auth: auth}

	sc := h.Ctrl(site)
	sc.Get("/", site.ActionHome).Name("site/home")
	sc.Get("/site/csrf", site.ActionCsrf).Name("site/csrf")
	sc.Get("/login", site.ActionLoginForm).Name("site/login")
	sc.Post("/login", site.ActionLogin)
	sc.Post("/logout", site.ActionLogout).Name("site/logout")
	sc.Get("/me", site.ActionMe).Name("site/me")
	sc.Post("/site/notify", site.ActionNotify).Name("site/notify")

	// Each demo page registers through its own controller's registrar, so
	// the controller middleware applies: the queue pages carry none, the
	// page-cache page carries its PageCache filter.
	queueDemo := &QueueController{}
	qc := h.Ctrl(queueDemo)
	qc.Get("/queue", queueDemo.ActionIndex).Name("queue/index")
	qc.Post("/queue/push", queueDemo.ActionPush).Name("queue/push")

	cacheDemo := &CacheController{}
	cc := h.Ctrl(cacheDemo)
	cc.Get("/pagecache", cacheDemo.ActionDemo).Name("site/pagecache")

	pc := h.Ctrl(post)
	// Browser pages.
	pc.Get("/posts", post.ActionList).Name("post/list")
	pc.Get("/posts/new", post.ActionNew).Name("post/new")
	pc.Post("/posts", post.ActionStore).Name("post/store")
	pc.Get("/posts/{id:[0-9]+}", post.ActionShow).Name("post/show")
	pc.Get("/posts/{id:[0-9]+}/edit", post.ActionEdit).Name("post/edit")
	pc.Post("/posts/{id:[0-9]+}", post.ActionSave).Name("post/save")
	pc.Post("/posts/{id:[0-9]+}/delete", post.ActionDestroy).Name("post/destroy")
	// JSON API.
	pc.Get("/post", post.ActionIndex).Name("post/index")
	pc.Get("/post/{id:[0-9]+}", post.ActionView).Name("post/view")
	pc.Post("/post", post.ActionCreate).Name("post/create")
	pc.Put("/post/{id:[0-9]+}", post.ActionUpdate).Name("post/update")
	pc.Delete("/post/{id:[0-9]+}", post.ActionDelete).Name("post/delete")

	if err := app.RegisterController("site", site); err != nil {
		fail(err)
	}
	if err := app.RegisterController("post", post); err != nil {
		fail(err)
	}

	// The nested module tree: /admin carries one access rule for its whole
	// subtree (every route below the prefix, explicit and convention
	// alike), and /admin/report nests below it. The submodule's route
	// names register module-relative — "dashboard" becomes
	// "admin/report/dashboard".
	admin, err := base.NewModule(app, "/admin")
	if err != nil {
		fail(err)
	}
	if err := app.RegisterModule(admin); err != nil {
		fail(err)
	}
	report, err := base.NewModule(app, "/admin/report")
	if err != nil {
		fail(err)
	}
	if err := admin.RegisterModule(report); err != nil {
		fail(err)
	}
	adminRoutes, err := h.Module("/admin")
	if err != nil {
		fail(err)
	}
	adminRoutes.Use(web.AccessControl{Auth: auth, Rules: []web.AccessRule{
		{Allow: true, Roles: []string{"@"}}, // module-wide: login required below /admin
	}}.Middleware())
	if err := admin.RegisterController("panel", &PanelController{}); err != nil {
		fail(err)
	}
	reportRoutes, err := h.Module("/admin/report")
	if err != nil {
		fail(err)
	}
	reportRoutes.Get("dashboard", (&ReportController{}).ActionDashboard).Name("dashboard")
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
