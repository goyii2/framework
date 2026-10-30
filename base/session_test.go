package base

import (
	"testing"

	"github.com/goyii2/framework/session"
)

// TestApplicationSessionSlot verifies the session component slot: the file
// fallback installs eagerly, a configured manager replaces it, nil restores
// the fallback.
func TestApplicationSessionSlot(t *testing.T) {
	app := New()

	if app.Session() == nil {
		t.Fatal("Session() = nil after construction")
	}
	if _, ok := app.Session().Store.(*session.FileStore); !ok {
		t.Fatalf("fallback session backend = %T, want *session.FileStore", app.Session().Store)
	}

	m, err := session.New(session.Config{Name: "DEMOSID", Backend: "file", Path: t.TempDir(), Timeout: 300})
	if err != nil {
		t.Fatalf("session.New(file): %v", err)
	}
	app.SetSession(m)
	if app.Session() != m {
		t.Error("SetSession must install the manager Session returns")
	}
	if app.Session().CookieName != "DEMOSID" {
		t.Errorf("installed manager cookie name = %q, want DEMOSID", app.Session().CookieName)
	}

	app.SetSession(nil)
	if app.Session() == m {
		t.Error("SetSession(nil) must restore the file fallback")
	}
	if app.Session().CookieName != "goyii-session" {
		t.Errorf("restored fallback cookie name = %q, want goyii-session", app.Session().CookieName)
	}
}
