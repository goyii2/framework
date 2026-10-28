package view

import (
	"fmt"

	"github.com/a-h/templ"
)

// DetailRow is one attribute row of a DetailView: a heading label and a
// value. A templ.Component value renders inline; nil renders as an empty
// cell; anything else renders as its fmt.Sprint form. The template escapes
// all text, so model values can be interpolated as-is.
type DetailRow struct {
	Label string
	Value any
}

// DetailView renders the attribute table of one model instance (Yii2's
// detail view). Configure as a struct and call Render to obtain the
// component.
type DetailView struct {
	Rows []DetailRow
}

// detailCell is the precomputed cell the template renders.
type detailCell struct {
	label     string
	text      string
	component templ.Component
}

func (d DetailView) cells() []detailCell {
	cells := make([]detailCell, 0, len(d.Rows))
	for _, row := range d.Rows {
		cell := detailCell{label: row.Label}
		switch v := row.Value.(type) {
		case nil:
			// empty cell
		case templ.Component:
			cell.component = v
		default:
			cell.text = fmt.Sprint(v)
		}
		cells = append(cells, cell)
	}
	return cells
}

// Render returns the detail table component.
func (d DetailView) Render() templ.Component { return detailView(d.cells()) }
