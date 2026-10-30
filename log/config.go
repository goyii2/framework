package log

import (
	"fmt"
	"log/slog"
	"strings"
)

// Config carries the log component values. The zero value yields a logger at
// info level writing to one console target on stdout.
type Config struct {
	Level   string         `yaml:"level"`   // debug | info | warn | error; empty = info
	Targets []TargetConfig `yaml:"targets"` // empty = one console target on stdout
	Gorm    GormConfig     `yaml:"gorm"`    // values of the db query logging adapter
}

// TargetConfig configures one built-in target. Type selects the sink
// (console | file); the remaining values are delivery rules the dispatcher
// applies before the sink sees a record.
type TargetConfig struct {
	Type       string   `yaml:"type"`       // console | file
	MinLevel   string   `yaml:"minLevel"`   // debug | info | warn | error
	Categories []string `yaml:"categories"` // include patterns ("web", "db*", "*"); empty = all
	Except     []string `yaml:"except"`     // exclude patterns; win over categories

	// File target values.
	Path      string `yaml:"path"`      // log file path
	MaxSizeMB int    `yaml:"maxSizeMB"` // rotate when the file exceeds this size
	MaxFiles  int    `yaml:"maxFiles"`  // rotated files kept on disk
}

// GormConfig carries the values of the database query logging adapter, which
// routes GORM's logger callbacks into the log component; the adapter is
// built where the database is wired, the values travel with the log
// configuration because that is where applications set them.
type GormConfig struct {
	Disable         bool `yaml:"disable"`         // true turns the adapter off (GORM default stdout logger)
	SlowThresholdMs int  `yaml:"slowThresholdMs"` // queries at or above this duration log as slow
}

// New builds a logger from the Config: each configured target is constructed,
// bound to its delivery rules, and appended; with no targets configured a
// single console target on stdout is installed. An error is returned when a
// target cannot be built (unknown type, unwritable path), listing every
// failure; no logger is returned then, callers decide on a fallback.
func New(cfg Config) (*Logger, error) {
	l := &Logger{level: ParseLevel(cfg.Level), category: DefaultCategory}

	if len(cfg.Targets) == 0 {
		l.bindings = append(l.bindings, binding{target: NewConsoleTarget(nil), filter: Filter{MinLevel: slog.LevelDebug}})
		return l, nil
	}

	var errs []string
	for _, tc := range cfg.Targets {
		t, err := buildTarget(tc)
		if err != nil {
			errs = append(errs, err.Error())
			continue
		}
		// An unset minLevel adds no floor of its own; the logger level is
		// the only gate then.
		filter := Filter{MinLevel: slog.LevelDebug, Includes: tc.Categories, Excludes: tc.Except}
		if strings.TrimSpace(tc.MinLevel) != "" {
			filter.MinLevel = ParseLevel(tc.MinLevel)
		}
		l.bindings = append(l.bindings, binding{target: t, filter: filter})
	}
	if len(errs) > 0 {
		return nil, fmt.Errorf("log: %s", strings.Join(errs, "; "))
	}
	return l, nil
}

// buildTarget constructs the built-in target the TargetConfig names.
func buildTarget(tc TargetConfig) (Target, error) {
	switch strings.ToLower(strings.TrimSpace(tc.Type)) {
	case "console", "":
		return NewConsoleTarget(nil), nil
	case "file":
		maxSize := int64(tc.MaxSizeMB) * 1024 * 1024
		if maxSize <= 0 {
			maxSize = 10 * 1024 * 1024
		}
		maxFiles := tc.MaxFiles
		if maxFiles <= 0 {
			maxFiles = 5
		}
		return NewFileTarget(tc.Path, maxSize, maxFiles)
	default:
		return nil, fmt.Errorf("log target type %q is unknown (supported: console, file)", tc.Type)
	}
}
