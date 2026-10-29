package gii

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

// The built-in templates, embedded so the generator works with no external
// files. A --templates directory overrides any of them by file name.
//
//go:embed templates
var templatesFS embed.FS

// Template file names inside the embedded templates directory — and the
// names a --templates override directory recognizes.
const (
	// ModelTemplate renders the Active Record model file.
	ModelTemplate = "model.go.tmpl"
	// ControllerTemplate renders the plain controller skeleton.
	ControllerTemplate = "controller.go.tmpl"
	// CrudControllerTemplate renders the CRUD controller file.
	CrudControllerTemplate = "crud_controller.go.tmpl"
	// CrudViewsTemplate renders the .templ view file of a CRUD set.
	CrudViewsTemplate = "crud_views.templ.tmpl"
)

// templateFuncs carries the helpers templates call; they wrap the naming
// rules of this package so overrides keep working against the same data.
var templateFuncs = template.FuncMap{
	"lowerName": lowerName,
}

// loadTemplate parses the named template: the same-named file from
// overrideDir when present, the embedded copy otherwise. A broken override
// fails here, before anything is written, instead of half-rendering a file.
func loadTemplate(name, overrideDir string) (*template.Template, error) {
	content, source := "", "embedded"
	if overrideDir != "" {
		path := filepath.Join(overrideDir, name)
		raw, err := os.ReadFile(path)
		if err == nil {
			content, source = string(raw), path
		} else if !os.IsNotExist(err) {
			return nil, fmt.Errorf("gii: read template %s: %w", path, err)
		}
	}
	if content == "" {
		raw, err := templatesFS.ReadFile("templates/" + name)
		if err != nil {
			return nil, fmt.Errorf("gii: embedded template %s: %w", name, err)
		}
		content = string(raw)
	}

	tmpl, err := template.New(name).Funcs(templateFuncs).Parse(content)
	if err != nil {
		// source still names the override path when one was used; the
		// embedded name otherwise.
		return nil, fmt.Errorf("gii: parse template %s: %w", source, err)
	}
	return tmpl, nil
}

// render executes tmpl over data and returns the output.
func render(tmpl *template.Template, data any) (string, error) {
	var sb strings.Builder
	if err := tmpl.Execute(&sb, data); err != nil {
		return "", fmt.Errorf("gii: render %s: %w", tmpl.Name(), err)
	}
	return sb.String(), nil
}
