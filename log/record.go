package log

import (
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"
)

// Record is one log entry as delivered to targets: the timestamp, level,
// category, message and structured attributes. Attrs carries the key/value
// pairs passed by the caller (mirroring slog's attribute model: odd trailing
// args are collected under the key "!BADKEY").
type Record struct {
	Time     time.Time
	Level    slog.Level
	Category string
	Message  string
	Attrs    []slog.Attr
}

// Target is the sink contract: every configured destination implements it
// (console, file, or anything injected programmatically). Write must be
// safe for concurrent use; the dispatcher applies level and category rules
// before a record reaches the target, so sinks receive only what their
// filter accepted.
type Target interface {
	// Name identifies the target in diagnostics.
	Name() string

	// Write appends one record; implementations own the formatting.
	Write(rec Record) error

	// Close releases the target's resources (file handles etc.).
	Close() error
}

// Filter holds the delivery rules a target is bound with. MinLevel drops
// records below it (the zero value floors at info, mirroring slog's
// HandlerOptions — pass slog.LevelDebug to disable the floor); Includes
// keeps only matching categories (empty = all); Excludes drops matching
// categories and wins over Includes. Patterns match exactly, or by prefix
// when they end with "*" ("db*" hits "db" and "db.query"); "*" alone
// matches everything.
type Filter struct {
	MinLevel slog.Level
	Includes []string
	Excludes []string
}

// Accept reports whether a record with the given category and level passes
// the filter.
func (f Filter) Accept(category string, level slog.Level) bool {
	if level < f.MinLevel {
		return false
	}
	if matchesAnyPattern(f.Excludes, category) {
		return false
	}
	if len(f.Includes) == 0 || matchesAnyPattern(f.Includes, category) {
		return true
	}
	return false
}

// matchesAnyPattern reports whether category matches any pattern.
func matchesAnyPattern(patterns []string, category string) bool {
	for _, p := range patterns {
		if matchPattern(p, category) {
			return true
		}
	}
	return false
}

// matchPattern matches a category against one pattern: "*" matches all,
// a trailing "*" matches by prefix, anything else matches exactly.
func matchPattern(pattern, category string) bool {
	if pattern == "*" {
		return true
	}
	if strings.HasSuffix(pattern, "*") {
		return strings.HasPrefix(category, pattern[:len(pattern)-1])
	}
	return pattern == category
}

// formatRecord renders one record as a single text line shared by the
// built-in targets:
//
//	2026-10-28T15:04:05.123+08:00 INFO [application] message key=value n=3
func formatRecord(rec Record) string {
	var b strings.Builder
	b.WriteString(rec.Time.Format(time.RFC3339Nano))
	b.WriteByte(' ')
	b.WriteString(levelName(rec.Level))
	b.WriteString(" [")
	b.WriteString(rec.Category)
	b.WriteString("] ")
	b.WriteString(rec.Message)
	for _, attr := range rec.Attrs {
		b.WriteByte(' ')
		b.WriteString(attr.Key)
		b.WriteByte('=')
		b.WriteString(formatValue(attr.Value.Any()))
	}
	return b.String()
}

// levelName renders a slog level the way slog's text handler does.
func levelName(level slog.Level) string {
	switch {
	case level < slog.LevelInfo:
		return "DEBUG"
	case level < slog.LevelWarn:
		return "INFO"
	case level < slog.LevelError:
		return "WARN"
	default:
		return "ERROR"
	}
}

// formatValue renders an attribute value, quoting strings that would
// otherwise break the key=value line format.
func formatValue(v any) string {
	if s, ok := v.(string); ok {
		if strings.ContainsAny(s, " \t\"=") {
			return strconv.Quote(s)
		}
		return s
	}
	return fmt.Sprintf("%v", v)
}
