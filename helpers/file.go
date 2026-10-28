package helpers

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// FileExists reports whether path names an existing file or directory. A
// permission error still proves existence, so only os.IsNotExist counts as
// absent — matching Yii2's fileExists semantics of "there is something
// there".
func FileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil || !os.IsNotExist(err)
}

// FileIsDir reports whether path names an existing directory. It is false
// for regular files, for missing paths and for symlinks pointing outside a
// directory (os.Stat follows symlinks).
func FileIsDir(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}

// FileFindOptions tunes FileFind. The zero value finds every regular file
// under the directory with case-sensitive pattern matching.
type FileFindOptions struct {
	// Include restricts the result to files whose relative path (or, for a
	// pattern without '/', whose base name) matches at least one pattern. An
	// empty list includes everything.
	Include []string
	// Exclude drops matching files the same way Include selects them;
	// Exclude wins over Include.
	Exclude []string
	// CaseInsensitive compares patterns and paths without regard to case,
	// for case-insensitive filesystems (the common Windows/macOS layouts).
	CaseInsensitive bool
}

// FileFind lists the regular files under dir, deepest results included,
// corresponding to Yii2's FileHelper::findFiles with its only/except
// wildcard options.
//
// Patterns are slash-separated regardless of platform and are matched
// against the path relative to dir ("assets/app.js", "controllers/User.php").
// Within one segment "*" matches any run of characters and "?" one
// character, path.Match style; the segment "**" matches any number of whole
// segments ("**/vendor/**"). A pattern containing no "/" is matched against
// the base name, so "*.php" finds PHP files at any depth. Patterns are
// validated up front: a malformed pattern is an error, not a silent
// non-match. Results are slash-separated, sorted, and never include
// directories themselves.
func FileFind(dir string, opts FileFindOptions) ([]string, error) {
	if err := validatePatterns(opts.Include); err != nil {
		return nil, err
	}
	if err := validatePatterns(opts.Exclude); err != nil {
		return nil, err
	}
	var found []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if !selected(rel, opts) {
			return nil
		}
		found = append(found, rel)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(found)
	return found, nil
}

// selected applies the Include/Exclude rules to one relative path. Exclude
// wins over Include, as in Yii2's findFiles.
func selected(rel string, opts FileFindOptions) bool {
	if matchesAny(rel, opts.Exclude, opts.CaseInsensitive) {
		return false
	}
	if len(opts.Include) == 0 {
		return true
	}
	return matchesAny(rel, opts.Include, opts.CaseInsensitive)
}

// matchesAny reports whether rel matches at least one pattern, applying the
// base-name shortcut for patterns without a slash.
func matchesAny(rel string, patterns []string, fold bool) bool {
	for _, pat := range patterns {
		var name string
		if strings.Contains(pat, "/") {
			name = rel
		} else {
			name = path.Base(rel)
		}
		if matchPathPattern(pat, name, fold) {
			return true
		}
	}
	return false
}

// validatePatterns reports malformed patterns eagerly so callers hear about
// typos instead of getting empty results.
func validatePatterns(patterns []string) error {
	for _, pat := range patterns {
		for _, seg := range strings.Split(pat, "/") {
			if seg == "**" {
				continue
			}
			if _, err := path.Match(seg, ""); err != nil {
				return fmt.Errorf("helpers: bad find pattern %q: %w", pat, err)
			}
		}
	}
	return nil
}

// matchPathPattern matches a slash-separated pattern against a
// slash-separated name, with "**" spanning any number of segments. When fold
// is set both sides are lowercased first, which is sufficient for the
// ASCII-leaning patterns filesystem matching is asked about.
func matchPathPattern(pattern, name string, fold bool) bool {
	if fold {
		pattern = strings.ToLower(pattern)
		name = strings.ToLower(name)
	}
	return matchSegs(strings.Split(pattern, "/"), strings.Split(name, "/"))
}

// matchSegs is the recursive matcher: the leading "**" may consume any
// number of segments; anything else must match its counterpart segment
// exactly (wildcards within the segment included).
func matchSegs(pattern, name []string) bool {
	if len(pattern) == 0 {
		return len(name) == 0
	}
	if pattern[0] == "**" {
		for skip := 0; skip <= len(name); skip++ {
			if matchSegs(pattern[1:], name[skip:]) {
				return true
			}
		}
		return false
	}
	if len(name) == 0 {
		return false
	}
	if ok, err := path.Match(pattern[0], name[0]); err != nil || !ok {
		return false
	}
	return matchSegs(pattern[1:], name[1:])
}

// FileCopyDir copies the directory tree rooted at src into dst,
// corresponding to Yii2's FileHelper::copyDirectory: subdirectories are
// created as needed and regular files are copied byte-for-byte with their
// permission bits preserved. Symlinks are skipped rather than followed —
// following them can duplicate half a filesystem and cycle forever. dst may
// not live inside src (and vice versa), which would make the copy chase its
// own tail.
func FileCopyDir(src, dst string) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}
	if !srcInfo.IsDir() {
		return fmt.Errorf("helpers: copy source %q is not a directory", src)
	}
	if err := checkNested(src, dst); err != nil {
		return err
	}
	if err := os.MkdirAll(dst, srcInfo.Mode().Perm()); err != nil {
		return err
	}
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == src {
			return nil
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		switch {
		case d.IsDir():
			info, err := d.Info()
			if err != nil {
				return err
			}
			return os.MkdirAll(target, info.Mode().Perm())
		case d.Type()&fs.ModeSymlink != 0:
			return nil
		case d.Type().IsRegular():
			info, err := d.Info()
			if err != nil {
				return err
			}
			return copyFile(p, target, info.Mode().Perm())
		default:
			return nil // devices, sockets and friends have no copy semantics
		}
	})
}

// checkNested rejects a copy whose destination sits inside the source tree
// (or equals it, or the other way around): the walk would chase and corrupt
// its own growing copy.
func checkNested(src, dst string) error {
	srcAbs, err := filepath.Abs(src)
	if err != nil {
		return err
	}
	dstAbs, err := filepath.Abs(dst)
	if err != nil {
		return err
	}
	if within, err := withinDir(srcAbs, dstAbs); err != nil {
		return err
	} else if within {
		return fmt.Errorf("helpers: copy destination %q is inside source %q", dst, src)
	}
	if within, err := withinDir(dstAbs, srcAbs); err != nil {
		return err
	} else if within {
		return fmt.Errorf("helpers: copy source %q is inside destination %q", src, dst)
	}
	return nil
}

// withinDir reports whether inner is outer itself or somewhere below it.
func withinDir(outer, inner string) (bool, error) {
	rel, err := filepath.Rel(outer, inner)
	if err != nil {
		return false, err
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))), nil
}

// copyFile copies one regular file, creating or truncating the target and
// restoring the source permission bits.
func copyFile(src, dst string, perm fs.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
