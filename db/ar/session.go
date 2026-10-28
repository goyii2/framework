package ar

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"github.com/goyii2/framework/base"
	"github.com/goyii2/framework/db"
)

// txKey is the unexported context key under which Transaction carries the
// in-flight transaction handle; queries and instance persistence inside the
// closure resolve to it.
type txKey struct{}

// connKey is the unexported context key under which UseConn carries a
// connection that overrides the application's db component.
type connKey struct{}

// UseConn returns a copy of ctx carrying conn as the database handle for all
// Active Record operations. It bypasses the application's db component — the
// way standalone tools that already hold a connection use this layer without
// assembling an application.
func UseConn(ctx context.Context, conn *db.Conn) context.Context {
	return context.WithValue(ctx, connKey{}, conn)
}

// session returns a clean GORM handle for ctx, resolving in order: the
// transaction Transaction put there, the connection UseConn put there, the
// application's db component. NewDB gives each call a fresh statement, so
// operations sharing one handle never leak conditions into each other; the
// context travels with the statement for cancellation and for hooks.
func session(ctx context.Context) (*gorm.DB, error) {
	var handle *gorm.DB
	if tx, ok := ctx.Value(txKey{}).(*gorm.DB); ok {
		handle = tx
	} else if conn, ok := ctx.Value(connKey{}).(*db.Conn); ok {
		handle = conn.DB
	} else {
		conn, err := base.FromContext(ctx).DB()
		if err != nil {
			return nil, fmt.Errorf("ar: resolve database connection: %w", err)
		}
		handle = conn.DB
	}
	return handle.Session(&gorm.Session{NewDB: true, Context: ctx}), nil
}

// Transaction runs fn inside a database transaction. fn receives a context
// carrying the transaction, so every query, created record and loaded
// instance inside the closure participates in it. A nil return commits; any
// error rolls the whole unit of work back.
//
// With a context that already carries a transaction, fn runs as a nested
// unit on the same connection: GORM brackets it with a savepoint, so an
// inner rollback discards only the inner work and the outer transaction may
// continue.
func Transaction(ctx context.Context, fn func(ctx context.Context) error) error {
	handle, err := session(ctx)
	if err != nil {
		return err
	}
	return handle.Transaction(func(tx *gorm.DB) error {
		return fn(context.WithValue(ctx, txKey{}, tx))
	})
}
