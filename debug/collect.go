// Package debug provides the development-time request introspection tool:
// a middleware that aggregates what one request did (routing, logs, SQL,
// timeline, session, configuration, mail) into a per-request JSON record on
// disk, and embedded /debug pages to browse that history — plus a toolbar
// injected into successful HTML responses linking each page to its own
// record. Everything mounts only when the application runs with debug on.
package debug

import (
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/goyii2/framework/config"
	"github.com/goyii2/framework/db"
	"github.com/goyii2/framework/log"
	"github.com/goyii2/framework/web"
)

// Entry is one request's collected data: the seven panels the detail page
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
	Config   ConfigSummary  `json:"config"`
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

// masked is what every secret renders as; an empty secret stays empty so a
// missing value is distinguishable from a set one.
const masked = "***"

// ConfigSummary is the configuration panel: the component choices the
// application runs with, secrets masked (DSN and stored passwords never
// reach the page).
type ConfigSummary struct {
	App struct {
		Name     string `json:"name"`
		Env      string `json:"env"`
		Language string `json:"language"`
		Debug    bool   `json:"debug"`
	} `json:"app"`
	Web struct {
		Listen string `json:"listen"`
	} `json:"web"`
	DB struct {
		Driver string `json:"driver"`
		DSN    string `json:"dsn"`
	} `json:"db"`
	Redis struct {
		Addr     string `json:"addr"`
		DB       int    `json:"db"`
		Password string `json:"password"`
	} `json:"redis"`
	Cache struct {
		Driver string `json:"driver"`
	} `json:"cache"`
	Session struct {
		Backend string `json:"backend"`
	} `json:"session"`
	Queue struct {
		Driver string `json:"driver"`
	} `json:"queue"`
	Log struct {
		Level   string   `json:"level"`
		Targets []string `json:"targets"`
	} `json:"log"`
	Mail struct {
		Driver string `json:"driver"`
		From   string `json:"from"`
		SMTP   struct {
			Host     string `json:"host"`
			Port     int    `json:"port"`
			Username string `json:"username"`
			Password string `json:"password"`
		} `json:"smtp"`
	} `json:"mail"`
}

// summarizeConfig builds the configuration panel from the application
// configuration, masking every secret: the DSN's credentials and the redis
// and SMTP passwords.
func summarizeConfig(cfg config.Config) ConfigSummary {
	var s ConfigSummary
	s.App.Name, s.App.Env, s.App.Language, s.App.Debug = cfg.App.Name, cfg.Env, cfg.App.Language, cfg.App.Debug
	s.Web.Listen = cfg.Web.Listen
	s.DB.Driver, s.DB.DSN = cfg.DB.Driver, maskDSN(cfg.DB.DSN)
	s.Redis.Addr, s.Redis.DB = cfg.Redis.Addr, cfg.Redis.DB
	s.Redis.Password = maskValue(cfg.Redis.Password)
	s.Cache.Driver = cfg.Cache.Driver
	s.Session.Backend = cfg.Session.Backend
	s.Queue.Driver = cfg.Queue.Driver
	s.Log.Level = cfg.Log.Level
	for _, t := range cfg.Log.Targets {
		s.Log.Targets = append(s.Log.Targets, t.Type)
	}
	s.Mail.Driver, s.Mail.From = cfg.Mail.Driver, cfg.Mail.From
	s.Mail.SMTP.Host, s.Mail.SMTP.Port, s.Mail.SMTP.Username = cfg.Mail.SMTP.Host, cfg.Mail.SMTP.Port, cfg.Mail.SMTP.Username
	s.Mail.SMTP.Password = maskValue(cfg.Mail.SMTP.Password)
	return s
}

// maskValue renders a stored secret: set values become the mask, empty ones
// stay empty.
func maskValue(v string) string {
	if v == "" {
		return ""
	}
	return masked
}

// maskDSN hides a DSN's credentials while keeping its shape: URL-style DSNs
// (postgres://user:pass@host/db), MySQL-style DSNs (user:pass@tcp(host)/db)
// and key=value DSNs (SQL Server: "user id=sa;password=secret;server=host")
// all keep everything but the password.
func maskDSN(dsn string) string {
	if dsn == "" {
		return ""
	}
	// key=value style first: mask the password pair's value up to the next
	// ";" (or the end).
	if i := kvPasswordValueStart(dsn); i >= 0 {
		return dsn[:i] + masked + kvRest(dsn[i:])
	}
	// Credential style: a username (optionally ":password") sits between the
	// scheme separator and the first "@". Everything between the colon and
	// the "@" is the password.
	if at := strings.IndexByte(dsn, '@'); at > 0 {
		head := dsn[:at]
		credsStart := strings.LastIndexByte(head, '/') + 1
		if colon := strings.IndexByte(head[credsStart:], ':'); colon >= 0 {
			colon += credsStart
			return dsn[:colon+1] + masked + dsn[at:]
		}
	}
	return dsn
}

// kvPasswordValueStart finds where a "password=..." pair's value starts in a
// key=value DSN, or -1 when there is none.
func kvPasswordValueStart(dsn string) int {
	lower := strings.ToLower(dsn)
	offset := 0
	for {
		i := strings.Index(lower[offset:], "password")
		if i < 0 {
			return -1
		}
		i += offset
		after := dsn[i+len("password"):]
		j := 0
		for j < len(after) && (after[j] == ' ' || after[j] == '\t') {
			j++
		}
		if j < len(after) && after[j] == '=' {
			k := j + 1
			for k < len(after) && (after[k] == ' ' || after[k] == '\t') {
				k++
			}
			return i + len("password") + k
		}
		// "password" was part of another key; keep scanning past it.
		offset = i + len("password")
	}
}

// kvRest returns the value run's remainder — everything from the next ";"
// on, or nothing when the pair ends the DSN.
func kvRest(fromValueStart string) string {
	if i := strings.IndexByte(fromValueStart, ';'); i >= 0 {
		return fromValueStart[i:]
	}
	return ""
}

// formatDurationMs renders fractional milliseconds for page display.
func formatDurationMs(ms float64) string {
	return strconv.FormatFloat(ms, 'f', 1, 64)
}
