package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadEnvDevOverlay(t *testing.T) {
	cfg, err := LoadEnv("testdata", "dev")
	if err != nil {
		t.Fatalf("LoadEnv: %v", err)
	}

	// Base values from app.yaml.
	if cfg.App.Name != "demo" || cfg.App.Timezone != "Asia/Shanghai" {
		t.Errorf("base app values lost: %+v", cfg.App)
	}
	if cfg.Web.Listen != "127.0.0.1:9000" {
		t.Errorf("Web.Listen = %q, want base value", cfg.Web.Listen)
	}
	if cfg.Session.Name != "DEMOSID" || cfg.Session.Timeout != 3600 {
		t.Errorf("base session values lost: %+v", cfg.Session)
	}

	// Overrides from web-dev.yaml.
	if !cfg.App.Debug {
		t.Error("App.Debug must be overridden to true by web-dev.yaml")
	}
	if cfg.Log.Level != "debug" {
		t.Errorf("Log.Level = %q, want dev override %q", cfg.Log.Level, "debug")
	}
	if cfg.DB.DSN != "root:dev@tcp(127.0.0.1:3306)/demo_dev" {
		t.Errorf("DB.DSN = %q, want dev override", cfg.DB.DSN)
	}
	// The driver key is absent from the overlay and keeps its base value.
	if cfg.DB.Driver != "mysql" {
		t.Errorf("DB.Driver = %q, want base value %q", cfg.DB.Driver, "mysql")
	}

	if cfg.Env != "dev" {
		t.Errorf("Env = %q, want %q", cfg.Env, "dev")
	}
}

func TestLoadEnvProdOverlay(t *testing.T) {
	cfg, err := LoadEnv("testdata", "prod")
	if err != nil {
		t.Fatalf("LoadEnv: %v", err)
	}
	if cfg.Web.Listen != ":80" {
		t.Errorf("Web.Listen = %q, want prod override %q", cfg.Web.Listen, ":80")
	}
	if cfg.App.Debug {
		t.Error("App.Debug must be false in prod")
	}
	if cfg.Log.Level != "info" {
		t.Errorf("Log.Level = %q, want base value %q", cfg.Log.Level, "info")
	}
	if cfg.DB.DSN == "" {
		t.Error("base DB.DSN lost when overlay omits the key")
	}
}

func TestLoadEnvWithoutOverlayUsesBaseOnly(t *testing.T) {
	cfg, err := LoadEnv("testdata", "test")
	if err != nil {
		t.Fatalf("LoadEnv without overlay file: %v", err)
	}
	if cfg.App.Name != "demo" {
		t.Errorf("App.Name = %q, want base value", cfg.App.Name)
	}
	if cfg.Log.Level != "info" {
		t.Errorf("Log.Level = %q, want base value", cfg.Log.Level)
	}
}

func TestEnvOverrides(t *testing.T) {
	t.Setenv("APP_NAME", "from-env")
	t.Setenv("APP_DEBUG", "false")
	t.Setenv("DB_DSN", "env-dsn")
	t.Setenv("WEB_LISTEN", ":9999")
	t.Setenv("CACHE_REDIS_DB", "7")
	t.Setenv("SESSION_TIMEOUT", "60")

	cfg, err := LoadEnv("testdata", "dev")
	if err != nil {
		t.Fatalf("LoadEnv: %v", err)
	}

	if cfg.App.Name != "from-env" {
		t.Errorf("APP_NAME override not applied: %q", cfg.App.Name)
	}
	if cfg.App.Debug {
		t.Error("APP_DEBUG=false must override the dev overlay's true")
	}
	if cfg.DB.DSN != "env-dsn" {
		t.Errorf("DB_DSN override not applied: %q", cfg.DB.DSN)
	}
	if cfg.Web.Listen != ":9999" {
		t.Errorf("WEB_LISTEN override not applied: %q", cfg.Web.Listen)
	}
	if cfg.Cache.Redis.DB != 7 {
		t.Errorf("CACHE_REDIS_DB override not applied: %d", cfg.Cache.Redis.DB)
	}
	if cfg.Session.Timeout != 60 {
		t.Errorf("SESSION_TIMEOUT override not applied: %d", cfg.Session.Timeout)
	}
}

func TestEnvOverrideInvalidValueFails(t *testing.T) {
	t.Setenv("APP_DEBUG", "not-a-bool")
	if _, err := LoadEnv("testdata", "dev"); err == nil {
		t.Fatal("invalid APP_DEBUG value must fail the load")
	} else if !strings.Contains(err.Error(), "APP_DEBUG") {
		t.Errorf("error %q does not name the offending variable APP_DEBUG", err)
	}
}

func TestLoadSelectsEnvironmentFromGOYIIEnv(t *testing.T) {
	t.Setenv(EnvVarName, "prod")
	cfg, err := Load("testdata")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Env != "prod" {
		t.Errorf("Env = %q, want %q", cfg.Env, "prod")
	}
	if cfg.Web.Listen != ":80" {
		t.Errorf("Web.Listen = %q, want prod overlay value", cfg.Web.Listen)
	}
}

func TestCurrentEnv(t *testing.T) {
	t.Setenv(EnvVarName, "")
	if got := CurrentEnv(); got != "dev" {
		t.Errorf("CurrentEnv() = %q, want default %q", got, "dev")
	}
	t.Setenv(EnvVarName, "staging")
	if got := CurrentEnv(); got != "staging" {
		t.Errorf("CurrentEnv() = %q, want %q", got, "staging")
	}
}

func TestLoadEnvMissingFilesFail(t *testing.T) {
	dir := filepath.Join(os.TempDir(), "goyii-config-missing")
	if _, err := LoadEnv(dir, "dev"); err == nil {
		t.Fatal("loading from a directory without config files must fail")
	}
}
