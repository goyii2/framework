package base

import (
	"fmt"
	"os"
	"strings"

	"github.com/redis/go-redis/v9"

	"github.com/goyii2/framework/config"
	"github.com/goyii2/framework/mutex"
)

// Mutex returns the mutex component, building it from the configuration's
// mutex section on first access. The value is the mutex package's Mutex
// interface — process or redis behind one contract — so request code
// depends on the contract, never a driver; tests replace it with SetMutex.
func (a *Application) Mutex() mutex.Mutex { return a.mutexComp.get() }

// SetMutex replaces the mutex component (e.g. a test fake or a differently
// configured driver); nil restores lazy initialization from the
// configuration.
func (a *Application) SetMutex(m mutex.Mutex) { a.mutexComp.set(m) }

// newDefaultMutex builds the default mutex component from the mutex
// configuration section. The redis driver shares the application's redis
// connection unconditionally — a mutex coordinates processes, so a
// per-section override would hand out locks nobody else sees — while the
// process driver (the default) touches nothing. A section the constructor
// rejects (an unknown driver) is reported on stderr and degrades to the
// process driver so the application still starts.
func newDefaultMutex(cfg config.Mutex, shared *component[redis.UniversalClient]) mutex.Mutex {
	var client redis.UniversalClient
	if strings.EqualFold(strings.TrimSpace(cfg.Driver), "redis") {
		client = shared.get()
	}
	m, err := mutex.New(mutex.Config{Driver: cfg.Driver}, mutex.WithRedis(client))
	if err != nil {
		fmt.Fprintf(os.Stderr, "goyii: %v; falling back to the process mutex\n", err)
		return mutex.NewProcess()
	}
	return m
}
