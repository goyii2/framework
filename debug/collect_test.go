package debug

import (
	"log/slog"
	"testing"
	"time"

	"github.com/goyii2/framework/log"
)

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
