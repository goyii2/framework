package debug

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
)

// sortedKeys returns the map's keys sorted, for deterministic page rendering.
func sortedKeys(m map[string]any) []string {
	return slices.Sorted(maps.Keys(m))
}

// formatValue renders a session value for the page.
func formatValue(v any) string {
	switch v := v.(type) {
	case string:
		return v
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", v)
	}
}

// formatParams renders match parameters as "name=value" pairs.
func formatParams(m map[string]string) string {
	parts := make([]string, 0, len(m))
	for _, k := range slices.Sorted(maps.Keys(m)) {
		parts = append(parts, k+"="+m[k])
	}
	return strings.Join(parts, " ")
}

// formatAttrs renders log attributes as "key=value" pairs.
func formatAttrs(m map[string]any) string {
	parts := make([]string, 0, len(m))
	for _, k := range sortedKeys(m) {
		parts = append(parts, k+"="+formatValue(m[k]))
	}
	return strings.Join(parts, " ")
}

// configRows flattens the configuration summary into display rows, secrets
// already masked by summarizeConfig.
func configRows(c ConfigSummary) [][2]string {
	rows := [][2]string{
		{"app.name", c.App.Name},
		{"app.env", c.App.Env},
		{"app.language", c.App.Language},
		{"app.debug", strconv.FormatBool(c.App.Debug)},
		{"web.listen", c.Web.Listen},
		{"db.driver", c.DB.Driver},
		{"db.dsn", c.DB.DSN},
		{"redis.addr", c.Redis.Addr},
		{"redis.db", strconv.Itoa(c.Redis.DB)},
		{"redis.password", c.Redis.Password},
		{"cache.driver", c.Cache.Driver},
		{"session.backend", c.Session.Backend},
		{"queue.driver", c.Queue.Driver},
		{"log.level", c.Log.Level},
		{"log.targets", strings.Join(c.Log.Targets, ", ")},
		{"mail.driver", c.Mail.Driver},
		{"mail.from", c.Mail.From},
		{"mail.smtp.host", c.Mail.SMTP.Host},
		{"mail.smtp.port", strconv.Itoa(c.Mail.SMTP.Port)},
		{"mail.smtp.username", c.Mail.SMTP.Username},
		{"mail.smtp.password", c.Mail.SMTP.Password},
	}
	return rows
}
