package log

import (
	"io"
	"os"
	"sync"
)

// ConsoleTarget writes records as text lines to an io.Writer, stdout by
// default. Writes are serialized so concurrent logging never interleaves
// partial lines.
type ConsoleTarget struct {
	mu sync.Mutex
	w  io.Writer
}

// NewConsoleTarget builds a console target writing to w; a nil w means
// stdout.
func NewConsoleTarget(w io.Writer) *ConsoleTarget {
	if w == nil {
		w = os.Stdout
	}
	return &ConsoleTarget{w: w}
}

// Name identifies the target in diagnostics.
func (t *ConsoleTarget) Name() string { return "console" }

// Write renders one record as a line to the underlying writer.
func (t *ConsoleTarget) Write(rec Record) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, err := t.w.Write([]byte(formatRecord(rec) + "\n")); err != nil {
		return err
	}
	return nil
}

// Close releases resources; a console target holds none.
func (t *ConsoleTarget) Close() error { return nil }
