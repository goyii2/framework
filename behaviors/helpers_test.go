package behaviors

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/goyii2/framework/base"
	"github.com/goyii2/framework/config"
	"github.com/goyii2/framework/db"
	"github.com/goyii2/framework/db/ar"
	"github.com/goyii2/framework/web"
)

// tsDoc carries the time.Time timestamp variant alone.
type tsDoc struct {
	ar.Model
	Hooks
	Timestamp
	ID    int64  `gorm:"column:id;primaryKey"`
	Title string `gorm:"column:title"`
}

func (tsDoc) TableName() string { return "ts_docs" }

// unixDoc carries the integer-seconds timestamp variant alone.
type unixDoc struct {
	ar.Model
	Hooks
	TimestampUnix
	ID    int64  `gorm:"column:id;primaryKey"`
	Title string `gorm:"column:title"`
}

func (unixDoc) TableName() string { return "unix_docs" }

// auditDoc carries Blameable alone; its writer identity comes from the
// context the save runs with.
type auditDoc struct {
	ar.Model
	Hooks
	Blameable
	ID   int64  `gorm:"column:id;primaryKey"`
	Note string `gorm:"column:note"`
}

func (auditDoc) TableName() string { return "audit_docs" }

// article composes Timestamp and Sluggable — the publishing-table shape —
// with default slug options.
type article struct {
	ar.Model
	Hooks
	Timestamp
	Sluggable
	ID    int64  `gorm:"column:id;primaryKey"`
	Title string `gorm:"column:title"`
}

func (article) TableName() string { return "articles" }

func (article) SlugOptions() SlugOptions { return SlugOptions{Source: "Title"} }

// shoutArticle derives slugs with a custom separator and preserved case,
// showing the option seam.
type shoutArticle struct {
	ar.Model
	Hooks
	Sluggable
	ID    int64  `gorm:"column:id;primaryKey"`
	Title string `gorm:"column:title"`
}

func (shoutArticle) TableName() string { return "articles" }

func (shoutArticle) SlugOptions() SlugOptions {
	return SlugOptions{Source: "Title", Separator: "_", KeepCase: true}
}

// unicodeArticle keeps non-ASCII text in its slugs.
type unicodeArticle struct {
	ar.Model
	Hooks
	Sluggable
	ID    int64  `gorm:"column:id;primaryKey"`
	Title string `gorm:"column:title"`
}

func (unicodeArticle) TableName() string { return "articles" }

func (unicodeArticle) SlugOptions() SlugOptions {
	return SlugOptions{Source: "Title", KeepUnicode: true}
}

// lockedDoc composes OptimisticLock with Timestamp — the guarded-row shape.
type lockedDoc struct {
	ar.Model
	Hooks
	Timestamp
	OptimisticLock
	ID      int64  `gorm:"column:id;primaryKey"`
	Payload string `gorm:"column:payload"`
}

func (lockedDoc) TableName() string { return "locked_docs" }

// allDoc composes every behavior on one model, the composition proof: one
// record, four fills, one save.
type allDoc struct {
	ar.Model
	Hooks
	Timestamp
	Blameable
	Sluggable
	OptimisticLock
	ID    int64  `gorm:"column:id;primaryKey"`
	Title string `gorm:"column:title"`
}

func (allDoc) TableName() string { return "all_docs" }

func (allDoc) SlugOptions() SlugOptions { return SlugOptions{Source: "Title"} }

// modelDDL creates the tables the test models map to. Explicit DDL, not
// AutoMigrate, matching the framework's own test convention.
var modelDDL = []string{
	"CREATE TABLE ts_docs (id INTEGER PRIMARY KEY AUTOINCREMENT, title TEXT NOT NULL, created_at DATETIME, updated_at DATETIME)",
	"CREATE TABLE unix_docs (id INTEGER PRIMARY KEY AUTOINCREMENT, title TEXT NOT NULL, created_at INTEGER, updated_at INTEGER)",
	"CREATE TABLE audit_docs (id INTEGER PRIMARY KEY AUTOINCREMENT, note TEXT NOT NULL, created_by INTEGER NOT NULL DEFAULT 0, updated_by INTEGER NOT NULL DEFAULT 0)",
	"CREATE TABLE articles (id INTEGER PRIMARY KEY AUTOINCREMENT, title TEXT NOT NULL, slug TEXT, created_at DATETIME, updated_at DATETIME)",
	"CREATE TABLE locked_docs (id INTEGER PRIMARY KEY AUTOINCREMENT, payload TEXT NOT NULL, version INTEGER NOT NULL, created_at DATETIME, updated_at DATETIME)",
	"CREATE TABLE all_docs (id INTEGER PRIMARY KEY AUTOINCREMENT, title TEXT NOT NULL, slug TEXT, created_by INTEGER NOT NULL DEFAULT 0, updated_by INTEGER NOT NULL DEFAULT 0, version INTEGER NOT NULL, created_at DATETIME, updated_at DATETIME)",
}

// newTestContext opens an in-memory sqlite database, creates the test
// tables and returns a context whose application carries the connection.
// The pool is capped at one connection because an in-memory database
// exists per connection.
func newTestContext(t *testing.T) context.Context {
	t.Helper()
	conn, err := db.Connect(config.DB{Driver: "sqlite", DSN: ":memory:"})
	if err != nil {
		t.Fatalf("connect in-memory sqlite: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	sqlDB, err := conn.DB.DB()
	if err != nil {
		t.Fatalf("raw connection: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	for _, ddl := range modelDDL {
		if err := conn.Exec(ddl).Error; err != nil {
			t.Fatalf("create table: %v", err)
		}
	}
	app := base.New(config.Config{})
	app.SetDB(conn)
	return app.WithContext(context.Background())
}

// asUser returns ctx with identity id published as the current identity —
// the seam console commands and background jobs use outside a request.
func asUser(ctx context.Context, id int64) context.Context {
	return web.WithIdentity(ctx, testIdentity{id: id})
}

// testIdentity is the minimal Identity for tests: a numeric id.
type testIdentity struct{ id int64 }

func (u testIdentity) GetID() string      { return strconv.FormatInt(u.id, 10) }
func (u testIdentity) GetName() string    { return fmt.Sprintf("user %d", u.id) }
func (u testIdentity) GetRoles() []string { return nil }

// newWebApp assembles the full HTTP pipeline over the test database:
// session middleware, HTTP basic authentication resolving a fixed user
// table, and the /note actions creating and updating audit rows. It
// returns the ready handler plus the plain context for direct queries.
func newWebApp(t *testing.T, ctx context.Context) http.Handler {
	t.Helper()
	app := base.New(config.Config{
		Web:     config.Web{DisableCsrf: true},
		Session: config.Session{Backend: "file", Path: t.TempDir()},
	})
	conn, err := base.FromContext(ctx).DB()
	if err != nil {
		t.Fatalf("test connection: %v", err)
	}
	app.SetDB(conn)

	handler := web.NewHandler(app)
	users := map[string]testIdentity{
		"alice": {id: 7}, "bob": {id: 11}, "carol": {id: 42},
	}
	handler.Use(web.HttpBasic{
		Authenticate: func(username, password string) (web.Identity, bool) {
			user, ok := users[username]
			if !ok || password != "pw" {
				return nil, false
			}
			return user, true
		},
	}.Middleware())

	handler.Handle("POST /note", func(c *web.Context) error {
		note := ar.New[auditDoc](c.Context()).New()
		note.Note = c.Request().URL.Query().Get("note")
		if err := note.Save(); err != nil {
			return err
		}
		return c.JSON(map[string]any{"id": note.ID, "createdBy": note.CreatedBy, "updatedBy": note.UpdatedBy})
	})
	handler.Handle("POST /note/{id}", func(c *web.Context) error {
		id, err := c.ParamInt64("id")
		if err != nil {
			return err
		}
		note, err := ar.FindByID[auditDoc](c.Context(), id)
		if err != nil {
			return err
		}
		note.Note = "touched"
		if err := note.Save(); err != nil {
			return err
		}
		return c.JSON(map[string]any{"id": note.ID, "createdBy": note.CreatedBy, "updatedBy": note.UpdatedBy})
	})
	return handler
}

// postAs issues an authenticated POST against handler and returns the
// response status.
func postAs(t *testing.T, handler http.Handler, target, username string) (*http.Response, error) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, target, nil)
	req.SetBasicAuth(username, "pw")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec.Result(), nil
}

// serve starts a real HTTP server around handler; the caller defers close.
func serve(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv
}
