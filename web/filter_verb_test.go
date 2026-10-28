package web_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/goyii2/framework/base"
	"github.com/goyii2/framework/config"
	"github.com/goyii2/framework/web"
)

func TestVerbFilterAllowsListedVerbs(t *testing.T) {
	app := base.New(config.Config{Web: config.Web{DisableCsrf: true}})
	h := web.NewHandler(app)
	h.Use(web.VerbFilter{Allowed: []string{"get", "post"}}.Middleware())
	h.Handle("/thing", func(c *web.Context) error {
		return c.JSON(map[string]string{"m": c.Request().Method})
	})

	for _, m := range []string{http.MethodGet, http.MethodPost} {
		if res := req(t, h, m, "/thing"); res.StatusCode != http.StatusOK {
			t.Errorf("%s status = %d, want 200 (listed verb)", m, res.StatusCode)
		}
	}
}

func TestVerbFilterRejectsUnlistedVerb405WithAllow(t *testing.T) {
	app := base.New(config.Config{Web: config.Web{DisableCsrf: true}})
	h := web.NewHandler(app)
	h.Use(web.VerbFilter{Allowed: []string{"POST", "GET"}}.Middleware())
	h.Handle("/thing", func(c *web.Context) error { return nil })

	r := httptest.NewRequest(http.MethodPut, "/thing", nil)
	r.Header.Set("Accept", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	res := rec.Result()

	if res.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", res.StatusCode)
	}
	if allow := res.Header.Get("Allow"); allow != "GET, POST" {
		t.Errorf("Allow = %q, want sorted \"GET, POST\"", allow)
	}
}

func TestVerbFilterRulesScopeByPath(t *testing.T) {
	app := base.New(config.Config{Web: config.Web{DisableCsrf: true}})
	h := web.NewHandler(app)
	h.Use(web.VerbFilter{Rules: []web.VerbRule{
		{Path: "/api/thing", Verbs: []string{"GET"}},
	}}.Middleware())
	h.Handle("/api/thing", func(c *web.Context) error { return nil })
	h.Handle("/other", func(c *web.Context) error { return nil })

	if res := req(t, h, http.MethodPut, "/api/thing"); res.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("PUT /api/thing status = %d, want 405 (rule applies)", res.StatusCode)
	}
	if res := req(t, h, http.MethodPut, "/other"); res.StatusCode != http.StatusOK {
		t.Errorf("PUT /other status = %d, want 200 (path without a rule is unfiltered)", res.StatusCode)
	}
}

func TestVerbFilterWorksStandaloneWithoutWebContext(t *testing.T) {
	var actionRan bool
	next := web.VerbFilter{Allowed: []string{"GET"}}.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		actionRan = true
	}))

	rec := httptest.NewRecorder()
	next.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/x", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
	if allow := rec.Header().Get("Allow"); allow != "GET" {
		t.Errorf("Allow = %q, want GET", allow)
	}
	if actionRan {
		t.Error("action ran for a rejected verb")
	}
}

func TestVerbFilterAtControllerLevel(t *testing.T) {
	app := base.New(config.Config{Web: config.Web{DisableCsrf: true}})
	h := web.NewHandler(app)

	ctrl := verbController{}
	h.Ctrl(ctrl).Handle("/pay", ctrl.ActionPay)

	if res := req(t, h, http.MethodPost, "/pay"); res.StatusCode != http.StatusOK {
		t.Errorf("POST /pay status = %d, want 200", res.StatusCode)
	}
	res := req(t, h, http.MethodGet, "/pay")
	if res.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("GET /pay status = %d, want 405", res.StatusCode)
	}
	if allow := res.Header.Get("Allow"); allow != "POST" {
		t.Errorf("Allow = %q, want POST", allow)
	}
}

// verbController accepts payments by POST only.
type verbController struct {
	web.Controller
}

func (verbController) Middleware() []web.Middleware {
	return []web.Middleware{web.VerbFilter{Allowed: []string{"POST"}}.Middleware()}
}

func (verbController) ActionPay(c *web.Context) error {
	return c.JSON(map[string]string{"m": "paid"})
}
