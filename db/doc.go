// Package db provides database connections and the generic Active Record
// data access layer; it corresponds to Yii2's yii\db subsystem.
//
// The connection component wraps GORM for DML access. The DSN comes from the
// application configuration's db section, with the DB_DSN environment
// variable taking precedence when set. Versioned DDL (migrations) lives in
// the migrate package and opens its own database/sql connection from the same
// DSN; the two layers share configuration but not handles.
//
// The generic Active Record layer built on the connection lives in the
// db/ar subpackage.
package db
