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

func TestLoadLogSection(t *testing.T) {
	dir := t.TempDir()
	appYAML := `
app:
  name: logdemo
log:
  level: debug
  targets:
    - type: console
      minLevel: warn
      categories: ["web*", "db"]
      except: ["web/session"]
    - type: file
      path: logs/app.log
      maxSizeMB: 20
      maxFiles: 7
  gorm:
    slowThresholdMs: 500
`
	if err := os.WriteFile(filepath.Join(dir, "app.yaml"), []byte(appYAML), 0o644); err != nil {
		t.Fatalf("writing app.yaml: %v", err)
	}

	cfg, err := LoadEnv(dir, "dev")
	if err != nil {
		t.Fatalf("LoadEnv: %v", err)
	}

	if cfg.Log.Level != "debug" {
		t.Errorf("Log.Level = %q, want %q", cfg.Log.Level, "debug")
	}
	if len(cfg.Log.Targets) != 2 {
		t.Fatalf("Log.Targets = %+v, want 2 entries", cfg.Log.Targets)
	}

	console := cfg.Log.Targets[0]
	if console.Type != "console" || console.MinLevel != "warn" {
		t.Errorf("console target = %+v", console)
	}
	if strings.Join(console.Categories, ",") != "web*,db" {
		t.Errorf("console categories = %v", console.Categories)
	}
	if len(console.Except) != 1 || console.Except[0] != "web/session" {
		t.Errorf("console except = %v", console.Except)
	}

	file := cfg.Log.Targets[1]
	if file.Type != "file" || file.Path != "logs/app.log" || file.MaxSizeMB != 20 || file.MaxFiles != 7 {
		t.Errorf("file target = %+v", file)
	}

	if cfg.Log.Gorm.Disable {
		t.Error("gorm adapter must be enabled by default")
	}
	if cfg.Log.Gorm.SlowThresholdMs != 500 {
		t.Errorf("gorm slow threshold = %d, want 500", cfg.Log.Gorm.SlowThresholdMs)
	}
}

func TestLogDefaults(t *testing.T) {
	cfg := WithDefaults(Config{})

	if cfg.Log.Gorm.SlowThresholdMs != 200 {
		t.Errorf("default gorm slow threshold = %d, want 200", cfg.Log.Gorm.SlowThresholdMs)
	}
	if cfg.Log.Gorm.Disable {
		t.Error("the gorm adapter must default to enabled")
	}
	if len(cfg.Log.Targets) != 0 {
		t.Errorf("WithDefaults must not invent targets, got %+v", cfg.Log.Targets)
	}
}

func TestLoadMailSection(t *testing.T) {
	dir := t.TempDir()
	appYAML := `
app:
  name: maildemo
mail:
  driver: smtp
  from: Robot <noreply@example.com>
  smtp:
    host: mail.example.com
    port: 465
    username: alerts
    password: s3cret
    encryption: smtps
`
	if err := os.WriteFile(filepath.Join(dir, "app.yaml"), []byte(appYAML), 0o644); err != nil {
		t.Fatalf("writing app.yaml: %v", err)
	}

	cfg, err := LoadEnv(dir, "prod")
	if err != nil {
		t.Fatalf("LoadEnv: %v", err)
	}
	if cfg.Mail.Driver != "smtp" || cfg.Mail.From != "Robot <noreply@example.com>" {
		t.Errorf("mail section = %+v", cfg.Mail)
	}
	smtp := cfg.Mail.SMTP
	if smtp.Host != "mail.example.com" || smtp.Port != 465 || smtp.Username != "alerts" ||
		smtp.Password != "s3cret" || smtp.Encryption != "smtps" {
		t.Errorf("smtp section = %+v", smtp)
	}
}

func TestMailDefaults(t *testing.T) {
	cfg := WithDefaults(Config{})
	if cfg.Mail.Driver != "log" {
		t.Errorf("default mail driver = %q, want log", cfg.Mail.Driver)
	}
	if cfg.Mail.From != "" || cfg.Mail.SMTP.Host != "" {
		t.Errorf("WithDefaults must not invent mail values, got %+v", cfg.Mail)
	}
}

func TestMailEnvOverrides(t *testing.T) {
	t.Setenv("MAIL_DRIVER", "smtp")
	t.Setenv("MAIL_FROM", "env@example.com")
	t.Setenv("MAIL_SMTP_HOST", "mail.env.example.com")
	t.Setenv("MAIL_SMTP_PORT", "2525")
	t.Setenv("MAIL_SMTP_ENCRYPTION", "none")

	cfg, err := LoadEnv("testdata", "dev")
	if err != nil {
		t.Fatalf("LoadEnv: %v", err)
	}
	if cfg.Mail.Driver != "smtp" || cfg.Mail.From != "env@example.com" {
		t.Errorf("MAIL_DRIVER/MAIL_FROM overrides not applied: %+v", cfg.Mail)
	}
	if cfg.Mail.SMTP.Host != "mail.env.example.com" || cfg.Mail.SMTP.Port != 2525 ||
		cfg.Mail.SMTP.Encryption != "none" {
		t.Errorf("MAIL_SMTP_* overrides not applied: %+v", cfg.Mail.SMTP)
	}
}

func TestLoadRedisSection(t *testing.T) {
	dir := t.TempDir()
	appYAML := `
app:
  name: redisdemo
redis:
  addr: redis.internal:6379
  password: top-secret
  db: 2
cache:
  driver: redis
session:
  backend: redis
  redis:
    addr: redis.session:6379
`
	if err := os.WriteFile(filepath.Join(dir, "app.yaml"), []byte(appYAML), 0o644); err != nil {
		t.Fatalf("writing app.yaml: %v", err)
	}

	cfg, err := LoadEnv(dir, "dev")
	if err != nil {
		t.Fatalf("LoadEnv: %v", err)
	}
	// The top-level section carries the shared connection's values.
	if cfg.Redis != (Redis{Addr: "redis.internal:6379", Password: "top-secret", DB: 2}) {
		t.Errorf("top-level redis section = %+v", cfg.Redis)
	}
	// Sections keep their own values verbatim: the cache section stays empty
	// (it inherits the shared connection at wiring time, not in the config)
	// and the session section keeps its override.
	if cfg.Cache.Redis != (Redis{}) {
		t.Errorf("empty cache redis section must stay empty, got %+v", cfg.Cache.Redis)
	}
	if cfg.Session.Redis != (Redis{Addr: "redis.session:6379"}) {
		t.Errorf("session redis override = %+v", cfg.Session.Redis)
	}
}

func TestRedisEnvOverrides(t *testing.T) {
	t.Setenv("REDIS_ADDR", "redis.env:6380")
	t.Setenv("REDIS_DB", "5")

	cfg, err := LoadEnv("testdata", "dev")
	if err != nil {
		t.Fatalf("LoadEnv: %v", err)
	}
	if cfg.Redis.Addr != "redis.env:6380" || cfg.Redis.DB != 5 {
		t.Errorf("REDIS_* overrides not applied: %+v", cfg.Redis)
	}
}

func TestRedisDefaults(t *testing.T) {
	cfg := WithDefaults(Config{})
	if cfg.Redis != (Redis{}) {
		t.Errorf("WithDefaults must not invent redis values, got %+v", cfg.Redis)
	}
}

func TestLoadMutexSection(t *testing.T) {
	dir := t.TempDir()
	appYAML := `
app:
  name: mutexdemo
mutex:
  driver: redis
`
	if err := os.WriteFile(filepath.Join(dir, "app.yaml"), []byte(appYAML), 0o644); err != nil {
		t.Fatalf("writing app.yaml: %v", err)
	}

	cfg, err := LoadEnv(dir, "dev")
	if err != nil {
		t.Fatalf("LoadEnv: %v", err)
	}
	if cfg.Mutex != (Mutex{Driver: "redis"}) {
		t.Errorf("mutex section = %+v", cfg.Mutex)
	}
}

func TestMutexDefaults(t *testing.T) {
	cfg := WithDefaults(Config{})
	if cfg.Mutex.Driver != "process" {
		t.Errorf("default mutex driver = %q, want process", cfg.Mutex.Driver)
	}
}

func TestMutexEnvOverrides(t *testing.T) {
	t.Setenv("MUTEX_DRIVER", "redis")

	cfg, err := LoadEnv("testdata", "dev")
	if err != nil {
		t.Fatalf("LoadEnv: %v", err)
	}
	if cfg.Mutex.Driver != "redis" {
		t.Errorf("MUTEX_DRIVER override not applied: %+v", cfg.Mutex)
	}
}
