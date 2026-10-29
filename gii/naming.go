package gii

import (
	"fmt"
	"strings"
	"unicode"
)

// initialisms lists the short words column and table names commonly carry
// that Go spells in all caps in exported identifiers.
var initialisms = map[string]string{
	"id":   "ID",
	"url":  "URL",
	"api":  "API",
	"uuid": "UUID",
	"html": "HTML",
	"sql":  "SQL",
	"json": "JSON",
	"http": "HTTP",
	"ip":   "IP",
	"css":  "CSS",
}

// exportedName converts a snake_case identifier to an exported CamelCase
// one: "post" -> "Post", "author_id" -> "AuthorID". Unknown words are
// capitalized plainly; the initialism table keeps the common short ones in
// Go's conventional spelling.
func exportedName(snake string) string {
	parts := strings.Split(snake, "_")
	for i, part := range parts {
		if part == "" {
			continue
		}
		if fixed, ok := initialisms[strings.ToLower(part)]; ok {
			parts[i] = fixed
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + part[1:]
	}
	return strings.Join(parts, "")
}

// lowerName is exportedName's unexported twin: "author_id" -> "authorID".
func lowerName(snake string) string {
	name := exportedName(snake)
	if name == "" {
		return name
	}
	return strings.ToLower(name[:1]) + name[1:]
}

// label turns a snake_case column name into a human heading: "author_id"
// -> "Author ID".
func label(snake string) string {
	parts := strings.Split(snake, "_")
	for i, part := range parts {
		if part == "" {
			continue
		}
		if fixed, ok := initialisms[strings.ToLower(part)]; ok {
			parts[i] = fixed
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + part[1:]
	}
	return strings.Join(parts, " ")
}

// isValidIdentifier reports whether s is a usable Go identifier: letters
// and digits, not starting with a digit. It backs the controller id and
// package name validation — generated code embeds both.
func isValidIdentifier(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		switch {
		case r == '_' || unicode.IsLetter(r):
		case i > 0 && unicode.IsDigit(r):
		default:
			return false
		}
	}
	return true
}

// validateControllerID checks an --id value: a plain identifier, with dash
// and underscore tolerated as word separators the way controller ids are
// written ("admin-user" -> AdminUser).
func validateControllerID(id string) error {
	if id == "" {
		return fmt.Errorf("gii: controller id is required (--id)")
	}
	for _, r := range id {
		switch {
		case r == '-' || r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r):
		default:
			return fmt.Errorf("gii: controller id %q may contain letters, digits, dash and underscore only", id)
		}
	}
	return nil
}

// controllerName resolves the names a controller id implies: the struct
// ("post" -> "PostController"), its receiver ("pc") and the route prefix.
func controllerName(id string) (structName, receiver string) {
	base := exportedName(strings.NewReplacer("-", "_").Replace(id))
	if base == "" {
		return "Controller", "c"
	}
	return base + "Controller", strings.ToLower(base[:1]) + "c"
}
