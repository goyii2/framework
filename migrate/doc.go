// Package migrate provides versioned database migrations (the migration
// commands and history table); it corresponds to Yii2's MigrateController
// subsystem.
//
// The migration engine was merged from github.com/phgors/yiimigrate (by the
// same maintainer as this framework) and keeps its original architecture:
// raw database/sql connections for DDL (no ORM), four dialects (MySQL,
// PostgreSQL, SQL Server, SQLite), the "migration" history table, per-dialect
// advisory locks and dry-run.
package migrate
