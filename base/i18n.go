package base

import (
	"os"

	"github.com/goyii2/framework/config"
	"github.com/goyii2/framework/i18n"
)

// I18n returns the internationalization component — the message source
// translation requests resolve through — building it from the configuration
// on first access. The default source loads <lang>/<category>.json trees
// from the configured messages directory (an absent directory means no
// translations and every message renders in the source language);
// applications embedding their translations install the embedded filesystem
// with SetI18n before serving.
func (a *Application) I18n() *i18n.MessageSource { return a.i18nComp.get() }

// SetI18n replaces the internationalization component (e.g. a source backed
// by an embedded message filesystem); nil restores lazy initialization from
// the configuration.
func (a *Application) SetI18n(m *i18n.MessageSource) { a.i18nComp.set(m) }

// newDefaultI18n builds the default message source from the application and
// i18n configuration sections. Construction cannot fail: a missing or
// unreadable messages directory produces an empty source, and every
// translation then falls back to the message id.
func newDefaultI18n(cfg config.Config) *i18n.MessageSource {
	return i18n.NewMessageSource(os.DirFS(cfg.I18n.MessagesDir),
		i18n.WithSourceLanguage(cfg.App.Language),
		i18n.WithDefaultLanguage(cfg.I18n.Language),
	)
}
