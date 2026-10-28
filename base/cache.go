package base

import (
	"github.com/goyii2/framework/cache"
	"github.com/goyii2/framework/config"
)

// Cache returns the cache component, building it from the configuration's
// cache section on first access. The value is the cache package's Cache
// interface — memory, file or redis behind one contract — so request code
// depends on the contract, never a backend; tests replace it with SetCache.
func (a *Application) Cache() cache.Cache { return a.cacheComp.get() }

// SetCache replaces the cache component (e.g. a test fake or a pre-wired
// backend); nil restores lazy initialization from the configuration.
func (a *Application) SetCache(c cache.Cache) { a.cacheComp.set(c) }

// newDefaultCache builds the default cache component from the cache
// configuration section. Construction cannot fail: the memory backend needs
// nothing, the file backend creates its directory on first write, and the
// redis client connects lazily on first use.
func newDefaultCache(cfg config.Cache) cache.Cache {
	return cache.NewFromConfig(cfg)
}
