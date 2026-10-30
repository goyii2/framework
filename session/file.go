package session

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// FileStore persists session records as one JSON file per session id under a
// directory. A record's lifetime is its file's age: loads past the ttl treat
// the record as expired and remove it. Writes go to a temporary file in the
// same directory followed by a rename, so a crash mid-write never leaves a
// half-written record behind.
type FileStore struct {
	dir string

	// mu serializes operations on the directory. Per-record locking would
	// allow more parallelism, but session traffic is short and rare; the
	// simple lock keeps read-modify-write cycles correct across goroutines.
	mu sync.Mutex
}

// NewFileStore creates the file backend rooted at dir. The directory is
// created on first write.
func NewFileStore(dir string) *FileStore {
	return &FileStore{dir: dir}
}

// Load returns the record for id when it exists and has not outlived ttl;
// expired records are removed on sight.
func (s *FileStore) Load(ctx context.Context, id string, ttl time.Duration) (map[string]any, bool, error) {
	if !validSessionID(id) {
		// Ids double as file names; a foreign shape never reaches disk.
		return nil, false, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	path := s.path(id)
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if time.Since(info.ModTime()) >= ttl {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return nil, false, err
		}
		return nil, false, nil
	}

	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		// Removed between stat and read: as good as absent.
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	data, err := unmarshalSession(b)
	if err != nil {
		return nil, false, err
	}
	return data, data != nil, nil
}

// Save replaces the record for id and refreshes its lifetime: the write time
// becomes the new expiry base.
func (s *FileStore) Save(ctx context.Context, id string, data map[string]any, ttl time.Duration) error {
	if !validSessionID(id) {
		return fmt.Errorf("session: invalid session id shape")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return err
	}
	b, err := marshalSession(data)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(s.dir, "sess-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, s.path(id))
}

// Destroy removes the record for id; absent records are not an error.
func (s *FileStore) Destroy(ctx context.Context, id string) error {
	if !validSessionID(id) {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.Remove(s.path(id)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// path maps a session id to its file inside the store directory.
func (s *FileStore) path(id string) string {
	return filepath.Join(s.dir, id)
}
