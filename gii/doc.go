// Package gii is the code generator: it reads a table's structure through
// GORM's migrator and renders Go sources from templates — Active Record
// models, controller skeletons and full CRUD sets with templ views. The
// templates are embedded and overridable file by file; generation is pure
// flags, no interactive wizard. The gii/commands subpackage attaches the
// generators below the goyii console root command.
package gii
