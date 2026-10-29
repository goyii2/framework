// Package vite integrates a Vite frontend build with templ-rendered pages:
// entry scripts and their stylesheets are resolved from the build manifest
// (the resource manifest — hash-named URLs in production) or straight from
// the Vite dev server in development, and a dev-only reverse proxy forwards
// static paths to the dev server with WebSocket upgrades passing through,
// which HMR requires.
//
// A Vite value is struct-configured and used through a pointer so the
// manifest is parsed once for the process:
//
//	v := &vite.Vite{Manifest: "web/dist/.vite/manifest.json"}
//	if v.IsDev() {
//		handler.Use(v.DevProxy("/assets"))
//	}
//	head, err := v.Entry("src/main.ts")
//
// There is deliberately no asset manager and no generic asset registry: the
// manifest is the whole contract. go:embed or a static file server hands out
// the build outputs; this package only draws the tags.
package vite
