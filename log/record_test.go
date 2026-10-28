package log

import (
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestMatchPattern(t *testing.T) {
	cases := []struct {
		pattern  string
		category string
		want     bool
	}{
		{"db", "db", true},
		{"db", "db.query", false},
		{"db*", "db", true},
		{"db*", "db.query", true},
		{"db*", "web", false},
		{"*", "anything", true},
		{"*", "", true},
		{"", "", true},
		{"", "db", false},
	}
	for _, tc := range cases {
		if got := matchPattern(tc.pattern, tc.category); got != tc.want {
			t.Errorf("matchPattern(%q, %q) = %v, want %v", tc.pattern, tc.category, got, tc.want)
		}
	}
}

func TestFilterAcceptLevelFloor(t *testing.T) {
	f := Filter{MinLevel: slog.LevelWarn}
	for _, level := range []slog.Level{slog.LevelDebug, slog.LevelInfo} {
		if f.Accept("db", level) {
			t.Errorf("level %v below MinLevel must be rejected", level)
		}
	}
	for _, level := range []slog.Level{slog.LevelWarn, slog.LevelError} {
		if !f.Accept("db", level) {
			t.Errorf("level %v at or above MinLevel must be accepted", level)
		}
	}
}

func TestFilterAcceptIncludeRules(t *testing.T) {
	f := Filter{Includes: []string{"web", "db*"}}

	for _, category := range []string{"web", "db", "db.query"} {
		if !f.Accept(category, slog.LevelInfo) {
			t.Errorf("category %q matches an include pattern and must be accepted", category)
		}
	}
	for _, category := range []string{"web/session", "cache", "application"} {
		if f.Accept(category, slog.LevelInfo) {
			t.Errorf("category %q matches no include pattern and must be rejected", category)
		}
	}

	// Empty includes accept every category; the floor-free filter lets even
	// debug records through.
	all := Filter{MinLevel: slog.LevelDebug}
	if !all.Accept("anything", slog.LevelDebug) {
		t.Error("empty Includes must accept every category")
	}

	// The zero filter floors at info, mirroring slog's HandlerOptions.
	zero := Filter{}
	if zero.Accept("anything", slog.LevelDebug) {
		t.Error("zero filter must floor at info (debug rejected)")
	}
	if !zero.Accept("anything", slog.LevelInfo) {
		t.Error("zero filter must accept info records")
	}
}

func TestFilterAcceptExcludeWinsOverInclude(t *testing.T) {
	f := Filter{Includes: []string{"db*"}, Excludes: []string{"db.slow"}}

	if !f.Accept("db.query", slog.LevelInfo) {
		t.Error("db.query matches the include and no exclude; must be accepted")
	}
	if f.Accept("db.slow", slog.LevelInfo) {
		t.Error("db.slow matches the exclude; must be rejected even though the include also matches")
	}

	// A bare "*" include with an exclude still drops the excluded category.
	star := Filter{Includes: []string{"*"}, Excludes: []string{"noisy"}}
	if star.Accept("noisy", slog.LevelInfo) {
		t.Error("excluded category must be rejected even with the * include")
	}
	if !star.Accept("quiet", slog.LevelInfo) {
		t.Error("non-excluded category must pass the * include")
	}
}

func TestFormatRecord(t *testing.T) {
	rec := Record{
		Time:     time.Date(2026, 10, 28, 15, 4, 5, 123000000, time.UTC),
		Level:    slog.LevelWarn,
		Category: "db.query",
		Message:  "slow query",
		Attrs: []slog.Attr{
			slog.Any("elapsed_ms", 12.5),
			slog.Any("sql", "SELECT 1"),
			slog.Any("note", "two words"),
		},
	}
	got := formatRecord(rec)
	want := `2026-10-28T15:04:05.123Z WARN [db.query] slow query elapsed_ms=12.5 sql="SELECT 1" note="two words"`
	if got != want {
		t.Errorf("formatRecord() =\n  %s\nwant\n  %s", got, want)
	}
}

func TestFormatRecordLevels(t *testing.T) {
	for _, tc := range []struct {
		level slog.Level
		want  string
	}{
		{slog.LevelDebug, "DEBUG"},
		{slog.LevelInfo, "INFO"},
		{slog.LevelWarn, "WARN"},
		{slog.LevelError, "ERROR"},
	} {
		got := formatRecord(Record{Time: time.Now(), Level: tc.level, Message: "m"})
		if !strings.Contains(got, tc.want) {
			t.Errorf("level %v formatted as %q, want it to contain %q", tc.level, got, tc.want)
		}
	}
}
