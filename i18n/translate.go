package i18n

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"golang.org/x/text/feature/plural"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
	"golang.org/x/text/number"
)

// languageKey is the context key carrying the request's display language;
// sourceKey carries the MessageSource. Both are installed by the framework
// (the application context and the web language middleware); standalone code
// installs them with WithLanguage / WithSource.
type languageKey struct{}

type sourceKey struct{}

// WithLanguage returns a copy of ctx carrying lang as the display language.
func WithLanguage(ctx context.Context, lang string) context.Context {
	return context.WithValue(ctx, languageKey{}, lang)
}

// LanguageFromContext returns the display language carried by ctx; false
// when none was installed.
func LanguageFromContext(ctx context.Context) (string, bool) {
	lang, ok := ctx.Value(languageKey{}).(string)
	return lang, ok && lang != ""
}

// WithSource returns a copy of ctx carrying src as the message source.
func WithSource(ctx context.Context, src *MessageSource) context.Context {
	return context.WithValue(ctx, sourceKey{}, src)
}

// SourceFromContext returns the message source carried by ctx; false when
// none was installed (a CLI context, or code outside the framework
// pipeline).
func SourceFromContext(ctx context.Context) (*MessageSource, bool) {
	src, ok := ctx.Value(sourceKey{}).(*MessageSource)
	return src, ok && src != nil
}

// T translates msgID — a string written in the source language — into the
// display language resolved from ctx, looking it up under category in the
// message source. Named placeholders ({name}) in the translated text are
// replaced with params; numbers format per the display language. Without a
// message source in the context the id itself renders, placeholders still
// substituted.
func T(ctx context.Context, category, msgID string, params map[string]any) string {
	if ctx == nil {
		return renderMessage(language.Und, msgID, params)
	}
	lang, _ := LanguageFromContext(ctx)
	src, ok := SourceFromContext(ctx)
	if !ok {
		return renderMessage(parseOrDefault(lang, language.Und), msgID, params)
	}
	return src.Translate(lang, category, msgID, params)
}

// parseOrDefault parses lang, falling back to def when empty or invalid.
func parseOrDefault(lang string, def language.Tag) language.Tag {
	if lang == "" {
		return def
	}
	if tag, err := language.Parse(lang); err == nil {
		return tag
	}
	return def
}

// renderMessage substitutes {name} placeholders in text with params: numeric
// values render through the locale-aware decimal formatter, everything else
// through fmt.Sprint. Placeholders with no matching param stay verbatim.
func renderMessage(tag language.Tag, text string, params map[string]any) string {
	if len(params) == 0 || !strings.ContainsRune(text, '{') {
		return text
	}
	printer := message.NewPrinter(tag)
	for name, value := range params {
		placeholder := "{" + name + "}"
		if !strings.Contains(text, placeholder) {
			continue
		}
		text = strings.ReplaceAll(text, placeholder, formatValue(printer, value))
	}
	return text
}

// formatValue renders one placeholder value: numbers localized, the rest via
// the default formatting.
func formatValue(printer *message.Printer, value any) string {
	switch v := value.(type) {
	case int:
		return printer.Sprint(number.Decimal(v))
	case int8:
		return printer.Sprint(number.Decimal(int64(v)))
	case int16:
		return printer.Sprint(number.Decimal(int64(v)))
	case int32:
		return printer.Sprint(number.Decimal(int64(v)))
	case int64:
		return printer.Sprint(number.Decimal(v))
	case uint:
		return printer.Sprint(number.Decimal(uint64(v)))
	case uint8:
		return printer.Sprint(number.Decimal(uint64(v)))
	case uint16:
		return printer.Sprint(number.Decimal(uint64(v)))
	case uint32:
		return printer.Sprint(number.Decimal(uint64(v)))
	case uint64:
		return printer.Sprint(number.Decimal(v))
	case float32:
		return printer.Sprint(number.Decimal(v))
	case float64:
		return printer.Sprint(number.Decimal(v))
	default:
		return fmt.Sprint(value)
	}
}

// pluralCount extracts the count driving plural form selection: the param
// named "n". Whole numbers only; anything else reports false and the message
// renders its "other" form.
func pluralCount(params map[string]any) (int64, bool) {
	v, ok := params["n"]
	if !ok {
		return 0, false
	}
	switch n := v.(type) {
	case int:
		return int64(n), true
	case int8:
		return int64(n), true
	case int16:
		return int64(n), true
	case int32:
		return int64(n), true
	case int64:
		return n, true
	case uint:
		return int64(n), true
	case uint8:
		return int64(n), true
	case uint16:
		return int64(n), true
	case uint32:
		return int64(n), true
	case uint64:
		if n > 1<<63-1 {
			return 0, false
		}
		return int64(n), true
	case float64:
		if n == float64(int64(n)) {
			return int64(n), true
		}
		return 0, false
	case string:
		parsed, err := strconv.ParseInt(n, 10, 64)
		if err != nil {
			return 0, false
		}
		return parsed, true
	case time.Duration:
		return int64(n), true
	default:
		return 0, false
	}
}

// cardinalForm selects the CLDR cardinal plural category ("one", "other",
// ...) for count under the plural rules of tag.
func cardinalForm(tag language.Tag, count int64) string {
	switch plural.Cardinal.MatchPlural(tag, int(count), 0, 0, 0, 0) {
	case plural.Zero:
		return "zero"
	case plural.One:
		return "one"
	case plural.Two:
		return "two"
	case plural.Few:
		return "few"
	case plural.Many:
		return "many"
	default:
		return "other"
	}
}
