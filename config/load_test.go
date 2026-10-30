package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/goyii2/framework/cache"
	"github.com/goyii2/framework/config"
	"github.com/goyii2/framework/db"
)

// projectConfig is the shape a concrete project defines: identity and server
// values of its own plus the parameter bundles of the component packages it
// uses, plus fully custom sections. The loader must fill any such struct —
// it owns none of them.
type projectConfig struct {
	App struct {
		Name  string `yaml:"name"`
		Debug bool   `yaml:"debug"`
	} `yaml:"app"`
	Web struct {
		Listen string `yaml:"listen"`
	} `yaml:"web"`
	DB    db.Config    `yaml:"db"`
	Cache cache.Config `yaml:"cache"`
	Log   struct {
		Level string `yaml:"level"`
	} `yaml:"log"`
	Session struct {
		Name    string `yaml:"name"`
		Backend string `yaml:"backend"`
		Timeout int    `yaml:"timeout"`
	} `yaml:"session"`
	Email struct {
		APIKey      string        `yaml:"apiKey"`
		Retries     int           `yaml:"retries"`
		SendTimeout time.Duration `yaml:"sendTimeout"`
	} `yaml:"email"`
}

// loadProject loads the testdata configuration as the project type; the
// environment is pinned to dev so the overlay applies deterministically.
func loadProject(t *testing.T) projectConfig {
	t.Helper()
	t.Setenv(config.EnvVarName, "")
	cfg, err := config.LoadAs[projectConfig]("testdata")
	if err != nil {
		t.Fatalf("LoadAs: %v", err)
	}
	return cfg
}

func TestLoadAsAppliesDevOverlay(t *testing.T) {
	cfg := loadProject(t)

	// Base values from app.yaml.
	if cfg.App.Name != "demo" {
		t.Errorf("App.Name = %q, want base value", cfg.App.Name)
	}
	if cfg.Web.Listen != "127.0.0.1:9000" {
		t.Errorf("Web.Listen = %q, want base value", cfg.Web.Listen)
	}
	if cfg.Session.Name != "DEMOSID" || cfg.Session.Timeout != 3600 {
		t.Errorf("base session values lost: %+v", cfg.Session)
	}
	// Component parameter bundles fill like any other struct.
	if cfg.DB.Driver != "mysql" {
		t.Errorf("DB.Driver = %q, want base value", cfg.DB.Driver)
	}
	if cfg.Cache.Redis.Addr != "127.0.0.1:6379" || cfg.Cache.Redis.DB != 1 {
		t.Errorf("nested cache redis section = %+v, want base values", cfg.Cache.Redis)
	}
	if cfg.Email.APIKey != "base-key" || cfg.Email.Retries != 3 {
		t.Errorf("custom email section = %+v, want base values", cfg.Email)
	}

	// Overrides from web-dev.yaml.
	if !cfg.App.Debug {
		t.Error("App.Debug must be overridden to true by web-dev.yaml")
	}
	if cfg.Log.Level != "debug" {
		t.Errorf("Log.Level = %q, want dev override", cfg.Log.Level)
	}
	if cfg.DB.DSN != "root:dev@tcp(127.0.0.1:3306)/demo_dev" {
		t.Errorf("DB.DSN = %q, want dev override", cfg.DB.DSN)
	}
}

func TestLoadAsWithoutOverlayUsesBaseOnly(t *testing.T) {
	t.Setenv(config.EnvVarName, "test")
	cfg, err := config.LoadAs[projectConfig]("testdata")
	if err != nil {
		t.Fatalf("LoadAs without overlay file: %v", err)
	}
	if cfg.App.Name != "demo" {
		t.Errorf("App.Name = %q, want base value", cfg.App.Name)
	}
	if cfg.Log.Level != "info" {
		t.Errorf("Log.Level = %q, want base value", cfg.Log.Level)
	}
}

func TestLoadAsSelectsEnvironmentFromGOYIIEnv(t *testing.T) {
	t.Setenv(config.EnvVarName, "prod")
	cfg, err := config.LoadAs[projectConfig]("testdata")
	if err != nil {
		t.Fatalf("LoadAs: %v", err)
	}
	if cfg.Web.Listen != ":80" {
		t.Errorf("Web.Listen = %q, want prod override", cfg.Web.Listen)
	}
	if cfg.App.Debug {
		t.Error("App.Debug must be false in prod")
	}
	// The prod overlay omits the log level; the base value stands.
	if cfg.Log.Level != "info" {
		t.Errorf("Log.Level = %q, want base value", cfg.Log.Level)
	}
}

func TestEnv(t *testing.T) {
	t.Setenv(config.EnvVarName, "")
	if got := config.Env(); got != "dev" {
		t.Errorf("Env() = %q, want default %q", got, "dev")
	}
	t.Setenv(config.EnvVarName, "staging")
	if got := config.Env(); got != "staging" {
		t.Errorf("Env() = %q, want %q", got, "staging")
	}
}

func TestLoadAsEnvOverrides(t *testing.T) {
	t.Setenv(config.EnvVarName, "")
	t.Setenv("APP_NAME", "from-env")
	t.Setenv("APP_DEBUG", "false")
	t.Setenv("DB_DSN", "env-dsn")
	t.Setenv("WEB_LISTEN", ":9999")
	t.Setenv("CACHE_REDIS_DB", "7")
	t.Setenv("SESSION_TIMEOUT", "60")
	t.Setenv("EMAIL_APIKEY", "env-key")
	t.Setenv("EMAIL_SENDTIMEOUT", "90s")

	cfg, err := config.LoadAs[projectConfig]("testdata")
	if err != nil {
		t.Fatalf("LoadAs: %v", err)
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
	if cfg.Email.APIKey != "env-key" {
		t.Errorf("EMAIL_APIKEY override not applied: %q", cfg.Email.APIKey)
	}
	if cfg.Email.SendTimeout != 90*time.Second {
		t.Errorf("EMAIL_SENDTIMEOUT override not applied: %s", cfg.Email.SendTimeout)
	}
}

func TestLoadAsEnvOverrideInvalidValueFails(t *testing.T) {
	t.Setenv(config.EnvVarName, "")
	t.Setenv("EMAIL_RETRIES", "not-a-number")
	if _, err := config.LoadAs[projectConfig]("testdata"); err == nil {
		t.Fatal("invalid EMAIL_RETRIES value must fail the load")
	} else if !strings.Contains(err.Error(), "EMAIL_RETRIES") {
		t.Errorf("error %q does not name the offending variable EMAIL_RETRIES", err)
	}
}

func TestLoadAsMissingFilesFail(t *testing.T) {
	dir := filepath.Join(os.TempDir(), "goyii-config-missing")
	if _, err := config.LoadAs[projectConfig](dir); err == nil {
		t.Fatal("loading from a directory without config files must fail")
	} else if !strings.Contains(err.Error(), dir) {
		t.Errorf("error %q must name the directory it searched", err)
	}
}

func TestLoadAsFillsNoDefaults(t *testing.T) {
	dir := t.TempDir()
	appYAML := `
app:
  name: bare
`
	if err := os.WriteFile(filepath.Join(dir, "app.yaml"), []byte(appYAML), 0o644); err != nil {
		t.Fatalf("writing app.yaml: %v", err)
	}

	cfg, err := config.LoadAs[projectConfig](dir)
	if err != nil {
		t.Fatalf("LoadAs: %v", err)
	}
	// The loader fills no defaults: absent values stay zero, defaulting is
	// the component constructors' business.
	if cfg.Log.Level != "" {
		t.Errorf("Log.Level = %q, want the zero value", cfg.Log.Level)
	}
	if cfg.Cache.TTL != 0 {
		t.Errorf("Cache.TTL = %d, want the zero value", cfg.Cache.TTL)
	}
	if cfg.Session.Timeout != 0 {
		t.Errorf("Session.Timeout = %d, want the zero value", cfg.Session.Timeout)
	}
}

func TestLoadAsRejectsNonStructTarget(t *testing.T) {
	if _, err := config.LoadAs[string]("testdata"); err == nil {
		t.Fatal("LoadAs into a non-struct type must fail")
	}
}
