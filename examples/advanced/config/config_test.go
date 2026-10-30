package config

import (
	"testing"

	goyii "github.com/goyii2/framework/config"
)

// The tests here load the configuration files this example ships, against
// the struct this package defines — the advanced template's environment
// story is its configuration, so drift between the struct, app.yaml and
// web-dev.yaml must fail a test instead of silently breaking the demo.
// They run in this package's directory, where the yaml files sit.

// TestLoadDevAppliesOverlay pins the dev story: web-dev.yaml overlays
// app.yaml (debug flips on), and fields the overlay does not name keep
// their base values.
func TestLoadDevAppliesOverlay(t *testing.T) {
	t.Setenv("GOYII_ENV", "dev")
	t.Setenv("APP_DEBUG", "") // empty counts as unset; shield the machine

	cfg, err := goyii.LoadAs[Config](".")
	if err != nil {
		t.Fatalf("LoadAs: %v", err)
	}
	if !cfg.App.Debug {
		t.Fatal("App.Debug = false, want the web-dev.yaml overlay to switch it on")
	}
	if cfg.App.Name != "advanced" {
		t.Fatalf("App.Name = %q, want the app.yaml base value %q", cfg.App.Name, "advanced")
	}
}

// TestLoadEnvironmentWithoutOverlayUsesBaseValues pins the other half: an
// environment with no web-<env>.yaml file reads app.yaml alone — the base
// keeps the terse debug setting.
func TestLoadEnvironmentWithoutOverlayUsesBaseValues(t *testing.T) {
	t.Setenv("GOYII_ENV", "prod")
	t.Setenv("APP_DEBUG", "")

	cfg, err := goyii.LoadAs[Config](".")
	if err != nil {
		t.Fatalf("LoadAs: %v", err)
	}
	if cfg.App.Debug {
		t.Fatal("App.Debug = true, want the app.yaml base value (off) with no overlay present")
	}
}

// TestDebugEnvironmentOverride pins the third machine: APP_DEBUG wins over
// both the overlay and the base file.
func TestDebugEnvironmentOverride(t *testing.T) {
	t.Setenv("GOYII_ENV", "prod")
	t.Setenv("APP_DEBUG", "true")

	cfg, err := goyii.LoadAs[Config](".")
	if err != nil {
		t.Fatalf("LoadAs: %v", err)
	}
	if !cfg.App.Debug {
		t.Fatal("App.Debug = false, want the environment value to win")
	}
}

// TestLoadFillsIdentityAndComponentSections pins that the shipped files and
// the struct agree: identity, the web listen value and the component
// parameter bundles all parse into their fields.
func TestLoadFillsIdentityAndComponentSections(t *testing.T) {
	t.Setenv("GOYII_ENV", "dev")
	t.Setenv("APP_DEBUG", "")

	cfg, err := goyii.LoadAs[Config](".")
	if err != nil {
		t.Fatalf("LoadAs: %v", err)
	}
	if cfg.App.Name != "advanced" {
		t.Fatalf("App.Name = %q, want advanced", cfg.App.Name)
	}
	if cfg.Web.Listen != "127.0.0.1:8090" {
		t.Fatalf("Web.Listen = %q, want 127.0.0.1:8090", cfg.Web.Listen)
	}
	if cfg.DB.Driver != "sqlite" || cfg.DB.DSN == "" {
		t.Fatalf("DB = %+v, want the sqlite connection", cfg.DB)
	}
	if cfg.Log.Level != "info" || cfg.Log.Gorm.SlowThresholdMs != 200 {
		t.Fatalf("Log = %+v, want the lean logging setup", cfg.Log)
	}
}
