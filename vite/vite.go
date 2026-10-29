package vite

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"sync"

	"github.com/a-h/templ"
)

// Sentinel errors Entry reports. Both are wrapped with the offending path
// or name, so errors.Is tells them apart while the message points at the
// cause.
var (
	// ErrInvalidManifest: the manifest exists but cannot be used — unreadable,
	// corrupt JSON, wrong structure, chunks without a file. A manifest that
	// is simply absent is not an error; it switches Vite into dev mode.
	ErrInvalidManifest = errors.New("vite: invalid manifest")

	// ErrEntryNotFound: the requested name is not a key of the manifest.
	// Keys are the source paths Vite writes, e.g. "src/main.ts".
	ErrEntryNotFound = errors.New("vite: entry not found")
)

// mode is how a Vite resolves assets.
type mode int

const (
	modeBuild mode = iota // manifest loaded; hashed build URLs
	modeDev               // dev-server URLs (Dev switch or manifest absent)
)

// Vite resolves frontend entry assets into templ components: from the build
// manifest in production, straight from the Vite dev server in development.
// Configure it as a struct and use it through a pointer, so the manifest is
// loaded once:
//
//	v := &vite.Vite{Manifest: "web/dist/.vite/manifest.json"}
//	head, err := v.Entry("src/main.ts")
//
// The mode is decided on first use and stays fixed for the process:
//
//   - Dev set: dev mode; the manifest is never read.
//   - Manifest absent (or empty): dev mode. Pointing at a build that has not
//     run yet therefore yields dev URLs instead of an error — running vite
//     build and restarting the app flips to hashed URLs.
//   - otherwise: build mode; Entry draws URLs from the manifest and reports
//     ErrEntryNotFound / ErrInvalidManifest explicitly, so a wrong entry
//     name or a broken build fails the first render instead of shipping a
//     broken page.
//
// The zero Vite is usable: with no manifest it connects to the default dev
// server. Keep one pointer for the process — a value copy made before first
// use would resolve its own mode a second time.
type Vite struct {
	// Manifest is the path of the build manifest — Vite 5+ writes it to
	// "<frontend>/dist/.vite/manifest.json". Empty means dev mode.
	Manifest string

	// Base is the URL prefix placed in front of the manifest's hashed file
	// paths; it must match Vite's own base option. "/" (the default) serves
	// the build outputs from the site root.
	Base string

	// Dev forces dev mode: entry scripts point at the dev server and the
	// @vite/client runtime is injected for HMR.
	Dev bool

	// DevServer is the origin of the Vite dev server; default
	// "http://localhost:5173".
	DevServer string

	once    sync.Once
	mode    mode
	chunks  map[string]chunk
	loadErr error
}

// resolve samples the mode once: dev by switch or absent manifest, build
// with the manifest parsed and kept for every later call. A manifest that
// exists but cannot be parsed leaves loadErr set; Entry reports it.
func (v *Vite) resolve() {
	v.once.Do(func() {
		if v.Dev || v.Manifest == "" {
			v.mode = modeDev
			return
		}
		data, err := os.ReadFile(v.Manifest)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				v.mode = modeDev
				return
			}
			v.loadErr = fmt.Errorf("%w: read %s: %w", ErrInvalidManifest, v.Manifest, err)
			return
		}
		chunks, err := parseManifest(data)
		if err != nil {
			v.loadErr = fmt.Errorf("%w: %s: %w", ErrInvalidManifest, v.Manifest, err)
			return
		}
		v.mode = modeBuild
		v.chunks = chunks
	})
}

// IsDev reports whether Entry serves dev-server URLs: the Dev switch is on,
// or no manifest is in place yet. Assemblies use it to mount DevProxy only
// in development, so a production build never forwards static paths to
// localhost.
func (v *Vite) IsDev() bool {
	if v.Dev {
		return true
	}
	v.resolve()
	return v.mode == modeDev
}

// Entry returns the asset tags of one entry as a single component: a
// stylesheet link per CSS file of the entry's static-import subtree, then
// the entry's script (type=module, hashed URL in build mode). The name is
// the manifest key — the entry's source path, e.g. "src/main.ts".
//
// In dev mode the component loads @vite/client first (the HMR runtime), then
// the entry straight from the dev server. No stylesheet links are emitted in
// dev mode: the dev server injects styles through its JS module graph, so a
// link would duplicate them.
//
// The error is explicit rather than a silently empty component, so a wrong
// entry name or a broken build surfaces at the first render.
func (v *Vite) Entry(name string) (templ.Component, error) {
	if v.IsDev() {
		origin := v.devOrigin()
		client := origin + "/@vite/client"
		script := origin + "/" + strings.TrimPrefix(name, "/")
		return entryTags(nil, []string{client, script}), nil
	}
	if v.loadErr != nil {
		return nil, v.loadErr
	}
	c, ok := v.chunks[name]
	if !ok {
		return nil, fmt.Errorf("%w: %q is not a key of %s", ErrEntryNotFound, name, v.Manifest)
	}
	base := v.base()
	styles := v.collectCSS(name, base)
	return entryTags(styles, []string{joinURL(base, c.File)}), nil
}

// base returns Base normalized to a path prefix with leading and trailing
// slash; "" and "/" both mean the site root.
func (v *Vite) base() string {
	b := v.Base
	if b == "" {
		return "/"
	}
	if !strings.HasPrefix(b, "/") {
		b = "/" + b
	}
	return strings.TrimSuffix(b, "/") + "/"
}

// devOrigin returns DevServer without a trailing slash; "" means the
// default localhost:5173.
func (v *Vite) devOrigin() string {
	origin := v.DevServer
	if origin == "" {
		origin = "http://localhost:5173"
	}
	return strings.TrimSuffix(origin, "/")
}

// joinURL concatenates a normalized prefix (always trailing slash) and a
// manifest-relative path.
func joinURL(prefix, file string) string {
	return prefix + strings.TrimPrefix(file, "/")
}
