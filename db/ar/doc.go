// Package ar implements the Active Record data access style on top of the
// application's database connection: one model struct maps to one table row,
// query construction uses the Yii2 vocabulary, and model instances carry
// Save, Delete and Reload. It corresponds to Yii2's yii\db\ActiveRecord and
// ActiveQuery.
//
// A model is a plain struct with GORM column tags; embedding Model gives
// instances the persistence methods:
//
//	type User struct {
//	    ar.Model
//	    ID   int64  `gorm:"column:id;primaryKey"`
//	    Name string `gorm:"column:name"`
//	}
//
// Queries work with any struct, embedded Model or not; records of types
// without it are simply detached from the persistence helpers.
//
// The database handle always resolves from the context at execution time:
// the open transaction when inside Transaction, then a connection installed
// with UseConn, then the application's db component (base.FromContext).
// Model lifecycle hooks are the GORM hook interfaces (BeforeCreate,
// BeforeSave, AfterDelete, ...) implemented as methods on the model struct;
// the package adds no event mechanism of its own.
//
// Typical use:
//
//	q := ar.New[User](ctx).Where("status = ?", 1).OrderBy("id DESC").Limit(10)
//	users, err := q.All()
//	user, err := ar.FindByID[User](ctx, 7)
//	fresh := ar.New[User](ctx).New() // fill fields, then fresh.Save()
package ar
