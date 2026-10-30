// Package debug provides the development-time request introspection tool:
// a middleware that aggregates what one request did (routing, logs, SQL,
// timeline, session, mail) into a per-request JSON record on
// disk, and embedded /debug pages to browse that history — plus a toolbar
// injected into successful HTML responses linking each page to its own
// record. Everything mounts only when the application runs with debug on.
package debug

import (
	"log/slog"
	"strconv"
	"time"

	"github.com/goyii2/framework/db"
	"github.com/goyii2/framework/log"
	"github.com/goyii2/framework/web"
)

// Entry is one request's collected data: the six panels the detail page
// shows, written as a single JSON file per request.
type Entry struct {
	ID       string         `json:"id"`
	Time     time.Time      `json:"time"`
	Request  RequestInfo    `json:"request"`
	Logs     []LogLine      `json:"logs"`
	SQL      []SQLInfo      `json:"sql"`
	Timeline []TimelineItem `json:"timeline"`
	Session  *SessionInfo   `json:"session,omitempty"`
	User     *UserInfo      `json:"user,omitempty"`
	Mail     []MailInfo     `json:"mail"`
}

// RequestInfo is the request/routing panel: what came in, what it matched,
// what went out.
type RequestInfo struct {
	Method     string            `json:"method"`
	Path       string            `json:"path"`
	Query      string            `json:"query"`
	RemoteAddr string            `json:"remoteAddr"`
	Host       string            `json:"host"`
	Proto      string            `json:"proto"`
	Route      string            `json:"route"`
	Params     map[string]string `json:"params,omitempty"`
	Status     int               `json:"status"`
	DurationMs float64           `json:"durationMs"`
	Error      string            `json:"error,omitempty"`
}

// LogLine is one log record as the log panel shows it.
type LogLine struct {
	Time     time.Time      `json:"time"`
	Level    string         `json:"level"`
	Category string         `json:"category"`
	Message  string         `json:"message"`
	Attrs    map[string]any `json:"attrs,omitempty"`
}

// SQLInfo is one executed statement extracted from the db log category.
type SQLInfo struct {
	SQL        string  `json:"sql"`
	DurationMs float64 `json:"durationMs"`
	Rows       int64   `json:"rows"`
	Level      string  `json:"level"`
	Message    string  `json:"message"`
	Error      string  `json:"error,omitempty"`
}

// TimelineItem is one profiling interval reassembled from the profiler's
// begin/end record pair.
type TimelineItem struct {
	Name       string  `json:"name"`
	DurationMs float64 `json:"durationMs"`
}

// SessionInfo is the session panel: the request-scoped session's id and
// values at the end of the request.
type SessionInfo struct {
	ID     string         `json:"id"`
	Values map[string]any `json:"values"`
}

// UserInfo is the user panel: the authenticated state the request ended
// with, or a guest marker.
type UserInfo struct {
	Guest bool     `json:"guest"`
	ID    string   `json:"id,omitempty"`
	Name  string   `json:"name,omitempty"`
	Roles []string `json:"roles,omitempty"`
}

// MailInfo is one message recorded by the log mail driver.
type MailInfo struct {
	Time    time.Time `json:"time"`
	Message string    `json:"message"`
}

// collectLogPanels folds the request's captured log records into the log,
// SQL, timeline and mail panels. SQL comes from the db-category records the
// GORM adapter emits (sql/elapsed_ms/rows attributes); the timeline pairs
// the profiler's begin/end records by token; mail records carry the fully
// rendered message the log driver recorded.
func collectLogPanels(records []log.Record) (logs []LogLine, sqls []SQLInfo, timeline []TimelineItem, mails []MailInfo) {
	begins := map[string]bool{}
	for _, rec := range records {
		line := LogLine{
			Time:     rec.Time,
			Level:    levelLabel(rec.Level),
			Category: rec.Category,
			Message:  rec.Message,
		}
		if len(rec.Attrs) > 0 {
			line.Attrs = make(map[string]any, len(rec.Attrs))
			for _, a := range rec.Attrs {
				line.Attrs[a.Key] = a.Value.Any()
			}
		}
		logs = append(logs, line)

		switch {
		case rec.Category == db.GormCategory && hasAttr(rec, "sql"):
			sqls = append(sqls, sqlFromRecord(rec, line.Attrs))
		case rec.Category == log.ProfileCategory:
			switch rec.Message {
			case "profile begin":
				begins[attrString(rec, "token")] = true
			case "profile end":
				token := attrString(rec, "token")
				if begins[token] {
					delete(begins, token)
					timeline = append(timeline, TimelineItem{
						Name:       token,
						DurationMs: attrFloat(rec, "elapsed_ms"),
					})
				}
			}
		case rec.Category == "mail":
			mails = append(mails, MailInfo{Time: rec.Time, Message: rec.Message})
		}
	}
	return logs, sqls, timeline, mails
}

// sqlFromRecord rebuilds one executed statement from a db-category record.
func sqlFromRecord(rec log.Record, attrs map[string]any) SQLInfo {
	info := SQLInfo{
		Level:   levelLabel(rec.Level),
		Message: rec.Message,
		SQL:     attrString(rec, "sql"),
		Error:   attrString(rec, "err"),
	}
	if v, ok := attrs["elapsed_ms"].(float64); ok {
		info.DurationMs = v
	}
	if v, ok := attrs["rows"].(int64); ok {
		info.Rows = v
	}
	return info
}

// collectSessionUser snapshots the request-scoped session and user after the
// action ran; nil panels when the session middleware is not in the chain.
func collectSessionUser(c *web.Context) (*SessionInfo, *UserInfo) {
	if c == nil || c.Session() == nil {
		return nil, nil
	}
	sess := &SessionInfo{ID: c.Session().ID(), Values: c.Session().All()}
	user := &UserInfo{}
	if u := c.User(); u != nil && !u.IsGuest() {
		id := u.Identity()
		user.ID, user.Name, user.Roles = id.GetID(), id.GetName(), id.GetRoles()
	} else {
		user.Guest = true
	}
	return sess, user
}

// levelLabel renders a slog level the way the panels display it.
func levelLabel(level slog.Level) string {
	switch {
	case level < slog.LevelInfo:
		return "debug"
	case level < slog.LevelWarn:
		return "info"
	case level < slog.LevelError:
		return "warn"
	default:
		return "error"
	}
}

// hasAttr reports whether rec carries an attribute with key.
func hasAttr(rec log.Record, key string) bool {
	for _, a := range rec.Attrs {
		if a.Key == key {
			return true
		}
	}
	return false
}

// attrString returns the attribute value as a string, "" when absent or not
// a string.
func attrString(rec log.Record, key string) string {
	for _, a := range rec.Attrs {
		if a.Key == key {
			s, _ := a.Value.Any().(string)
			return s
		}
	}
	return ""
}

// attrFloat returns the attribute value as a float, 0 when absent or not a
// number.
func attrFloat(rec log.Record, key string) float64 {
	for _, a := range rec.Attrs {
		if a.Key == key {
			f, _ := a.Value.Any().(float64)
			return f
		}
	}
	return 0
}

// formatDurationMs renders fractional milliseconds for page display.
func formatDurationMs(ms float64) string {
	return strconv.FormatFloat(ms, 'f', 1, 64)
}
