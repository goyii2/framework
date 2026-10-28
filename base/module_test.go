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

// TestModuleNestingReserved pins the reserved nesting seam: registering a
// module under a module must fail with a dedicated error until nesting is
// implemented.
func TestModuleNestingReserved(t *testing.T) {
	app := New(config.Default())
	parent, _ := NewModule(app, "/admin")
	child, _ := NewModule(app, "/admin/settings")

	if err := parent.RegisterModule(child); !errors.Is(err, ErrModuleNestingUnsupported) {
		t.Errorf("nested registration error = %v, want ErrModuleNestingUnsupported", err)
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
			m.RegisterController("ctrl", &fakeController{}) // duplicate ids are expected to error
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
