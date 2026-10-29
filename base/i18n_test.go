package base

import (
	"context"
	"testing"
	"testing/fstest"

	"github.com/goyii2/framework/config"
	"github.com/goyii2/framework/i18n"
)

func TestApplicationI18nComponent(t *testing.T) {
	app := New(config.Config{})
	if src := app.I18n(); src == nil {
		t.Fatal("I18n() = nil, want the lazily built default source")
	}
	// The default source loads from the configured directory; absent from
	// the test working directory, it carries no translations.
	if langs := app.I18n().Languages(); len(langs) != 0 {
		t.Errorf("default source languages = %v, want none without a messages directory", langs)
	}

	custom := i18n.NewMessageSource(fstest.MapFS{
		"zh-CN/app.json": &fstest.MapFile{Data: []byte(`{"Hello": "你好"}`)},
	})
	app.SetI18n(custom)
	if app.I18n() != custom {
		t.Error("SetI18n did not replace the component")
	}

	app.SetI18n(nil)
	if app.I18n() == custom {
		t.Error("SetI18n(nil) did not restore lazy initialization")
	}
}

// TestWithContextCarriesI18nSource proves the request context hands the i18n
// helpers their source: WithContext stashes it under i18n's own key.
func TestWithContextCarriesI18nSource(t *testing.T) {
	app := New(config.Config{})
	ctx := app.WithContext(context.Background())

	src, ok := i18n.SourceFromContext(ctx)
	if !ok || src == nil {
		t.Fatal("WithContext did not carry the i18n source")
	}
	if src != app.I18n() {
		t.Error("carried source differs from the application component")
	}
	if got := i18n.T(ctx, "app", "Hello", nil); got != "Hello" {
		t.Errorf("T through the app context = %q, want the id on an empty source", got)
	}

	// A replaced component is what the next WithContext hands out.
	custom := i18n.NewMessageSource(fstest.MapFS{
		"zh-CN/app.json": &fstest.MapFile{Data: []byte(`{"Hello": "你好"}`)},
	}, i18n.WithDefaultLanguage("zh-CN"))
	app.SetI18n(custom)
	if got := i18n.T(app.WithContext(context.Background()), "app", "Hello", nil); got != "你好" {
		t.Errorf("T after SetI18n = %q, want the replacement source's default language", got)
	}
}
