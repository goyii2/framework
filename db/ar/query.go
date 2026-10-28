package ar

import (
	"context"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

// Query is the chainable query builder for one model type — the counterpart
// of Yii2's ActiveQuery. Construct it with New; the vocabulary mirrors Yii2
// (Where, OrderBy, Limit, Offset, With, Join) and every chain method returns
// a new Query, leaving the receiver untouched. All, One and Count execute.
type Query[T any] struct {
	ctx  context.Context
	mods []func(*gorm.DB) *gorm.DB
}

// New starts a query for model T against the database resolved from ctx.
func New[T any](ctx context.Context) *Query[T] {
	return &Query[T]{ctx: ctx}
}

// extend returns a copy of q with mod appended to its statement pipeline.
func (q *Query[T]) extend(mod func(*gorm.DB) *gorm.DB) *Query[T] {
	mods := make([]func(*gorm.DB) *gorm.DB, len(q.mods)+1)
	copy(mods, q.mods)
	mods[len(q.mods)] = mod
	return &Query[T]{ctx: q.ctx, mods: mods}
}

// Where adds a search condition. It accepts what GORM accepts: a SQL
// fragment with ? placeholders plus its arguments, or a struct or map
// stating equality on the matching columns (Yii2's hash format).
func (q *Query[T]) Where(condition any, args ...any) *Query[T] {
	return q.extend(func(tx *gorm.DB) *gorm.DB { return tx.Where(condition, args...) })
}

// OrderBy sorts the results; expressions are joined in order, e.g.
// OrderBy("id DESC", "name").
func (q *Query[T]) OrderBy(expressions ...string) *Query[T] {
	return q.extend(func(tx *gorm.DB) *gorm.DB { return tx.Order(strings.Join(expressions, ", ")) })
}

// Limit caps the result at n records.
func (q *Query[T]) Limit(n int) *Query[T] {
	return q.extend(func(tx *gorm.DB) *gorm.DB { return tx.Limit(n) })
}

// Offset skips the first n records of the result.
func (q *Query[T]) Offset(n int) *Query[T] {
	return q.extend(func(tx *gorm.DB) *gorm.DB { return tx.Offset(n) })
}

// With eager-loads the named associations before the records are returned,
// e.g. With("Posts"); nested associations use dot notation ("Posts.Tags").
func (q *Query[T]) With(relations ...string) *Query[T] {
	return q.extend(func(tx *gorm.DB) *gorm.DB {
		for _, relation := range relations {
			tx = tx.Preload(relation)
		}
		return tx
	})
}

// Join adds a JOIN clause including its condition, e.g.
// Join("LEFT JOIN posts ON posts.user_id = users.id").
func (q *Query[T]) Join(clause string, args ...any) *Query[T] {
	return q.extend(func(tx *gorm.DB) *gorm.DB { return tx.Joins(clause, args...) })
}

// statement resolves the database handle and applies the accumulated
// pipeline to it.
func (q *Query[T]) statement() (*gorm.DB, error) {
	handle, err := session(q.ctx)
	if err != nil {
		return nil, err
	}
	tx := handle
	for _, mod := range q.mods {
		tx = mod(tx)
	}
	return tx, nil
}

// New returns a pointer to a zero-valued model bound to the query's context
// — the Active Record way of constructing a record to insert. Filling its
// fields and calling Save writes the row. For a type without an embedded
// Model the record comes back detached (no persistence methods).
func (q *Query[T]) New() *T {
	record := new(T)
	bindModels(q.ctx, record)
	return record
}

// All executes the query and returns every matching record, each bound to
// the query's context.
func (q *Query[T]) All() ([]*T, error) {
	tx, err := q.statement()
	if err != nil {
		return nil, err
	}
	var records []*T
	if err := tx.Find(&records).Error; err != nil {
		return nil, fmt.Errorf("ar: find %T: %w", *new(T), err)
	}
	bindModels(q.ctx, records...)
	return records, nil
}

// One executes the query and returns the first matching record; when nothing
// matches, the returned error wraps gorm.ErrRecordNotFound.
func (q *Query[T]) One() (*T, error) {
	tx, err := q.statement()
	if err != nil {
		return nil, err
	}
	record := new(T)
	if err := tx.Take(record).Error; err != nil {
		return nil, fmt.Errorf("ar: find one %T: %w", record, err)
	}
	bindModels(q.ctx, record)
	return record, nil
}

// Count executes the query as a count of matching records, ignoring Limit
// and Offset.
func (q *Query[T]) Count() (int64, error) {
	tx, err := q.statement()
	if err != nil {
		return 0, err
	}
	var n int64
	// Count builds from Statement.Model, which the chain never set (Find and
	// Take derive it from their destination instead).
	if err := tx.Model(new(T)).Count(&n).Error; err != nil {
		return 0, fmt.Errorf("ar: count %T: %w", *new(T), err)
	}
	return n, nil
}

// FindByID loads the record of model T whose single-column primary key
// equals id. When nothing matches, the returned error wraps
// gorm.ErrRecordNotFound.
func FindByID[T any](ctx context.Context, id any) (*T, error) {
	handle, err := session(ctx)
	if err != nil {
		return nil, err
	}
	record := new(T)
	field, err := primaryKey(handle, record)
	if err != nil {
		return nil, err
	}
	if err := handle.Where(field.DBName+" = ?", id).Take(record).Error; err != nil {
		return nil, fmt.Errorf("ar: find %T by primary key: %w", record, err)
	}
	bindModels(ctx, record)
	return record, nil
}
