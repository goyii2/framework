package config

import (
	"testing"

	goyii "github.com/goyii2/framework/config"
)

// The tests here load the configuration files this example ships, against the
// struct this package defines — the walkthrough's overlay and environment
// demonstrations are its configuration, so drift between the struct, app.yaml
// and web-dev.yaml must fail a test instead of silently breaking the demo.
// They run in this package's directory, where the yaml files sit.

// TestLoadDevAppliesOverlay pins the dev story: web-dev.yaml overlays app.yaml
// (the provider key is the overlay's), and fields the overlay does not name
// keep their base values (the sender).
func TestLoadDevAppliesOverlay(t *testing.T) {
	t.Setenv("GOYII_ENV", "dev")
	t.Setenv("EMAIL_APIKEY", "") // empty counts as unset; shield the machine

	cfg, err := goyii.LoadAs[Config](".")
	if err != nil {
		t.Fatalf("LoadAs: %v", err)
	}
	if got, want := cfg.Email.APIKey, "dev-key-from-web-dev-overlay"; got != want {
		t.Fatalf("Email.APIKey = %q, want the web-dev.yaml overlay value %q", got, want)
	}
	if got, want := cfg.Email.Sender, "goyii basic <notifications@goyii.example>"; got != want {
		t.Fatalf("Email.Sender = %q, want the app.yaml base value %q", got, want)
	}
}

// TestLoadEnvironmentWithoutOverlayUsesBaseValues pins the other half: an
// environment with no web-<env>.yaml file reads app.yaml alone.
func TestLoadEnvironmentWithoutOverlayUsesBaseValues(t *testing.T) {
	t.Setenv("GOYII_ENV", "prod")
	t.Setenv("EMAIL_APIKEY", "")

	cfg, err := goyii.LoadAs[Config](".")
	if err != nil {
		t.Fatalf("LoadAs: %v", err)
	}
	if got, want := cfg.Email.APIKey, "base-key-from-app-yaml"; got != want {
		t.Fatalf("Email.APIKey = %q, want the app.yaml base value %q", got, want)
	}
	if got, want := cfg.Email.Sender, "goyii basic <notifications@goyii.example>"; got != want {
		t.Fatalf("Email.Sender = %q, want %q", got, want)
	}
}

// TestEmailAPIKeyEnvironmentOverride pins the third machine: EMAIL_APIKEY wins
// over both the overlay and the base file — the documented way to feed the
// credential in production.
func TestEmailAPIKeyEnvironmentOverride(t *testing.T) {
	t.Setenv("GOYII_ENV", "dev")
	t.Setenv("EMAIL_APIKEY", "key-from-the-environment")

	cfg, err := goyii.LoadAs[Config](".")
	if err != nil {
		t.Fatalf("LoadAs: %v", err)
	}
	if got, want := cfg.Email.APIKey, "key-from-the-environment"; got != want {
		t.Fatalf("Email.APIKey = %q, want the environment value %q", got, want)
	}
}

// TestLoadFillsIdentityAndComponentSections pins that the shipped files and
// the struct agree beyond the custom section: identity, the web listen value
// and the component parameter bundles all parse into their fields.
func TestLoadFillsIdentityAndComponentSections(t *testing.T) {
	t.Setenv("GOYII_ENV", "dev")
	t.Setenv("EMAIL_APIKEY", "")

	cfg, err := goyii.LoadAs[Config](".")
	if err != nil {
		t.Fatalf("LoadAs: %v", err)
	}
	if cfg.App.Name != "basic" || !cfg.App.Debug {
		t.Fatalf("App = %+v, want name basic with debug on", cfg.App)
	}
	if cfg.Web.Listen != "127.0.0.1:8090" {
		t.Fatalf("Web.Listen = %q, want 127.0.0.1:8090", cfg.Web.Listen)
	}
	if cfg.DB.Driver != "sqlite" || cfg.DB.DSN == "" {
		t.Fatalf("DB = %+v, want the sqlite walkthrough connection", cfg.DB)
	}
	if cfg.Queue.Driver != "db" || cfg.Queue.TableName != "queue" {
		t.Fatalf("Queue = %+v, want the db walkthrough queue", cfg.Queue)
	}
	if cfg.Cache.Driver != "file" || cfg.Cache.Path != "runtime/cache" {
		t.Fatalf("Cache = %+v, want the file walkthrough cache", cfg.Cache)
	}
	if cfg.Session.Backend != "file" || cfg.Session.Path != "runtime/session" {
		t.Fatalf("Session = %+v, want the file walkthrough session", cfg.Session)
	}
	if cfg.Mail.Driver != "log" || cfg.Mail.From == "" {
		t.Fatalf("Mail = %+v, want the log walkthrough mailer with a sender", cfg.Mail)
	}
	if cfg.I18n.Language != "en-US" {
		t.Fatalf("I18n.Language = %q, want en-US", cfg.I18n.Language)
	}
	if cfg.Log.Level != "debug" || len(cfg.Log.Targets) != 2 {
		t.Fatalf("Log = %+v, want the walkthrough logging setup", cfg.Log)
	}
}
