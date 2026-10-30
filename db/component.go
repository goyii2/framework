package db

import (
	"sync"
	"sync/atomic"
)

// Component is the lazily initialized database connection holder, for
// applications that want lazy or retryable open semantics instead of eager
// assembly (New at the application entry). The first Get opens the
// connection from the stored configuration (with any stored options
// applied); Set replaces the connection at any time (nil resets the holder
// so the next Get opens from the configuration again), which is how tests
// inject fakes or pre-opened connections. Unlike a value component, opening
// can fail, so Get returns the error; a failed open is not cached — the next
// Get retries, so a database that becomes reachable later still connects.
// Safe for concurrent use.
type Component struct {
	mu   sync.Mutex // serializes the first open
	cfg  Config
	opts []Option
	val  atomic.Pointer[Conn]
}

// NewComponent creates a holder that opens connections for cfg on first
// use, applying the given options on every open.
func NewComponent(cfg Config, opts ...Option) *Component {
	return &Component{cfg: cfg, opts: opts}
}

// Get returns the current connection, opening it on first access.
func (c *Component) Get() (*Conn, error) {
	if conn := c.val.Load(); conn != nil {
		return conn, nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if conn := c.val.Load(); conn != nil {
		return conn, nil
	}
	conn, err := New(c.cfg, c.opts...)
	if err != nil {
		return nil, err
	}
	c.val.Store(conn)
	return conn, nil
}

// Set replaces the connection. A nil connection resets the holder to lazy
// initialization from the stored configuration.
func (c *Component) Set(conn *Conn) {
	if conn == nil {
		c.val.Store(nil)
		return
	}
	c.val.Store(conn)
}
