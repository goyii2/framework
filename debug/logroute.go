package debug

import (
	"bytes"
	"runtime"
	"strconv"
	"sync"

	"github.com/goyii2/framework/log"
)

// requestRouter attributes log records to requests without invasive
// instrumentation: every record a request emits is written synchronously on
// the goroutine serving that request (action code, the GORM adapter, the
// mail logger, the profiler all run there), so routing by the emitting
// goroutine's identity separates concurrent requests exactly. begin
// registers a memory target under the calling goroutine, end unregisters it,
// and the dispatch target — bound once into the application's logger at
// mount time — forwards each record to whatever the writing goroutine
// registered. Records from goroutines serving no request (background
// workers, CLI) fall through and are dropped.
type requestRouter struct {
	mu     sync.RWMutex
	byGoid map[uint64]*log.MemoryTarget
}

// begin registers a fresh memory target for the calling goroutine's request
// and returns it.
func (rr *requestRouter) begin() *log.MemoryTarget {
	t := log.NewMemoryTarget(4096)
	rr.mu.Lock()
	defer rr.mu.Unlock()
	if rr.byGoid == nil {
		rr.byGoid = map[uint64]*log.MemoryTarget{}
	}
	rr.byGoid[goid()] = t
	return t
}

// end unregisters the calling goroutine's request target; records it did not
// capture are dropped from the panels.
func (rr *requestRouter) end() {
	rr.mu.Lock()
	defer rr.mu.Unlock()
	delete(rr.byGoid, goid())
}

// target returns the memory target registered for the calling goroutine, nil
// when this goroutine serves no request.
func (rr *requestRouter) target() *log.MemoryTarget {
	rr.mu.RLock()
	defer rr.mu.RUnlock()
	return rr.byGoid[goid()]
}

// dispatchTarget is the log target the panel binds into the application's
// logger: it carries every dispatched record to the request the emitting
// goroutine belongs to.
type dispatchTarget struct {
	router *requestRouter
}

// Name identifies the target in diagnostics.
func (d *dispatchTarget) Name() string { return "debug" }

// Write forwards the record to the current goroutine's request, when there
// is one.
func (d *dispatchTarget) Write(rec log.Record) error {
	if t := d.router.target(); t != nil {
		return t.Write(rec)
	}
	return nil
}

// Close releases nothing; routing holds no resources.
func (d *dispatchTarget) Close() error { return nil }

// goid returns the calling goroutine's id. The runtime offers no direct
// accessor; the first line of a shallow stack dump ("goroutine 42
// [running]:") is the standard extraction — a fraction of a microsecond,
// paid once per log record and only while the debug panel is mounted.
func goid() uint64 {
	var buf [64]byte
	n := runtime.Stack(buf[:], false)
	line := buf[:n]
	if !bytes.HasPrefix(line, goHead) {
		return 0
	}
	line = line[len(goHead):]
	if i := bytes.IndexByte(line, ' '); i >= 0 {
		line = line[:i]
	}
	id, err := strconv.ParseUint(string(line), 10, 64)
	if err != nil {
		return 0
	}
	return id
}

// goHead is the constant prefix of a stack dump's first line.
var goHead = []byte("goroutine ")
