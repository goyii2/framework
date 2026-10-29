package i18n

import (
	"context"
	"sync"
	"testing"
	"testing/fstest"
)

func ctxFor(lang string) context.Context {
	return WithLanguage(WithSource(context.Background(), NewMessageSource(testMessages())), lang)
}

func filesFS(files map[string]string) fstest.MapFS {
	out := fstest.MapFS{}
	for path, data := range files {
		out[path] = &fstest.MapFile{Data: []byte(data)}
	}
	return out
}

func TestTTranslatesAndSubstitutesParams(t *testing.T) {
	got := T(ctxFor("zh-CN"), "app", "Hello, {name}!", map[string]any{"name": "Ada"})
	if got != "你好，Ada！" {
		t.Errorf("T = %q, want 你好，Ada！", got)
	}
}

func TestTWithoutSourceReturnsIDWithParams(t *testing.T) {
	got := T(context.Background(), "app", "Hello, {name}!", map[string]any{"name": "Ada"})
	if got != "Hello, Ada!" {
		t.Errorf("T without source = %q, want the id with the param substituted", got)
	}
}

func TestTMissFallsBackToID(t *testing.T) {
	got := T(ctxFor("zh-CN"), "app", "No translation for this one.", nil)
	if got != "No translation for this one." {
		t.Errorf("T on miss = %q, want the id verbatim", got)
	}
}

func TestTUnknownLanguageFallsBackToDefault(t *testing.T) {
	// fr carries no translations; the display default (en-US, the source
	// language) renders the id itself.
	got := T(ctxFor("fr-FR"), "app", "Hello, {name}!", map[string]any{"name": "Ada"})
	if got != "Hello, Ada!" {
		t.Errorf("T for an untranslated language = %q, want the source text", got)
	}
}

func TestTWalksParentLanguageChain(t *testing.T) {
	// The id exists only under zh; a zh-CN request resolves down to it.
	got := T(ctxFor("zh-CN"), "app", "Only in the zh parent, second.", nil)
	if got != "只在 zh 目录，第二条。" {
		t.Errorf("T down the chain = %q, want the zh parent entry", got)
	}
	// The child directory wins when it carries the id itself.
	got = T(ctxFor("zh-CN"), "app", "Only in the zh parent.", nil)
	if got != "只在 zh-CN 目录。" {
		t.Errorf("T with both levels carrying the id = %q, want the zh-CN entry", got)
	}
}

func TestPluralFormsGerman(t *testing.T) {
	src := NewMessageSource(filesFS(map[string]string{
		"de-DE/shop.json": `{"You have {n} apple.": {"one": "Sie haben {n} Apfel.", "other": "Sie haben {n} Äpfel."}}`,
	}))
	ctx := WithLanguage(WithSource(context.Background(), src), "de-DE")
	if got := T(ctx, "shop", "You have {n} apple.", map[string]any{"n": 1}); got != "Sie haben 1 Apfel." {
		t.Errorf("plural n=1 = %q, want the one form", got)
	}
	if got := T(ctx, "shop", "You have {n} apple.", map[string]any{"n": 5}); got != "Sie haben 5 Äpfel." {
		t.Errorf("plural n=5 = %q, want the other form", got)
	}
	// Without the driving param the "other" form renders.
	if got := T(ctx, "shop", "You have {n} apple.", nil); got != "Sie haben {n} Äpfel." {
		t.Errorf("plural without n = %q, want the other form with the raw placeholder", got)
	}
}

func TestPluralFormsChinese(t *testing.T) {
	// Chinese cardinal rules know only "other"; even n=1 renders it.
	ctx := ctxFor("zh-CN")
	if got := T(ctx, "shop", "You have {n} apple.", map[string]any{"n": 1}); got != "您有 1 个苹果。" {
		t.Errorf("zh plural n=1 = %q, want the other form with n substituted", got)
	}
	if got := T(ctx, "shop", "You have {n} apple.", map[string]any{"n": 5}); got != "您有 5 个苹果。" {
		t.Errorf("zh plural n=5 = %q, want the other form", got)
	}
}

func TestNumbersLocalizeInsideMessages(t *testing.T) {
	src := NewMessageSource(filesFS(map[string]string{
		"de-DE/shop.json": `{"The total is {amount}.": "Summe: {amount}."}`,
	}))
	ctx := WithLanguage(WithSource(context.Background(), src), "de-DE")
	if got := T(ctx, "shop", "The total is {amount}.", map[string]any{"amount": 1234.5}); got != "Summe: 1.234,5." {
		t.Errorf("localized number in message = %q, want German decimal separators", got)
	}
	// Without a source the substitution still localizes per the language.
	bare := WithLanguage(context.Background(), "de-DE")
	if got := T(bare, "shop", "The total is {amount}.", map[string]any{"amount": 1234.5}); got != "The total is 1.234,5." {
		t.Errorf("bare-context number = %q, want German separators on the raw text", got)
	}
}

// TestConcurrentTranslationAndLanguageSwitching exercises the read paths
// under concurrent use: one shared source, per-goroutine languages and
// parameters. Run with -race to catch any hidden mutation.
func TestConcurrentTranslationAndLanguageSwitching(t *testing.T) {
	src := NewMessageSource(testMessages())
	base := WithSource(context.Background(), src)
	langs := []string{"zh-CN", "zh", "de-DE", "en-US", "fr-FR", ""}
	want := map[string]string{
		"zh-CN": "你好，Ada！",
		"zh":    "Hello, Ada!", // the zh directory exists but carries no hello entry; the chain falls to the id
		"de-DE": "Hallo, Ada！",
		"en-US": "Hello, Ada!",
		"fr-FR": "Hello, Ada!",
		"":      "Hello, Ada!",
	}
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			lang := langs[i%len(langs)]
			ctx := WithLanguage(base, lang)
			for j := 0; j < 200; j++ {
				got := T(ctx, "app", "Hello, {name}!", map[string]any{"name": "Ada"})
				if got != want[lang] {
					t.Errorf("T(%q) = %q, want %q", lang, got, want[lang])
					return
				}
				src.Negotiate(lang, "en")
				F(ctx).Language()
			}
		}(i)
	}
	wg.Wait()
}

func TestLanguageAndSourceContextHelpers(t *testing.T) {
	ctx := context.Background()
	if _, ok := LanguageFromContext(ctx); ok {
		t.Error("LanguageFromContext on a bare context reported a language")
	}
	if _, ok := SourceFromContext(ctx); ok {
		t.Error("SourceFromContext on a bare context reported a source")
	}
	ctx = WithLanguage(WithSource(ctx, nil), "")
	if _, ok := LanguageFromContext(ctx); ok {
		t.Error("empty language reported present")
	}
	if _, ok := SourceFromContext(ctx); ok {
		t.Error("nil source reported present")
	}
	src := NewMessageSource(nil)
	if _, ok := SourceFromContext(WithSource(context.Background(), src)); !ok {
		t.Error("installed source not reported")
	}
}
