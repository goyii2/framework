package vite

import (
	"encoding/json"
	"fmt"
)

// chunk is one entry of the Vite build manifest (.vite/manifest.json, the
// Vite 5+ default path): the build output of one module, keyed by source
// path for entries ("src/main.ts") and by internal id for shared chunks
// ("_shared.7a88ce62"). Fields follow what Vite writes; extra fields it may
// add (name, dynamicImports, ...) are ignored.
type chunk struct {
	// File is the hashed output path relative to the build outDir,
	// e.g. "assets/main-BQ3agFh.js".
	File string `json:"file"`
	// Src is the module's source path; for entries it equals the key.
	Src string `json:"src"`
	// IsEntry marks entry chunks. Informational: Entry accepts any manifest
	// key, so shared chunks can be referenced directly too.
	IsEntry bool `json:"isEntry"`
	// CSS lists the stylesheets this chunk brings in.
	CSS []string `json:"css"`
	// Imports lists the manifest keys of the chunk's static imports.
	Imports []string `json:"imports"`
}

// parseManifest decodes manifest JSON into its chunks, rejecting shapes
// that cannot yield usable asset URLs (unreadable JSON, wrong structure,
// chunks without a file, nothing at all).
func parseManifest(data []byte) (map[string]chunk, error) {
	var chunks map[string]chunk
	if err := json.Unmarshal(data, &chunks); err != nil {
		return nil, err
	}
	if len(chunks) == 0 {
		return nil, fmt.Errorf("manifest has no chunks")
	}
	for key, c := range chunks {
		if c.File == "" {
			return nil, fmt.Errorf("chunk %q has no file", key)
		}
	}
	return chunks, nil
}

// collectCSS walks name's static-import subtree and returns the stylesheet
// URLs: dependency chunks before the importing chunk (mirroring module
// execution order), each file emitted once. visited guards against cycles
// and re-walks; import keys missing from the manifest are skipped.
func (v *Vite) collectCSS(name string, base string) []string {
	var styles []string
	emitted := map[string]bool{}
	visited := map[string]bool{}
	var walk func(key string)
	walk = func(key string) {
		if visited[key] {
			return
		}
		visited[key] = true
		c := v.chunks[key]
		for _, imp := range c.Imports {
			walk(imp)
		}
		for _, css := range c.CSS {
			url := joinURL(base, css)
			if !emitted[url] {
				emitted[url] = true
				styles = append(styles, url)
			}
		}
	}
	walk(name)
	return styles
}
