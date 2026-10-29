package vite

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/a-h/templ"
)

// sampleManifest mirrors a Vite 5 build: two entries, one shared chunk and
// one chunk importing the shared chunk again (CSS dedupe case).
const sampleManifest = `{
  "src/main.ts": {
    "file": "assets/main-BQ3agFh.js",
    "src": "src/main.ts",
    "isEntry": true,
    "imports": ["_shared.7a88ce62"],
    "css": ["assets/main-BkeYGeK0.css"]
  },
  "src/admin.ts": {
    "file": "assets/admin-Cx4Yv12z.js",
    "src": "src/admin.ts",
    "isEntry": true,
    "imports": ["_shared.7a88ce62", "_theme.a1b2c3"],
    "css": ["assets/admin-Dz9q1m.css"]
  },
  "_shared.7a88ce62": {
    "file": "assets/shared-B7NtZUdu.js",
    "name": "shared",
    "css": ["assets/shared-77aa.css"]
  },
  "_theme.a1b2c3": {
    "file": "assets/theme-11aabb.js",
    "name": "theme",
    "imports": ["_shared.7a88ce62"],
    "css": ["assets/theme-22bb.css"]
  }
}`

// writeManifest writes content under a temp dir the way vite build would
// and returns its path.
func writeManifest(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), ".vite", "manifest.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// renderString renders one component to a string.
func renderString(t *testing.T, c templ.Component) string {
	t.Helper()
	var sb strings.Builder
	if err := c.Render(context.Background(), &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	return sb.String()
}

func TestParseManifest(t *testing.T) {
	chunks, err := parseManifest([]byte(sampleManifest))
	if err != nil {
		t.Fatalf("parseManifest: %v", err)
	}
	if len(chunks) != 4 {
		t.Fatalf("len(chunks) = %d, want 4", len(chunks))
	}
	main := chunks["src/main.ts"]
	if main.File != "assets/main-BQ3agFh.js" {
		t.Errorf("File = %q", main.File)
	}
	if main.Src != "src/main.ts" {
		t.Errorf("Src = %q", main.Src)
	}
	if !main.IsEntry {
		t.Error("src/main.ts IsEntry = false, want true")
	}
	if len(main.Imports) != 1 || main.Imports[0] != "_shared.7a88ce62" {
		t.Errorf("Imports = %v", main.Imports)
	}
	if len(main.CSS) != 1 || main.CSS[0] != "assets/main-BkeYGeK0.css" {
		t.Errorf("CSS = %v", main.CSS)
	}
	if shared := chunks["_shared.7a88ce62"]; shared.IsEntry {
		t.Error("shared chunk IsEntry = true, want false")
	}
}

func TestParseManifestRejectsBrokenInput(t *testing.T) {
	cases := map[string]string{
		"not json":       `{"src/main.ts":`,
		"wrong shape":    `{"src/main.ts": "a string"}`,
		"no file":        `{"src/main.ts": {"src": "src/main.ts", "isEntry": true}}`,
		"empty manifest": `{}`,
	}
	for name, content := range cases {
		if _, err := parseManifest([]byte(content)); err == nil {
			t.Errorf("%s: parseManifest succeeded, want error", name)
		}
	}
}

func TestEntryBuildModeRendersHashedTags(t *testing.T) {
	v := &Vite{Manifest: writeManifest(t, sampleManifest)}
	got, err := v.Entry("src/main.ts")
	if err != nil {
		t.Fatalf("Entry: %v", err)
	}
	// Dependency CSS (shared chunk) before the entry's own CSS, then the
	// hashed module script.
	want := `<link rel="stylesheet" href="/assets/shared-77aa.css">` +
		`<link rel="stylesheet" href="/assets/main-BkeYGeK0.css">` +
		`<script type="module" src="/assets/main-BQ3agFh.js"></script>`
	if html := renderString(t, got); html != want {
		t.Errorf("Entry html =\n%s\nwant\n%s", html, want)
	}
}

func TestEntryCollectsCSSRecursively(t *testing.T) {
	// src/admin.ts imports _shared (directly and again through _theme):
	// every CSS file appears once, dependencies first.
	v := &Vite{Manifest: writeManifest(t, sampleManifest)}
	got, err := v.Entry("src/admin.ts")
	if err != nil {
		t.Fatalf("Entry: %v", err)
	}
	html := renderString(t, got)
	wantOrder := []string{
		`<link rel="stylesheet" href="/assets/shared-77aa.css">`,
		`<link rel="stylesheet" href="/assets/theme-22bb.css">`,
		`<link rel="stylesheet" href="/assets/admin-Dz9q1m.css">`,
		`<script type="module" src="/assets/admin-Cx4Yv12z.js"></script>`,
	}
	at := -1
	for _, tag := range wantOrder {
		i := strings.Index(html, tag)
		if i < 0 {
			t.Fatalf("Entry html missing %s:\n%s", tag, html)
		}
		if n := strings.Count(html, tag); n != 1 {
			t.Errorf("%s appears %d times, want 1", tag, n)
		}
		if i <= at {
			t.Errorf("%s out of order in:\n%s", tag, html)
		}
		at = i
	}
}

func TestBasePrefix(t *testing.T) {
	manifest := writeManifest(t, sampleManifest)
	cases := map[string]string{
		"":         "/assets/main-BQ3agFh.js",
		"/":        "/assets/main-BQ3agFh.js",
		"/my-app":  "/my-app/assets/main-BQ3agFh.js",
		"/my-app/": "/my-app/assets/main-BQ3agFh.js",
		"my-app":   "/my-app/assets/main-BQ3agFh.js",
	}
	for base, wantSrc := range cases {
		v := &Vite{Manifest: manifest, Base: base}
		got, err := v.Entry("src/main.ts")
		if err != nil {
			t.Fatalf("base %q: Entry: %v", base, err)
		}
		if html := renderString(t, got); !strings.Contains(html, `src="`+wantSrc+`"`) {
			t.Errorf("base %q: html missing src %q:\n%s", base, wantSrc, html)
		}
	}
}

func TestDevFallbackWhenManifestMissing(t *testing.T) {
	v := &Vite{Manifest: filepath.Join(t.TempDir(), "dist", ".vite", "manifest.json")}
	if !v.IsDev() {
		t.Fatal("IsDev = false with absent manifest, want true")
	}
	got, err := v.Entry("src/main.ts")
	if err != nil {
		t.Fatalf("Entry: %v", err)
	}
	want := `<script type="module" src="http://localhost:5173/@vite/client"></script>` +
		`<script type="module" src="http://localhost:5173/src/main.ts"></script>`
	if html := renderString(t, got); html != want {
		t.Errorf("Entry html =\n%s\nwant\n%s", html, want)
	}
	if html := renderString(t, got); strings.Contains(html, "<link") {
		t.Error("dev mode emitted a stylesheet link")
	}
}

func TestDevSwitchIgnoresManifest(t *testing.T) {
	v := &Vite{Manifest: writeManifest(t, sampleManifest), Dev: true}
	if !v.IsDev() {
		t.Fatal("IsDev = false with Dev set, want true")
	}
	got, err := v.Entry("src/main.ts")
	if err != nil {
		t.Fatalf("Entry: %v", err)
	}
	if html := renderString(t, got); !strings.Contains(html, `src="http://localhost:5173/src/main.ts"`) {
		t.Errorf("Entry html = %s, want dev-server script", html)
	}
}

func TestDevServerConfig(t *testing.T) {
	v := &Vite{Dev: true, DevServer: "http://127.0.0.1:3000/"}
	got, err := v.Entry("src/main.ts")
	if err != nil {
		t.Fatalf("Entry: %v", err)
	}
	html := renderString(t, got)
	for _, want := range []string{
		`src="http://127.0.0.1:3000/@vite/client"`,
		`src="http://127.0.0.1:3000/src/main.ts"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("html missing %s:\n%s", want, html)
		}
	}
}

func TestZeroViteIsDevAgainstDefaultServer(t *testing.T) {
	v := &Vite{}
	if !v.IsDev() {
		t.Fatal("zero Vite IsDev = false, want true")
	}
	got, err := v.Entry("src/main.ts")
	if err != nil {
		t.Fatalf("Entry: %v", err)
	}
	if html := renderString(t, got); !strings.Contains(html, `http://localhost:5173/src/main.ts"`) {
		t.Errorf("Entry html = %s, want default dev server", html)
	}
}

func TestModeIsStickyOnceResolved(t *testing.T) {
	manifest := filepath.Join(t.TempDir(), ".vite", "manifest.json")
	if err := os.MkdirAll(filepath.Dir(manifest), 0o755); err != nil {
		t.Fatal(err)
	}
	v := &Vite{Manifest: manifest}
	if !v.IsDev() {
		t.Fatal("IsDev = false before the build ran, want true")
	}
	// The build appears after the mode was sampled: the process stays in
	// dev mode (restart flips it) — IsDev and Entry must not disagree.
	if err := os.WriteFile(manifest, []byte(sampleManifest), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := v.Entry("src/main.ts")
	if err != nil {
		t.Fatalf("Entry: %v", err)
	}
	if html := renderString(t, got); !strings.Contains(html, "localhost:5173") {
		t.Errorf("Entry html = %s, want sticky dev mode", html)
	}
}

func TestEntryNotFound(t *testing.T) {
	v := &Vite{Manifest: writeManifest(t, sampleManifest)}
	_, err := v.Entry("src/other.ts")
	if !errors.Is(err, ErrEntryNotFound) {
		t.Fatalf("err = %v, want ErrEntryNotFound", err)
	}
	if !strings.Contains(err.Error(), "src/other.ts") {
		t.Errorf("err = %v, want the entry name in the message", err)
	}
}

func TestEntryInvalidManifest(t *testing.T) {
	v := &Vite{Manifest: writeManifest(t, `{"src/main.ts": `)}
	if v.IsDev() {
		t.Fatal("IsDev = true with corrupt manifest, want false")
	}
	_, err := v.Entry("src/main.ts")
	if !errors.Is(err, ErrInvalidManifest) {
		t.Fatalf("err = %v, want ErrInvalidManifest", err)
	}
	if !strings.Contains(err.Error(), "invalid manifest") {
		t.Errorf("err = %v, want reason in the message", err)
	}
}
