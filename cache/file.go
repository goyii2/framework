package cache

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"
)

// File persists one entry per file under a directory, the filename the hash
// of the prefixed key (Yii2's FileCache scheme, md5 for filename-safe
// uniform length). Entries survive process restarts and can be shared by
// processes on the same host. A write goes to a temporary file in the same
// directory followed by a rename, so a crash mid-write never leaves a
// half-written entry; expiry is the envelope's absolute time, checked (and
// lazily cleaned) on read.
type File struct {
	dir        string
	prefix     string
	defaultTTL time.Duration

	// mu serializes directory operations. Per-key locking would allow more
	// parallelism, but each operation is one small file and the simple lock
	// keeps check-then-write cycles correct across goroutines.
	mu sync.Mutex
}

// NewFile creates the file backend rooted at dir, created on first write.
// prefix is folded into the hashed filename so several caches can share a
// directory without colliding; empty means no prefix.
func NewFile(dir string, prefix string, defaultTTL time.Duration) *File {
	return &File{dir: dir, prefix: prefix, defaultTTL: defaultTTL}
}

// Get decodes the value under key into dest; expired and missing entries
// read the same, the expired file removed on sight.
func (f *File) Get(ctx context.Context, key string, dest any) (bool, error) {
	return get(ctx, f, key, dest)
}

// Set stores value under key, replacing any previous entry.
func (f *File) Set(ctx context.Context, key string, value any, ttl time.Duration) error {
	return set(ctx, f, key, value, ttl)
}

// Delete removes key; an absent key is not an error.
func (f *File) Delete(ctx context.Context, key string) error {
	if err := checkKey(key); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.remove(key)
}

// Exists reports whether key holds a live entry.
func (f *File) Exists(ctx context.Context, key string) (bool, error) {
	return exists(ctx, f, key)
}

// Flush removes the entry files this cache owns: the files whose names match
// the backend's 32-hex-hash scheme, plus leftover write temporaries from a
// crashed process. Foreign files in the directory are left alone.
func (f *File) Flush(ctx context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	entries, err := os.ReadDir(f.dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("cache: flush %s: %w", f.dir, err)
	}
	for _, e := range entries {
		if e.IsDir() || (!entryFileName.MatchString(e.Name()) && !tempFileName.MatchString(e.Name())) {
			continue
		}
		if err := os.Remove(filepath.Join(f.dir, e.Name())); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("cache: flush %s: %w", f.dir, err)
		}
	}
	return nil
}

// GetOrSet decodes the entry under key into dest, rebuilding it with build
// when missing, expired or invalidated by dep.
func (f *File) GetOrSet(ctx context.Context, key string, dest any, build func(context.Context) (any, error), ttl time.Duration, dep Dependency) error {
	return getOrSet(ctx, f, key, dest, build, ttl, dep)
}

// lookup reads the entry file, treating missing, unreadable-corrupted and
// expired files as misses; expired files are removed on sight.
func (f *File) lookup(ctx context.Context, key string) (entry, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	b, err := os.ReadFile(f.path(key))
	if os.IsNotExist(err) {
		return entry{}, false, nil
	}
	if err != nil {
		return entry{}, false, fmt.Errorf("cache: read entry for key %q: %w", key, err)
	}
	var e entry
	if err := json.Unmarshal(b, &e); err != nil {
		// A file the backend cannot decode was not written by it (or was
		// corrupted externally): drop it and read as a miss.
		_ = f.remove(key)
		return entry{}, false, nil
	}
	if e.expired(time.Now()) {
		if err := f.remove(key); err != nil {
			return entry{}, false, err
		}
		return entry{}, false, nil
	}
	return e, true, nil
}

// save writes the entry atomically under the resolved lifetime.
func (f *File) save(ctx context.Context, key string, e entry, ttl time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	e.ExpiresAt = time.Now().Add(ttlFor(ttl, f.defaultTTL)).UnixNano()
	b, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("cache: encode entry for key %q: %w", key, err)
	}
	if err := os.MkdirAll(f.dir, 0o700); err != nil {
		return fmt.Errorf("cache: create cache directory %s: %w", f.dir, err)
	}
	tmp, err := os.CreateTemp(f.dir, "cache-*")
	if err != nil {
		return fmt.Errorf("cache: write entry for key %q: %w", key, err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("cache: write entry for key %q: %w", key, err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("cache: write entry for key %q: %w", key, err)
	}
	if err := os.Rename(tmpName, f.path(key)); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("cache: write entry for key %q: %w", key, err)
	}
	return nil
}

// remove deletes the entry file; called with f.mu held.
func (f *File) remove(key string) error {
	if err := os.Remove(f.path(key)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("cache: remove entry for key %q: %w", key, err)
	}
	return nil
}

// path maps a cache key to its entry file: the hex md5 of the prefixed key,
// as in Yii2's FileCache.
func (f *File) path(key string) string {
	sum := md5.Sum([]byte(f.prefix + key))
	return filepath.Join(f.dir, hex.EncodeToString(sum[:]))
}

// entryFileName matches the backend's own file names (32 lowercase hex
// characters), so Flush never touches foreign files in the directory.
var entryFileName = regexp.MustCompile(`^[0-9a-f]{32}$`)

// tempFileName matches the write temporaries save creates ("cache-*"),
// which a crash can leave behind; Flush sweeps them up.
var tempFileName = regexp.MustCompile(`^cache-`)
