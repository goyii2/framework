package validators_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	z "github.com/Oudwins/zog"

	"github.com/goyii2/framework/base"
	"github.com/goyii2/framework/config"
	"github.com/goyii2/framework/db"
	"github.com/goyii2/framework/db/ar"
	"github.com/goyii2/framework/validators"
	"github.com/goyii2/framework/web"
)

// apiUser is the endpoint's model: validation rules attached per save
// scenario, exactly like any other Active Record model.
type apiUser struct {
	ar.Model
	ID    int64  `gorm:"column:id;primaryKey"`
	Name  string `gorm:"column:name"`
	Email string `gorm:"column:email"`
}

func (apiUser) TableName() string { return "api_users" }

func (u *apiUser) Rules() validators.RuleSet {
	return validators.RuleSet{
		validators.ScenarioInsert: z.Struct(z.Shape{
			"name":  z.String().Required(z.Message("name cannot be blank")),
			"email": z.String().Email(z.Message("email is not a valid address")),
		}),
	}
}

// userPayload is the request body shape ActionCreate binds.
type userPayload struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

// userController shows the API wiring: a save refused by validation surfaces
// as 422 carrying the field -> messages map, anything else falls through to
// the unified error handler.
type userController struct{ web.Controller }

func (userController) ActionCreate(c *web.Context) error {
	var payload userPayload
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
		return web.BadRequest("malformed JSON body")
	}
	user := ar.New[apiUser](c.Context()).New()
	user.Name, user.Email = payload.Name, payload.Email
	if err := user.Save(); err != nil {
		var verrs *validators.Errors
		if errors.As(err, &verrs) {
			return c.JSONStatus(http.StatusUnprocessableEntity, verrs.Fields)
		}
		return err
	}
	return c.JSONStatus(http.StatusCreated, user)
}

func newValidationApp(t *testing.T) (*web.Handler, *base.Application) {
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
	// One connection on purpose: the in-memory database exists per
	// connection, so a second pooled connection would see an empty database.
	sqlDB.SetMaxOpenConns(1)
	if err := conn.Exec(
		"CREATE TABLE api_users (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL, email TEXT NOT NULL DEFAULT '')",
	).Error; err != nil {
		t.Fatalf("create table: %v", err)
	}
	// A pure JSON API endpoint: CSRF cookie validation is off (the exact
	// configuration switch this application shape is meant to use).
	app := base.New(config.Config{Web: config.Web{DisableCsrf: true}})
	app.SetDB(conn)
	handler := web.NewHandler(app)
	handler.Post("/user", userController{}.ActionCreate)
	return handler, app
}

func TestValidationErrorsReturnAsUnprocessableJSON(t *testing.T) {
	handler, app := newValidationApp(t)

	body := `{"name":"","email":"not-an-email"}`
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/user", strings.NewReader(body)))

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q, want the JSON content type", got)
	}
	var fields map[string][]string
	if err := json.Unmarshal(rec.Body.Bytes(), &fields); err != nil {
		t.Fatalf("decode body %q: %v", rec.Body.String(), err)
	}
	want := map[string][]string{
		"name":  {"name cannot be blank"},
		"email": {"email is not a valid address"},
	}
	for field, messages := range want {
		if got := fields[field]; len(got) != 1 || got[0] != messages[0] {
			t.Errorf("body[%s] = %q, want %q", field, got, messages)
		}
	}
	if len(fields) != len(want) {
		t.Errorf("body = %v, want exactly the fields name and email", fields)
	}
	// The rejected record never reached the database.
	appCtx := app.WithContext(context.Background())
	if n, err := ar.New[apiUser](appCtx).Count(); err != nil || n != 0 {
		t.Errorf("count after rejected POST = %d (err %v), want 0", n, err)
	}
}

func TestValidRecordSavesThroughTheEndpoint(t *testing.T) {
	handler, app := newValidationApp(t)

	body := `{"name":"ada","email":"ada@example.com"}`
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/user", strings.NewReader(body)))

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rec.Code)
	}
	var saved struct {
		ID    int64  `json:"ID"`
		Name  string `json:"Name"`
		Email string `json:"Email"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &saved); err != nil {
		t.Fatalf("decode body %q: %v", rec.Body.String(), err)
	}
	if saved.Name != "ada" || saved.Email != "ada@example.com" {
		t.Errorf("response record = %+v, want the saved fields", saved)
	}
	appCtx := app.WithContext(context.Background())
	if n, err := ar.New[apiUser](appCtx).Count(); err != nil || n != 1 {
		t.Errorf("count after valid POST = %d (err %v), want 1", n, err)
	}
}
