package migrate

import (
	"sort"
	"strings"
)

// dialectRegistry maps every supported dialect name to its adapter. It is
// immutable after package initialization and is the single entry point for
// dialect knowledge, shared by the CLI and the migration generator.
var dialectRegistry = map[string]Dialect{
	"mysql":     MySQLDialect{},
	"sqlite":    SQLiteDialect{},
	"postgres":  PostgreSQLDialect{},
	"sqlserver": SQLServerDialect{},
}

// DialectByName returns the registered dialect for name. The lookup is
// case-insensitive and ignores surrounding whitespace, so " MySQL " resolves
// the same as "mysql". ok is false when no dialect is registered under name.
func DialectByName(name string) (dialect Dialect, ok bool) {
	dialect, ok = dialectRegistry[strings.ToLower(strings.TrimSpace(name))]
	return dialect, ok
}

// DialectNames returns the names of every registered dialect in sorted order.
func DialectNames() []string {
	names := make([]string, 0, len(dialectRegistry))
	for name := range dialectRegistry {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
