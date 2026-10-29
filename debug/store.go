package debug

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// defaultKeep is the number of request files retained when the panel
// configuration names none.
const defaultKeep = 200

// fileSuffix identifies request records among the files of the panel's
// directory.
const fileSuffix = ".json"

// store writes one entry as a JSON file under dir and evicts the oldest
// records beyond keep. File names sort chronologically (see nextID) and the
// name without the suffix is the entry's id — the detail page's ?id=
// parameter.
func (p *Panel) store(e *Entry) error {
	if err := os.MkdirAll(p.dir(), 0o755); err != nil {
		return err
	}
	if e.ID == "" {
		e.ID = p.nextID(e.Time)
	}
	data, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(p.dir(), e.ID+fileSuffix), data, 0o644); err != nil {
		return err
	}
	return p.evict()
}

// evict deletes the oldest record files until at most keep remain; listing
// trouble and individual deletions are reported, never fatal — the panel
// must not take the response path down with it.
func (p *Panel) evict() error {
	names, err := listEntries(p.dir())
	if err != nil {
		return err
	}
	keep := p.KeepFiles
	if keep <= 0 {
		keep = defaultKeep
	}
	var errs []string
	for _, name := range names[:max(0, len(names)-keep)] {
		if err := os.Remove(filepath.Join(p.dir(), name)); err != nil && !os.IsNotExist(err) {
			errs = append(errs, err.Error())
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("debug: evict: %s", strings.Join(errs, "; "))
	}
	return nil
}

// listEntries returns the record file names in the panel directory, oldest
// first (names sort chronologically by construction).
func listEntries(dir string) ([]string, error) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(ents))
	for _, ent := range ents {
		if ent.IsDir() || !strings.HasSuffix(ent.Name(), fileSuffix) {
			continue
		}
		names = append(names, ent.Name())
	}
	slices.Sort(names)
	return names, nil
}

// load reads one stored entry by id; an id that does not name a stored file
// (wrong shape, path traversal, already evicted) reports os.ErrNotExist.
func (p *Panel) load(id string) (*Entry, error) {
	if !validID(id) {
		return nil, os.ErrNotExist
	}
	data, err := os.ReadFile(filepath.Join(p.dir(), id+fileSuffix))
	if err != nil {
		return nil, err
	}
	var e Entry
	if err := json.Unmarshal(data, &e); err != nil {
		return nil, err
	}
	return &e, nil
}

// history reads the newest entries for the list page, newest first.
func (p *Panel) history(limit int) ([]*Entry, error) {
	names, err := listEntries(p.dir())
	if err != nil {
		return nil, err
	}
	var out []*Entry
	for i := len(names) - 1; i >= 0 && len(out) < limit; i-- {
		e, err := p.load(strings.TrimSuffix(names[i], fileSuffix))
		if err != nil {
			continue // evicted mid-read; the list skips it
		}
		out = append(out, e)
	}
	return out, nil
}

// validID reports whether id may name a stored entry: ids are generated
// (timestamp + sequence) so anything with other characters — separators,
// dots leading upward — is rejected before it reaches the filesystem.
func validID(id string) bool {
	if id == "" || len(id) > 64 {
		return false
	}
	for _, r := range id {
		switch {
		case r >= '0' && r <= '9', r >= 'A' && r <= 'Z', r == '-', r == '.':
		default:
			return false
		}
	}
	return !strings.Contains(id, "..")
}
