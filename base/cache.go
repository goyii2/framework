package base

import (
	"github.com/goyii2/framework/cache"
)

// Cache returns the cache component. The value is the cache package's Cache
// interface — memory, file or redis behind one contract — so request code
// depends on the contract, never a backend; tests replace it with SetCache.
func (a *Application) Cache() cache.Cache { return a.cache }

// SetCache replaces the cache component (e.g. a test fake or the backend
// built from the project's configuration); nil restores the memory fallback.
func (a *Application) SetCache(c cache.Cache) {
	if c == nil {
		c = newDefaultCache()
	}
	a.cache = c
}

// newDefaultCache builds the fallback cache component: the memory backend.
// The zero-value cache.Config selects it, so this constructor cannot fail; a
// failure here is a framework bug and panics.
func newDefaultCache() cache.Cache {
	c, err := cache.New(cache.Config{})
	if err != nil {
		panic("goyii: building the default cache component: " + err.Error())
	}
	return c
}
