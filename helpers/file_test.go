package helpers

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// writeFile is the test fixture helper: create p with content and permission
// 0644, failing the test on the first error.
func writeFile(t *testing.T, p, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// findFixture builds:
//
//	root/a.txt, root/b.log, root/sub/c.txt, root/sub/deep/d.txt
func findFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "a.txt"), "a")
	writeFile(t, filepath.Join(root, "b.log"), "b")
	writeFile(t, filepath.Join(root, "sub", "c.txt"), "c")
	writeFile(t, filepath.Join(root, "sub", "deep", "d.txt"), "d")
	return root
}

func TestFileExists(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "x.txt")
	writeFile(t, file, "x")

	if !FileExists(file) {
		t.Error("FileExists should be true for a regular file")
	}
	if !FileExists(dir) {
		t.Error("FileExists should be true for a directory (Yii2 fileExists counts both)")
	}
	if FileExists(filepath.Join(dir, "missing.txt")) {
		t.Error("FileExists should be false for a missing path")
	}
}

func TestFileIsDir(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "x.txt")
	writeFile(t, file, "x")

	if !FileIsDir(dir) {
		t.Error("FileIsDir should be true for a directory")
	}
	if FileIsDir(file) {
		t.Error("FileIsDir should be false for a regular file")
	}
	if FileIsDir(filepath.Join(dir, "missing")) {
		t.Error("FileIsDir should be false for a missing path")
	}
}

func TestFileFindDefaultsToAllFiles(t *testing.T) {
	root := findFixture(t)
	got, err := FileFind(root, FileFindOptions{})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"a.txt", "b.log", "sub/c.txt", "sub/deep/d.txt"}
	if len(got) != 4 {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Fatalf("results must be sorted slash-separated paths: got %v", got)
		}
	}
}

func TestFileFindIncludePatterns(t *testing.T) {
	root := findFixture(t)
	cases := []struct {
		include []string
		want    []string
	}{
		// No slash: matched against the base name at any depth.
		{[]string{"*.txt"}, []string{"a.txt", "sub/c.txt", "sub/deep/d.txt"}},
		{[]string{"*.log"}, []string{"b.log"}},
		// With a slash: matched against the relative path.
		{[]string{"sub/*.txt"}, []string{"sub/c.txt"}},
		{[]string{"a.txt"}, []string{"a.txt"}},
		// "**" spans segments.
		{[]string{"**/deep/*.txt"}, []string{"sub/deep/d.txt"}},
		{[]string{"sub/**"}, []string{"sub/c.txt", "sub/deep/d.txt"}},
		{[]string{"*.md"}, []string{}},
	}
	for _, tc := range cases {
		got, err := FileFind(root, FileFindOptions{Include: tc.include})
		if err != nil {
			t.Fatalf("include %v: %v", tc.include, err)
		}
		if len(got) != len(tc.want) {
			t.Errorf("include %v: got %v, want %v", tc.include, got, tc.want)
			continue
		}
		for i := range tc.want {
			if got[i] != tc.want[i] {
				t.Errorf("include %v: got %v, want %v", tc.include, got, tc.want)
				break
			}
		}
	}
}

func TestFileFindExcludeWins(t *testing.T) {
	root := findFixture(t)
	got, err := FileFind(root, FileFindOptions{
		Include: []string{"*.txt"},
		Exclude: []string{"sub/**"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "a.txt" {
		t.Errorf("exclude must override include: got %v", got)
	}
}

func TestFileFindCaseSensitivity(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "README.md"), "r")

	got, err := FileFind(root, FileFindOptions{Include: []string{"readme.md"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("default matching is case-sensitive: got %v", got)
	}

	got, err = FileFind(root, FileFindOptions{Include: []string{"readme.md"}, CaseInsensitive: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "README.md" {
		t.Errorf("case-insensitive matching should find it: got %v", got)
	}
}

func TestFileFindBadPatternIsError(t *testing.T) {
	root := findFixture(t)
	if _, err := FileFind(root, FileFindOptions{Include: []string{"[unclosed"}}); err == nil {
		t.Error("malformed pattern must be an error, not a silent non-match")
	}
	if _, err := FileFind(root, FileFindOptions{Exclude: []string{"ok.txt", "bad[.txt"}}); err == nil {
		t.Error("malformed exclude pattern must be an error")
	}
}

func TestFileFindMissingDirIsError(t *testing.T) {
	if _, err := FileFind(filepath.Join(t.TempDir(), "nope"), FileFindOptions{}); err == nil {
		t.Error("scanning a missing directory must be an error")
	}
}

func TestFileCopyDir(t *testing.T) {
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "a.txt"), "alpha")
	writeFile(t, filepath.Join(src, "nested", "b.txt"), "beta")

	dst := filepath.Join(t.TempDir(), "out")
	if err := FileCopyDir(src, dst); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct{ rel, want string }{
		{"a.txt", "alpha"},
		{filepath.Join("nested", "b.txt"), "beta"},
	} {
		b, err := os.ReadFile(filepath.Join(dst, tc.rel))
		if err != nil {
			t.Fatalf("copied %s missing: %v", tc.rel, err)
		}
		if string(b) != tc.want {
			t.Errorf("copied %s content got %q, want %q", tc.rel, b, tc.want)
		}
	}

	// Copying again over the existing destination must be idempotent
	// (O_TRUNC on an existing file).
	writeFile(t, filepath.Join(src, "a.txt"), "alpha-2")
	if err := FileCopyDir(src, dst); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(dst, "a.txt")); string(b) != "alpha-2" {
		t.Errorf("re-copy should overwrite, got %q", b)
	}
}

func TestFileCopyDirPreservesMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits are not meaningful on Windows")
	}
	src := t.TempDir()
	p := filepath.Join(src, "run.sh")
	writeFile(t, p, "#!/bin/sh\n")
	if err := os.Chmod(p, 0o755); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(t.TempDir(), "out")
	if err := FileCopyDir(src, dst); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(dst, "run.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Errorf("mode bits got %v, want -rwxr-xr-x", info.Mode().Perm())
	}
}

func TestFileCopyDirSkipsSymlinks(t *testing.T) {
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "real.txt"), "r")
	if err := os.Symlink(filepath.Join(src, "real.txt"), filepath.Join(src, "link.txt")); err != nil {
		t.Skip("symlinks unavailable on this system")
	}
	dst := filepath.Join(t.TempDir(), "out")
	if err := FileCopyDir(src, dst); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(dst, "link.txt")); !os.IsNotExist(err) {
		t.Error("symlinks must be skipped, not followed")
	}
}

func TestFileCopyDirRejectsBadSourcesAndNesting(t *testing.T) {
	tmp := t.TempDir()
	file := filepath.Join(tmp, "plain.txt")
	writeFile(t, file, "x")

	if err := FileCopyDir(file, filepath.Join(tmp, "out")); err == nil {
		t.Error("copying a regular file must be an error")
	}
	if err := FileCopyDir(filepath.Join(tmp, "missing"), filepath.Join(tmp, "out")); err == nil {
		t.Error("copying a missing directory must be an error")
	}

	dir := filepath.Join(tmp, "src")
	writeFile(t, filepath.Join(dir, "a.txt"), "a")
	if err := FileCopyDir(dir, filepath.Join(dir, "inside")); err == nil {
		t.Error("destination inside source must be an error")
	}
	if err := FileCopyDir(dir, dir); err == nil {
		t.Error("destination equal to source must be an error")
	}
}
