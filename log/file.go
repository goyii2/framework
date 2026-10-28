package log

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// FileTarget appends records as text lines to a file, rotating by size:
// when the current file would exceed its limit it is closed and shifted
// into a numbered backup (<path>.1, <path>.2, ...); at most maxFiles
// backups are kept, the oldest being removed on rotation. Writes are
// serialized under a mutex; each Write lands as one intact line.
type FileTarget struct {
	mu       sync.Mutex
	path     string
	maxSize  int64 // bytes; rotate once the current file would exceed it
	maxFiles int   // rotated backups kept; <=0 keeps unlimited
	f        *os.File
	size     int64
}

// NewFileTarget opens (creating when absent) the file at path and returns a
// target that rotates it at maxSizeBytes with maxFiles backups retained.
func NewFileTarget(path string, maxSizeBytes int64, maxFiles int) (*FileTarget, error) {
	if path == "" {
		return nil, fmt.Errorf("log: file target requires a path")
	}
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("log: file target %s: %w", path, err)
		}
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("log: file target %s: %w", path, err)
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("log: file target %s: %w", path, err)
	}
	return &FileTarget{
		path:     path,
		maxSize:  maxSizeBytes,
		maxFiles: maxFiles,
		f:        f,
		size:     info.Size(),
	}, nil
}

// Name identifies the target in diagnostics.
func (t *FileTarget) Name() string { return "file" }

// Write appends one record as a line, rotating first when the line would
// push the current file past its size limit.
func (t *FileTarget) Write(rec Record) error {
	line := []byte(formatRecord(rec) + "\n")

	t.mu.Lock()
	defer t.mu.Unlock()
	if t.f == nil {
		return fmt.Errorf("log: file target %s is closed", t.path)
	}
	if t.maxSize > 0 && t.size+int64(len(line)) > t.maxSize {
		if err := t.rotateLocked(); err != nil {
			return err
		}
	}
	n, err := t.f.Write(line)
	t.size += int64(n)
	if err != nil {
		return fmt.Errorf("log: file target %s: %w", t.path, err)
	}
	return nil
}

// Close releases the file handle; the target cannot be used afterwards.
func (t *FileTarget) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.f == nil {
		return nil
	}
	err := t.f.Close()
	t.f = nil
	if err != nil {
		return fmt.Errorf("log: file target %s: %w", t.path, err)
	}
	return nil
}

// rotateLocked shifts the existing file into the backup chain and reopens a
// fresh one: <path> becomes <path>.1, <path>.1 becomes <path>.2 and so on,
// dropping backups beyond the retention limit (maxFiles <= 0 keeps them
// all). The caller holds the mutex.
func (t *FileTarget) rotateLocked() error {
	if err := t.f.Close(); err != nil {
		return fmt.Errorf("log: file target %s: rotate: %w", t.path, err)
	}
	t.f = nil

	// Highest backup number that still participates in the shift: capped by
	// the retention limit, or the highest existing backup when retention is
	// unlimited.
	limit := 0
	if t.maxFiles > 0 {
		_ = os.Remove(backupPath(t.path, t.maxFiles)) // falls out of retention
		limit = t.maxFiles - 1
	} else {
		for n := 1; ; n++ {
			if _, err := os.Stat(backupPath(t.path, n)); err != nil {
				limit = n - 1
				break
			}
		}
	}
	for n := limit; n >= 1; n-- {
		if err := os.Rename(backupPath(t.path, n), backupPath(t.path, n+1)); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("log: file target %s: rotate: %w", t.path, err)
		}
	}
	if err := os.Rename(t.path, backupPath(t.path, 1)); err != nil {
		return fmt.Errorf("log: file target %s: rotate: %w", t.path, err)
	}
	f, err := os.OpenFile(t.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("log: file target %s: rotate: %w", t.path, err)
	}
	t.f = f
	t.size = 0
	return nil
}

// backupPath returns the numbered rotation backup path for a log file.
func backupPath(path string, n int) string {
	return path + "." + fmt.Sprint(n)
}
