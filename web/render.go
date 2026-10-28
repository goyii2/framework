package web

import (
	"errors"
	"fmt"

	"github.com/a-h/templ"
	"github.com/goyii2/framework/view"
)

// SetTitle sets the page title layouts render into the document head.
// Views contribute their title from the action before rendering:
//
//	c.SetTitle("Post #7")
//	return c.Render("post/view", data)
func (c *Context) SetTitle(title string) {
	c.title = title
}

// SetLayout overrides the page frame for the next render. The frame
// composes per scope, widest first: the built-in view.Shell when nothing
// overrides, a module's frame when its routes mount through
// ModuleRoutes.Layout, and this call inside an action for one page. A
// controller can set a group-wide frame by returning layout-setting
// middleware from its Middleware method.
func (c *Context) SetLayout(l view.Layout) {
	c.layout = l
	c.bare = false
}

// ClearLayout renders the next output as bare content with no frame at all
// (partial content for fragments and embedding into other documents).
func (c *Context) ClearLayout() {
	c.bare = true
	c.layout = nil
}

// Render resolves the named view in the handler's registry, builds it with
// data and writes the framed page as an HTML response. The registry lookup
// is a map hit — data flows to the factory the registration site provided,
// typically a typed constructor adapted with view.Typed. Unknown names are
// programming mistakes and return an error for the unified error handler.
func (c *Context) Render(name string, data any) error {
	if c.views == nil {
		return errors.New("web: no view registry: render outside the application handler")
	}
	factory, ok := c.views.Lookup(name)
	if !ok {
		return fmt.Errorf("web: view %q is not registered", name)
	}
	return c.renderComponent(factory(data))
}

// RenderComponent writes comp as an HTML response, framed by the active
// layout like a named view — the direct-component counterpart of Render for
// actions that build their component inline.
func (c *Context) RenderComponent(comp templ.Component) error {
	return c.renderComponent(comp)
}

// frame wraps comp in the active layout: the page-level override when set,
// the built-in shell otherwise; bare output skips framing.
func (c *Context) frame(comp templ.Component) templ.Component {
	if c.bare {
		return comp
	}
	page := view.Page{Title: c.title, Content: comp}
	if c.layout != nil {
		return c.layout(page)
	}
	return view.Shell(page)
}

// renderComponent frames and writes the component as an HTML response.
func (c *Context) renderComponent(comp templ.Component) error {
	if comp == nil {
		return errors.New("web: view produced a nil component")
	}
	c.w.Header().Set("Content-Type", "text/html; charset=utf-8")
	return c.frame(comp).Render(c.r.Context(), c.w)
}

// writeHTML writes comp as an HTML response carrying an explicit status
// code (the error pages' path; normal renders use the implicit 200).
func (c *Context) writeHTML(comp templ.Component, status int) error {
	c.w.Header().Set("Content-Type", "text/html; charset=utf-8")
	c.w.WriteHeader(status)
	return comp.Render(c.r.Context(), c.w)
}
