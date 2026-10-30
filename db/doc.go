// Package db provides database connections and the generic Active Record
// data access layer; it corresponds to Yii2's yii\db subsystem.
//
// The connection component wraps GORM for DML access. New opens it from
// Config — a project-owned struct whose values may come from any
// configuration source — with the DB_DSN environment variable taking
// precedence over the configured DSN when set. Versioned DDL (migrations)
// lives in the migrate package and opens its own database/sql connection
// from the same DSN; the two layers share configuration but not handles.
// Component is the optional lazy holder for applications that want lazy or
// retryable open semantics instead of eager assembly.
//
// The generic Active Record layer built on the connection lives in the
// db/ar subpackage.
package db
