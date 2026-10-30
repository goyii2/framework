package base

import (
	"github.com/goyii2/framework/mutex"
)

// Mutex returns the mutex component. The value is the mutex package's Mutex
// interface — process or redis behind one contract — so request code depends
// on the contract, never a driver; tests replace it with SetMutex.
func (a *Application) Mutex() mutex.Mutex { return a.mutex }

// SetMutex replaces the mutex component (e.g. a test fake or the driver
// built from the project's configuration, with the redis driver drawing the
// application's shared connection); nil restores the process fallback.
func (a *Application) SetMutex(m mutex.Mutex) {
	if m == nil {
		m = newDefaultMutex()
	}
	a.mutex = m
}

// newDefaultMutex builds the fallback mutex component: the process driver.
// It touches no external dependency, so this constructor cannot fail.
func newDefaultMutex() mutex.Mutex { return mutex.NewProcess() }
