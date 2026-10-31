// The basic example is the reference application for the framework's
// configuration and assembly shape — and the P0+P1+P2 walkthrough on top:
// migrations, seeding, session login, a validated CRUD API with access
// control, CSRF, negotiated errors, logging and caching — plus the browser
// face (templ pages with layout and widgets, RBAC menus, translated messages,
// a paginated sortable list, detail and edit forms, automatic timestamps) and
// the P2 face (queue, page cache, debug panel, Vite frontend).
//
// The layout follows the Yii2 mental model, one package per concern:
//
//	config/       this project's own configuration shape and yaml files
//	controllers/  the web controllers (one file per controller)
//	models/       the Active Record models
//	views/        the templ pages, layout and view registration
//	jobs/         the queue jobs
//	notifier/     a project-owned custom component (constructor injection)
//	commands/     the user-defined console commands (seed)
//	migrations/   the versioned schema migrations
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

	"github.com/goyii2/framework/base"
	"github.com/goyii2/framework/cache"
	// The framework's generic loader: fills whatever struct a project
	// defines, from app.yaml + web-<env>.yaml + environment variables.
	goyii "github.com/goyii2/framework/config"
	"github.com/goyii2/framework/console"
	"github.com/goyii2/framework/db"
	"github.com/goyii2/framework/db/ar"
	"github.com/goyii2/framework/debug"
	"github.com/goyii2/framework/examples/basic/commands"
	"github.com/goyii2/framework/examples/basic/config"
	"github.com/goyii2/framework/examples/basic/controllers"
	"github.com/goyii2/framework/examples/basic/jobs"
	"github.com/goyii2/framework/examples/basic/migrations"
	"github.com/goyii2/framework/examples/basic/models"
	"github.com/goyii2/framework/examples/basic/notifier"
	"github.com/goyii2/framework/examples/basic/statsmigrations"
	"github.com/goyii2/framework/examples/basic/views"
	"github.com/goyii2/framework/i18n"
	"github.com/goyii2/framework/log"
	"github.com/goyii2/framework/mail"
	migratecommands "github.com/goyii2/framework/migrate/commands"
	"github.com/goyii2/framework/queue"
	queuecommands "github.com/goyii2/framework/queue/commands"
	"github.com/goyii2/framework/rbac"
	"github.com/goyii2/framework/session"
	"github.com/goyii2/framework/url"
	"github.com/goyii2/framework/vite"
	"github.com/goyii2/framework/web"
)

// messages holds the translation trees the i18n middleware resolves:
// messages/<lang>/<category>.json, embedded so the single binary carries
// every language.
//
//go:embed messages
var messages embed.FS

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

	// The project's own configuration: config/ holds the struct and the yaml
	// files. LoadAs runs the three machines — app.yaml, the web-<env>.yaml
	// overlay (GOYII_ENV, dev by default), leaf-field environment overrides
	// (DB_DSN, APP_DEBUG, EMAIL_APIKEY, ...) — into that struct.
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
	// The secondary database: a project asset like any custom component,
	// assembled here and handed to its consumers — the only framework surface
	// it touches is the migrate target registered below (migrate --db=stats).
	statsConn, err := db.New(cfg.Stats)
	if err != nil {
		fail(err)
	}
	cacheC, err := cache.New(cfg.Cache)
	if err != nil {
		fail(err)
	}
	app.SetCache(cacheC)
	mailer, err := mail.New(cfg.Mail, mail.WithLogger(app.Logger().Category("mail")))
	if err != nil {
		fail(err)
	}
	app.SetMailer(mailer)
	queueQ, err := queue.New(cfg.Queue, queue.WithDB(app.DB()), app.QueueDispatcher())
	if err != nil {
		fail(err)
	}
	app.SetQueue(queueQ)
	sess, err := session.New(cfg.Session)
	if err != nil {
		fail(err)
	}
	app.SetSession(sess)
	base.SetDefault(app) // background code without a request context resolves this instance
	app.SetI18n(i18n.NewMessageSource(messages, i18n.WithDefaultLanguage(cfg.I18n.Language)))

	// The custom component of this project, built from the custom email
	// section: the transactional notifier, consumed by the site controller
	// (constructor injection below, the same shape as the auth manager). Its
	// startup line is the observable outlet of that section — the key it
	// prints has already been through the web-dev.yaml overlay, and the
	// EMAIL_APIKEY environment variable wins over both. Real credentials
	// never belong on a log; this one is a committed placeholder.
	notify, err := notifier.New(cfg.Email, app.Mailer())
	if err != nil {
		fail(err)
	}
	app.Logger().Info("notifier: transactional email ready", "sender", notify.Sender(), "apiKey", cfg.Email.APIKey)

	// The queue job registry is filled before any command runs: serve pushes
	// onto it, seed plants the demo jobs, and the queue workers resolve the
	// same names.
	if err := app.Queue().Jobs().Register(&jobs.EmailJob{}); err != nil {
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
	auth, err := rbac.NewDbManager(app.DB().DB)
	if err != nil {
		fail(err)
	}
	handler.SetAuthManager(auth)
	registerRoutes(handler, app, auth, notify)
	views.Register(handler.Views())
	url.SetDefault(handler.URLs()) // url.To resolves this handler's named routes
	// Absolute URL generation: the scheme-and-host prefix comes from the
	// project's configuration (WEB_HOSTINFO overrides), and the manager's
	// boot-time validation rejects a malformed value here instead of handing
	// half-configured absolute URLs to consumers.
	handler.URLs().SetHostInfo(cfg.Web.HostInfo)

	con := console.New(app, console.WithHandler(handler), console.WithListen(cfg.Web.Listen))
	con.AddCommand(migratecommands.New(app, migrations.All(),
		migratecommands.Target{Name: "stats", Conn: statsConn, Migrations: statsmigrations.All()},
	))
	con.AddCommand(queuecommands.New(app))
	con.AddCommand(commands.NewSeedCommand())
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
	views.HeadAssets = entry
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
func registerRoutes(h *web.Handler, app *base.Application, auth *rbac.DbManager, notify *notifier.Notifier) {
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
	h.Use(views.Layout())

	// The site controller receives the project's own notifier here — the
	// custom component rides along the same constructor injection the auth
	// manager uses on the post controller.
	site := &controllers.SiteController{Notify: notify}
	post := &controllers.PostController{Auth: auth}

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
	queueDemo := &controllers.QueueController{}
	qc := h.Ctrl(queueDemo)
	qc.Get("/queue", queueDemo.ActionIndex).Name("queue/index")
	qc.Post("/queue/push", queueDemo.ActionPush).Name("queue/push")

	cacheDemo := &controllers.CacheController{}
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
	if err := admin.RegisterController("panel", &controllers.PanelController{}); err != nil {
		fail(err)
	}
	reportRoutes, err := h.Module("/admin/report")
	if err != nil {
		fail(err)
	}
	reportRoutes.Get("dashboard", (&controllers.ReportController{}).ActionDashboard).Name("dashboard")
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
		user, err := ar.FindByID[models.User](app.WithContext(context.Background()), uid)
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
