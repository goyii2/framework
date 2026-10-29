package i18n

import (
	"io"
	"log/slog"
	"slices"
	"testing"
	"testing/fstest"
)

// testMessages is the canonical fixture: one language with two categories,
// a parent-language directory, and a second full locale.
func testMessages() fstest.MapFS {
	return fstest.MapFS{
		"zh-CN/app.json": &fstest.MapFile{Data: []byte(`{
			"Hello, {name}!": "你好，{name}！",
			"Only in the zh parent.": "只在 zh-CN 目录。"
		}`)},
		"zh-CN/shop.json": &fstest.MapFile{Data: []byte(`{
			"You have {n} apple.": {"one": "您有一个苹果。", "other": "您有 {n} 个苹果。"}
		}`)},
		"zh/app.json": &fstest.MapFile{Data: []byte(`{
			"Only in the zh parent.": "只在 zh 目录。",
			"Only in the zh parent, second.": "只在 zh 目录，第二条。"
		}`)},
		"de-DE/app.json": &fstest.MapFile{Data: []byte(`{
			"Hello, {name}!": "Hallo, {name}！"
		}`)},
	}
}

func TestNewMessageSourceLoadsLanguagesAndCategories(t *testing.T) {
	src := NewMessageSource(testMessages())
	want := []string{"de-DE", "zh", "zh-CN"}
	if got := src.Languages(); !slices.Equal(got, want) {
		t.Errorf("Languages() = %v, want %v", got, want)
	}
	if src.SourceLanguage() != "en-US" || src.DefaultLanguage() != "en-US" {
		t.Errorf("defaults: source = %q, default = %q, want en-US for both",
			src.SourceLanguage(), src.DefaultLanguage())
	}
}

func TestNewMessageSourceAcceptsEmbeddedPrefix(t *testing.T) {
	src := NewMessageSource(fstest.MapFS{
		"messages/zh-CN/app.json": &fstest.MapFile{Data: []byte(`{"a": "b"}`)},
	})
	if got := src.Languages(); !slices.Equal(got, []string{"zh-CN"}) {
		t.Errorf("Languages() = %v, want [zh-CN] from the messages/ prefix", got)
	}
}

func TestNewMessageSourceWithoutFilesIsEmpty(t *testing.T) {
	src := NewMessageSource(nil)
	if langs := src.Languages(); len(langs) != 0 {
		t.Errorf("Languages() = %v, want none", langs)
	}
	if got := src.Translate("zh-CN", "app", "Hello", nil); got != "Hello" {
		t.Errorf("Translate on an empty source = %q, want the id", got)
	}
}

func TestNewMessageSourceSkipsBrokenEntries(t *testing.T) {
	// Silence the skip warnings; they are expected here.
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	src := NewMessageSource(fstest.MapFS{
		"zh-CN/app.json":      &fstest.MapFile{Data: []byte(`{"ok": "好"}`)},
		"notlang/app.json":    &fstest.MapFile{Data: []byte(`{"x": "y"}`)},
		"zh-CN/broken.json":   &fstest.MapFile{Data: []byte(`{not json`)},
		"zh-CN/top.json":      &fstest.MapFile{Data: []byte(`{"x": "y"}`), Mode: 0}, // unreadable? MapFS ignores mode
		"zh-CN/nested/d.json": &fstest.MapFile{Data: []byte(`{}`)},
		"README.md":           &fstest.MapFile{Data: []byte(`notes`)},
	})
	if got := src.Languages(); !slices.Equal(got, []string{"zh-CN"}) {
		t.Fatalf("Languages() = %v, want only zh-CN", got)
	}
	if got := src.Translate("zh-CN", "app", "ok", nil); got != "好" {
		t.Errorf("Translate(ok) = %q, want the good entry to survive the skips", got)
	}
}

func TestLanguageOptions(t *testing.T) {
	src := NewMessageSource(nil, WithSourceLanguage("de-DE"))
	if src.SourceLanguage() != "de-DE" {
		t.Errorf("SourceLanguage() = %q, want de-DE", src.SourceLanguage())
	}
	// Without an explicit default the display language follows the source.
	if src.DefaultLanguage() != "de-DE" {
		t.Errorf("DefaultLanguage() = %q, want the source language", src.DefaultLanguage())
	}
	src = NewMessageSource(nil, WithSourceLanguage("de-DE"), WithDefaultLanguage("zh-CN"))
	if src.DefaultLanguage() != "zh-CN" {
		t.Errorf("DefaultLanguage() = %q, want the explicit zh-CN", src.DefaultLanguage())
	}
}
