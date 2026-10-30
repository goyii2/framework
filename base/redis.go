package base

import (
	"github.com/redis/go-redis/v9"
)

// Redis returns the application's shared redis connection — the one every
// redis-backed component (mutex, queue, session, a sharing cache) draws.
// Redis has no fallback: accessing it before the application entry installed
// a client (SetRedis) is a programmer error and panics. go-redis dials
// lazily on first use, so installing a client never opens a connection by
// itself.
func (a *Application) Redis() redis.UniversalClient {
	if a.redis == nil {
		panic("goyii: redis component not wired: construct the client in main and install it with SetRedis")
	}
	return a.redis
}

// SetRedis installs the shared redis connection (any UniversalClient shape:
// single node, cluster, ring — or a test fake). nil uninstalls it, putting
// Redis back into the not-wired state.
func (a *Application) SetRedis(c redis.UniversalClient) { a.redis = c }
