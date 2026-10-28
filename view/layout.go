package view

import (
	"context"
	"io"

	"github.com/a-h/templ"
)

// Page carries the slots a layout composes into the final document: the page
// title (set with web.Context.SetTitle), the content the action rendered and
// optional head / nav / footer components a page may contribute. Layouts
// render the slots they care about; unset components render as nothing.
type Page struct {
	Title   string
	Head    templ.Component
	Nav     templ.Component
	Content templ.Component
	Footer  templ.Component
}

// Layout frames action content into a full HTML page. Layouts are plain
// functions over Page, so composing one is a function call: an application
// writes its frame once (typically by wrapping Shell or replacing it), and a
// module or a single action can substitute its own without the frame being
// duplicated. See web.Context.SetLayout for the override levels.
type Layout func(p Page) templ.Component

// blank is the component widgets return when they hide themselves (LinkPager
// on a single page with HideOnSinglePage): it renders nothing, so callers
// can embed the result unconditionally.
type blank struct{}

func (blank) Render(_ context.Context, _ io.Writer) error { return nil }

// Blank returns a component that renders nothing.
func Blank() templ.Component { return blank{} }
