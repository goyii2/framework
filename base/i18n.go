package base

import (
	"github.com/goyii2/framework/i18n"
)

// I18n returns the internationalization component — the message source
// translation requests resolve through. The fallback is an empty source:
// every message renders in the language it is written in. Applications with
// translations install their source (an embedded filesystem, a directory)
// with SetI18n before serving.
func (a *Application) I18n() *i18n.MessageSource { return a.i18n }

// SetI18n replaces the internationalization component (e.g. a source backed
// by an embedded message filesystem); nil restores the empty fallback.
func (a *Application) SetI18n(m *i18n.MessageSource) {
	if m == nil {
		m = newDefaultI18n()
	}
	a.i18n = m
}

// newDefaultI18n builds the fallback message source: no translation trees,
// so every message resolves to its id. Construction cannot fail.
func newDefaultI18n() *i18n.MessageSource {
	return i18n.NewMessageSource(nil)
}
