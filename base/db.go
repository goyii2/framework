package base

import (
	"time"

	"github.com/goyii2/framework/db"
	"github.com/goyii2/framework/log"
)

// DB returns the application's database connection — the one DML,
// migrations and the queue's db driver share. The database has no fallback:
// accessing it before the application entry installed a connection is a
// programmer error and panics.
func (a *Application) DB() *db.Conn {
	if a.db == nil {
		panic("goyii: db component not wired: open the connection in main (db.New or ConnectDB) and install it with SetDB")
	}
	return a.db
}

// SetDB installs the database connection (e.g. a pre-opened connection or a
// test fake). nil uninstalls it, putting DB back into the not-wired state.
func (a *Application) SetDB(c *db.Conn) { a.db = c }

// ConnectDB opens the database connection for cfg and installs it as the
// application's db component in one step — the standard assembly call:
//
//	if err := app.ConnectDB(cfg.DB, cfg.Log.Gorm); err != nil { ... }
//
// The connection carries the GORM query-log adapter routing queries, slow
// queries and errors into the application's log component under the db
// category; gorm carries that adapter's display-side values (disable, slow
// threshold) from the project's log configuration, and a disabled adapter
// leaves GORM's own default logger in place. The error is the caller's to
// handle — the application keeps its previous state (usually none) on
// failure.
func (a *Application) ConnectDB(cfg db.Config, gorm log.GormConfig) error {
	var opts []db.Option
	if !gorm.Disable {
		slow := time.Duration(gorm.SlowThresholdMs) * time.Millisecond
		opts = append(opts, db.WithGormLogger(db.NewGormLogger(func() *log.Logger {
			return a.Logger()
		}, slow)))
	}
	conn, err := db.New(cfg, opts...)
	if err != nil {
		return err
	}
	a.db = conn
	return nil
}
