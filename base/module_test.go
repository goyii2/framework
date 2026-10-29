package base

import (
	"errors"
	"sync"
	"testing"

	"github.com/goyii2/framework/config"
)

// fakeController stands in for web-layer controller instances.
type fakeController struct{ name string }

func TestNewModuleValidatesPrefix(t *testing.T) {
	app := New(config.Default())

	ok := map[string]string{
		"/admin":  "/admin",
		"/admin/": "/admin", // trailing slash trimmed
	}
	for in, want := range ok {
		m, err := NewModule(app, in)
		if err != nil {
			t.Fatalf("NewModule(%q) unexpected error: %v", in, err)
		}
		if m.Prefix() != want {
			t.Errorf("NewModule(%q).Prefix() = %q, want %q", in, m.Prefix(), want)
		}
	}

	for _, in := range []string{"", "/", "admin", "admin/", "//"} {
		if _, err := NewModule(app, in); err == nil {
			t.Errorf("NewModule(%q) succeeded, want prefix validation error", in)
		}
	}
}

func TestModuleResolvesThroughOwningApplication(t *testing.T) {
	app := New(config.Default())
	m, err := NewModule(app, "/admin")
	if err != nil {
		t.Fatalf("NewModule unexpected error: %v", err)
	}
	if m.App() != app {
		t.Error("Module.App() must return the owning application")
	}
}

func TestModuleRegistersControllerOwnership(t *testing.T) {
	app := New(config.Default())
	m, _ := NewModule(app, "/admin")

	users := &fakeController{name: "user"}
	posts := &fakeController{name: "post"}
	if err := m.RegisterController("user", users); err != nil {
		t.Fatalf("RegisterController unexpected error: %v", err)
	}
	if err := m.RegisterController("post", posts); err != nil {
		t.Fatalf("RegisterController unexpected error: %v", err)
	}

	if got, ok := m.Controller("user"); !ok || got != any(users) {
		t.Errorf("Controller(%q) = %v, %v; want the registered instance", "user", got, ok)
	}
	if _, ok := m.Controller("missing"); ok {
		t.Error("Controller of an unregistered id must report miss")
	}

	ids := m.ControllerIDs()
	if len(ids) != 2 || ids[0] != "user" || ids[1] != "post" {
		t.Errorf("ControllerIDs() = %v, want [user post] in registration order", ids)
	}
}

func TestModuleRegisterControllerValidation(t *testing.T) {
	app := New(config.Default())
	m, _ := NewModule(app, "/admin")

	if err := m.RegisterController("user", &fakeController{}); err != nil {
		t.Fatalf("RegisterController unexpected error: %v", err)
	}
	if err := m.RegisterController("user", &fakeController{}); !errors.Is(err, ErrDuplicateControllerID) {
		t.Errorf("duplicate id error = %v, want ErrDuplicateControllerID", err)
	}
	for _, id := range []string{"", "a/b"} {
		if err := m.RegisterController(id, &fakeController{}); !errors.Is(err, ErrInvalidControllerID) {
			t.Errorf("RegisterController(%q) error = %v, want ErrInvalidControllerID", id, err)
		}
	}
}

func TestApplicationModuleRegistry(t *testing.T) {
	app := New(config.Default())
	admin, _ := NewModule(app, "/admin")
	blog, _ := NewModule(app, "/blog")
	if err := app.RegisterModule(admin); err != nil {
		t.Fatalf("RegisterModule unexpected error: %v", err)
	}
	if err := app.RegisterModule(blog); err != nil {
		t.Fatalf("RegisterModule unexpected error: %v", err)
	}

	if got, ok := app.Module("/admin"); !ok || got != admin {
		t.Error("Module(/admin) must return the registered instance")
	}
	if _, ok := app.Module("/missing"); ok {
		t.Error("Module of an unregistered prefix must report miss")
	}

	ms := app.Modules()
	if len(ms) != 2 || ms[0] != admin || ms[1] != blog {
		t.Errorf("Modules() = %v, want [%v %v] sorted by prefix", ms, admin, blog)
	}

	dup, _ := NewModule(app, "/admin")
	if err := app.RegisterModule(dup); !errors.Is(err, ErrDuplicateModulePrefix) {
		t.Errorf("duplicate prefix error = %v, want ErrDuplicateModulePrefix", err)
	}
	foreign, _ := NewModule(New(config.Default()), "/other")
	if err := app.RegisterModule(foreign); !errors.Is(err, ErrForeignModule) {
		t.Errorf("foreign module error = %v, want ErrForeignModule", err)
	}
	if err := app.RegisterModule(nil); !errors.Is(err, ErrNilModule) {
		t.Errorf("nil module error = %v, want ErrNilModule", err)
	}
}

// TestModuleNestingRegistersChild covers the nesting seam: a child module
// registered under its parent lands in the application table (so the router
// resolves its full prefix) and in the parent's child list, with the parent
// link walkable.
func TestModuleNestingRegistersChild(t *testing.T) {
	app := New(config.Default())
	admin, _ := NewModule(app, "/admin")
	if err := app.RegisterModule(admin); err != nil {
		t.Fatalf("registering parent: %v", err)
	}
	report, err := NewModule(app, "/admin/report")
	if err != nil {
		t.Fatalf("creating child: %v", err)
	}
	if err := admin.RegisterModule(report); err != nil {
		t.Fatalf("nesting child: %v", err)
	}

	if got, ok := app.Module("/admin/report"); !ok || got != report {
		t.Error("nested module must resolve through the application table")
	}
	if got := report.Parent(); got != admin {
		t.Error("child Parent() must return the module it was nested under")
	}
	if got := admin.Parent(); got != nil {
		t.Error("top-level module Parent() must be nil")
	}
	if ms := admin.Modules(); len(ms) != 1 || ms[0] != report {
		t.Errorf("admin.Modules() = %v, want [report]", ms)
	}
	if ms := report.Modules(); len(ms) != 0 {
		t.Errorf("leaf module Modules() = %v, want empty", ms)
	}

	// A second nesting level hangs off the intermediate module.
	deep, _ := NewModule(app, "/admin/report/deep")
	if err := report.RegisterModule(deep); err != nil {
		t.Fatalf("nesting grandchild: %v", err)
	}
	if deep.Parent() != report {
		t.Error("grandchild Parent() must be the intermediate module")
	}
}

// TestModuleNestingRejectedPairs pins the wiring mistakes nesting refuses:
// nil and foreign modules, prefixes unrelated to or deeper than the parent,
// and prefixes already taken anywhere in the application.
func TestModuleNestingRejectedPairs(t *testing.T) {
	app := New(config.Default())
	admin, _ := NewModule(app, "/admin")
	if err := app.RegisterModule(admin); err != nil {
		t.Fatalf("registering parent: %v", err)
	}

	if err := admin.RegisterModule(nil); !errors.Is(err, ErrNilModule) {
		t.Errorf("nil child error = %v, want ErrNilModule", err)
	}
	foreign, _ := NewModule(New(config.Default()), "/admin/report")
	if err := admin.RegisterModule(foreign); !errors.Is(err, ErrForeignModule) {
		t.Errorf("foreign child error = %v, want ErrForeignModule", err)
	}
	for _, prefix := range []string{
		"/blog",              // unrelated prefix
		"/admin",             // the parent itself
		"/administrator",     // sharing the parent's stem is not being below it
		"/admin/report/deep", // depth beyond one segment must nest through /admin/report
	} {
		child, err := NewModule(app, prefix)
		if err != nil {
			t.Fatalf("NewModule(%q) unexpected error: %v", prefix, err)
		}
		if err := admin.RegisterModule(child); !errors.Is(err, ErrInvalidSubmodulePrefix) {
			t.Errorf("nesting %q under /admin error = %v, want ErrInvalidSubmodulePrefix", prefix, err)
		}
	}

	// Prefix uniqueness is global: the same prefix cannot land twice,
	// whatever registration path is used.
	report, _ := NewModule(app, "/admin/report")
	if err := admin.RegisterModule(report); err != nil {
		t.Fatalf("nesting child: %v", err)
	}
	dup, _ := NewModule(app, "/admin/report")
	if err := admin.RegisterModule(dup); !errors.Is(err, ErrDuplicateModulePrefix) {
		t.Errorf("duplicate child error = %v, want ErrDuplicateModulePrefix", err)
	}
	if err := app.RegisterModule(dup); !errors.Is(err, ErrDuplicateModulePrefix) {
		t.Errorf("duplicate via application error = %v, want ErrDuplicateModulePrefix", err)
	}
	// A rejected duplicate leaves no trace in the tree.
	if ms := admin.Modules(); len(ms) != 1 {
		t.Errorf("admin.Modules() after rejected duplicates = %v, want one child", ms)
	}
}

// TestModuleConcurrentAccess mixes controller registration with lookups from
// many goroutines; run with -race.
func TestModuleConcurrentAccess(t *testing.T) {
	app := New(config.Default())
	m, _ := NewModule(app, "/admin")
	if err := app.RegisterModule(m); err != nil {
		t.Fatalf("RegisterModule unexpected error: %v", err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = m.RegisterController("ctrl", &fakeController{}) // duplicate ids are expected to error
			_, _ = m.Controller("ctrl")
			_, _ = app.Module("/admin")
			_ = app.Modules()
		}(i)
	}
	wg.Wait()

	if ids := m.ControllerIDs(); len(ids) != 1 || ids[0] != "ctrl" {
		t.Errorf("ControllerIDs() after concurrent registration = %v, want [ctrl]", ids)
	}
}
