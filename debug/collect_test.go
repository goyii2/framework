package debug

import (
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/goyii2/framework/config"
	"github.com/goyii2/framework/log"
)

func TestMaskDSN(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"url style", "postgres://user:secret@localhost:5432/app?sslmode=disable", "postgres://user:***@localhost:5432/app?sslmode=disable"},
		{"url style no port", "mysql://root:hunter2@db.internal/mydb", "mysql://root:***@db.internal/mydb"},
		{"mysql style", "root:pw123@tcp(127.0.0.1:3306)/db?parseTime=true", "root:***@tcp(127.0.0.1:3306)/db?parseTime=true"},
		{"sqlserver url style", "sqlserver://sa:mypass@host:1433/instance", "sqlserver://sa:***@host:1433/instance"},
		{"key=value style", "user id=sa;password=hunter2;server=localhost", "user id=sa;password=***;server=localhost"},
		{"key=value spaces", "user id = sa; password = hunter2 ; encrypt=disable", "user id = sa; password = ***; encrypt=disable"},
		{"key=value final pair", "server=localhost;password=topsecret", "server=localhost;password=***"},
		{"no password url", "postgres://localhost/app", "postgres://localhost/app"},
		{"no password kv", "server=localhost;encrypt=disable", "server=localhost;encrypt=disable"},
		{"empty", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := maskDSN(tc.in); got != tc.want {
				t.Errorf("maskDSN(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestMaskDSNNeverLeavesPassword(t *testing.T) {
	secret := "sup3rs3cret"
	dsns := []string{
		"postgres://ada:" + secret + "@localhost:5432/app",
		"ada:" + secret + "@tcp(localhost:3306)/app",
		"user id=sa;password=" + secret + ";server=localhost",
	}
	for _, dsn := range dsns {
		got := maskDSN(dsn)
		if strings.Contains(strings.ToLower(got), strings.ToLower(secret)) {
			t.Errorf("maskDSN(%q) = %q still carries the password", dsn, got)
		}
	}
}

func TestSummarizeConfigMasksSecrets(t *testing.T) {
	cfg := config.Default()
	cfg.App.Name = "myapp"
	cfg.Env = "dev"
	cfg.DB.Driver = "mysql"
	cfg.DB.DSN = "root:" + "pw" + "@tcp(localhost:3306)/app"
	cfg.Redis.Addr = "localhost:6379"
	cfg.Redis.Password = "redis-pw"
	cfg.Mail.SMTP.Host = "smtp.example.com"
	cfg.Mail.SMTP.Port = 587
	cfg.Mail.SMTP.Username = "ada"
	cfg.Mail.SMTP.Password = "smtp-pw"

	s := summarizeConfig(cfg)
	if want := "root:***@tcp(localhost:3306)/app"; s.DB.DSN != want {
		t.Errorf("db.dsn = %q, want %q", s.DB.DSN, want)
	}
	if s.Redis.Password != masked {
		t.Errorf("redis.password = %q, want %q", s.Redis.Password, masked)
	}
	if s.Mail.SMTP.Password != masked {
		t.Errorf("mail.smtp.password = %q, want %q", s.Mail.SMTP.Password, masked)
	}
	// Non-secrets survive.
	if s.DB.Driver != "mysql" || s.Redis.Addr != "localhost:6379" || s.Mail.SMTP.Host != "smtp.example.com" || s.Mail.SMTP.Username != "ada" {
		t.Errorf("non-secret values lost: %+v", s)
	}
	// Empty secrets stay empty (distinguishable from masked ones).
	empty := summarizeConfig(config.Default())
	if empty.DB.DSN != "" || empty.Redis.Password != "" || empty.Mail.SMTP.Password != "" {
		t.Errorf("empty secrets should stay empty: %+v", empty)
	}
}

func TestCollectLogPanelsExtractsSQL(t *testing.T) {
	now := time.Now()
	records := []log.Record{
		{Time: now, Level: slog.LevelDebug, Category: "db", Message: "query", Attrs: []slog.Attr{
			slog.String("sql", "SELECT * FROM post"),
			slog.Float64("elapsed_ms", 1.25),
			slog.Int64("rows", 3),
		}},
		{Time: now, Level: slog.LevelError, Category: "db", Message: "query failed", Attrs: []slog.Attr{
			slog.String("sql", "DELETE FROM post"),
			slog.Float64("elapsed_ms", 0.5),
			slog.Int64("rows", -1),
			slog.String("err", "constraint failed"),
		}},
		// db-category record without a statement (an adapter Info line) —
		// not a query, not on the SQL panel.
		{Time: now, Level: slog.LevelInfo, Category: "db", Message: "connected"},
	}
	_, sqls, _, _ := collectLogPanels(records)
	if len(sqls) != 2 {
		t.Fatalf("extracted %d statements, want 2", len(sqls))
	}
	first := sqls[0]
	if first.SQL != "SELECT * FROM post" || first.DurationMs != 1.25 || first.Rows != 3 || first.Level != "debug" {
		t.Errorf("first statement = %+v", first)
	}
	if sqls[1].Error != "constraint failed" || sqls[1].Level != "error" {
		t.Errorf("failed statement = %+v", sqls[1])
	}
}

func TestCollectLogPanelsPairsTimeline(t *testing.T) {
	now := time.Now()
	records := []log.Record{
		{Time: now, Level: slog.LevelInfo, Category: log.ProfileCategory, Message: "profile begin", Attrs: []slog.Attr{slog.String("token", "render list")}},
		{Time: now.Add(time.Millisecond), Level: slog.LevelInfo, Category: log.ProfileCategory, Message: "profile end", Attrs: []slog.Attr{
			slog.String("token", "render list"), slog.Float64("elapsed_ms", 12.5),
		}},
		// An end without a begin contributes nothing.
		{Time: now, Level: slog.LevelInfo, Category: log.ProfileCategory, Message: "profile end", Attrs: []slog.Attr{
			slog.String("token", "orphan"), slog.Float64("elapsed_ms", 99),
		}},
	}
	_, _, timeline, _ := collectLogPanels(records)
	if len(timeline) != 1 {
		t.Fatalf("timeline has %d items, want 1", len(timeline))
	}
	if timeline[0].Name != "render list" || timeline[0].DurationMs != 12.5 {
		t.Errorf("timeline item = %+v", timeline[0])
	}
}

func TestCollectLogPanelsCollectsMail(t *testing.T) {
	now := time.Now()
	records := []log.Record{
		{Time: now, Level: slog.LevelInfo, Category: "mail", Message: "From: noreply@example.com\r\nSubject: hi\r\n\r\nbody"},
	}
	logs, _, _, mails := collectLogPanels(records)
	if len(mails) != 1 || mails[0].Message != records[0].Message {
		t.Fatalf("mail panel = %+v", mails)
	}
	if len(logs) != 1 || logs[0].Category != "mail" {
		t.Errorf("log panel = %+v", logs)
	}
}
