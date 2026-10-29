package i18n

import (
	"context"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/currency"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
	"golang.org/x/text/number"
)

// Formatter renders values by one language's conventions: decimal separators
// and digit grouping for numbers, currency symbols and rounding for money,
// familiar date and time shapes for timestamps. Obtain the request's
// formatter with F; build one for a fixed language with NewFormatter.
type Formatter struct {
	tag     language.Tag
	printer *message.Printer
}

// NewFormatter builds the formatter for lang ("en-US", "zh-CN", ...). An
// empty or unparseable language falls back to "en-US".
func NewFormatter(lang string) *Formatter {
	tag := parseOrDefault(lang, language.MustParse("en-US"))
	return &Formatter{tag: tag, printer: message.NewPrinter(tag)}
}

// F returns the formatter for the language resolved from ctx: the request's
// language first, then the message source's default, then "en-US".
func F(ctx context.Context) *Formatter {
	if ctx != nil {
		if lang, ok := LanguageFromContext(ctx); ok {
			return NewFormatter(lang)
		}
		if src, ok := SourceFromContext(ctx); ok {
			return NewFormatter(src.DefaultLanguage())
		}
	}
	return NewFormatter("en-US")
}

// Language returns the formatter's language tag.
func (f *Formatter) Language() string { return f.tag.String() }

// Number renders v with the language's decimal separator and digit grouping
// ("1,234.5" in English, "1.234,5" in German).
func (f *Formatter) Number(v any) string {
	return f.printer.Sprint(number.Decimal(v))
}

// Currency renders v as an amount of the ISO 4217 currency iso, with the
// language's currency symbol and the currency's standard fraction digits
// ("$ 1,234.50" in English for USD). An unrecognized code prefixes the amount
// verbatim.
func (f *Formatter) Currency(iso string, v any) string {
	unit, err := currency.ParseISO(iso)
	if err != nil {
		return iso + " " + f.Number(v)
	}
	return f.printer.Sprint(currency.Symbol(unit.Amount(v)))
}

// Date renders t in the language's medium date shape ("Jan 2, 2006" in
// English, "2006年1月2日" in Chinese).
func (f *Formatter) Date(t time.Time) string { return f.formatDate(t, layouts(f.tag).date) }

// DateShort renders t in the language's short, numeric-first shape
// ("1/2/06" in English, "2006/1/2" in Chinese).
func (f *Formatter) DateShort(t time.Time) string {
	return f.formatDate(t, layouts(f.tag).dateShort)
}

// DateLong renders t in the language's long, fully written-out shape
// ("January 2, 2006" in English).
func (f *Formatter) DateLong(t time.Time) string {
	return f.formatDate(t, layouts(f.tag).dateLong)
}

// Time renders t in the language's medium time shape ("3:04 PM" in English,
// "15:04" in most others).
func (f *Formatter) Time(t time.Time) string { return t.Format(layouts(f.tag).time) }

// DateTime renders t as the language's medium date followed by its medium
// time.
func (f *Formatter) DateTime(t time.Time) string {
	l := layouts(f.tag)
	return f.formatDate(t, l.date) + " " + t.Format(l.time)
}

// formatDate applies one date layout and localizes month names when the
// language needs it (Go's time package renders month names in English only).
func (f *Formatter) formatDate(t time.Time, layout string) string {
	return localizeMonths(t.Format(layout), layouts(f.tag).months)
}

// dateLayouts bundles one language's date and time shapes. Layout strings
// use Go's reference date; months carries the language's month names when
// its layouts contain name references, nil when months render as English or
// numerals.
type dateLayouts struct {
	date      string
	dateShort string
	dateLong  string
	time      string
	months    *monthTable
}

// monthTable maps Go's English month names onto the language's; index 0 is
// January.
type monthTable struct {
	long  []string
	short []string
}

var englishLongMonths = [...]string{
	"January", "February", "March", "April", "May", "June",
	"July", "August", "September", "October", "November", "December",
}
var englishShortMonths = [...]string{
	"Jan", "Feb", "Mar", "Apr", "May", "Jun",
	"Jul", "Aug", "Sep", "Oct", "Nov", "Dec",
}

var layoutTable = map[string]dateLayouts{
	"en": {
		date:      "Jan 2, 2006",
		dateShort: "1/2/06",
		dateLong:  "January 2, 2006",
		time:      "3:04 PM",
	},
	"zh": {
		date:      "2006年1月2日",
		dateShort: "2006/1/2",
		dateLong:  "2006年1月2日",
		time:      "15:04",
	},
	"ja": {
		date:      "2006/01/02",
		dateShort: "2006/01/02",
		dateLong:  "2006年1月2日",
		time:      "15:04",
	},
	"ko": {
		date:      "2006년 1월 2일",
		dateShort: "2006. 1. 2.",
		dateLong:  "2006년 1월 2일",
		time:      "15:04",
	},
	"de": {
		date:      "2. Jan 2006",
		dateShort: "02.01.06",
		dateLong:  "2. January 2006",
		time:      "15:04",
		months: &monthTable{
			long: []string{
				"Januar", "Februar", "März", "April", "Mai", "Juni",
				"Juli", "August", "September", "Oktober", "November", "Dezember",
			},
			short: []string{
				"Jan.", "Feb.", "März", "Apr.", "Mai", "Juni",
				"Juli", "Aug.", "Sept.", "Okt.", "Nov.", "Dez.",
			},
		},
	},
	"fr": {
		date:      "2 Jan 2006",
		dateShort: "02/01/2006",
		dateLong:  "2 January 2006",
		time:      "15:04",
		months: &monthTable{
			long: []string{
				"janvier", "février", "mars", "avril", "mai", "juin",
				"juillet", "août", "septembre", "octobre", "novembre", "décembre",
			},
			short: []string{
				"janv.", "févr.", "mars", "avr.", "mai", "juin",
				"juil.", "août", "sept.", "oct.", "nov.", "déc.",
			},
		},
	},
}

// layouts resolves the date shapes for tag: the tag itself, then its parent
// languages, then its base language (the parent chain of zh-TW passes
// through zh-Hant without ever reaching "zh"), then English as the
// fallback.
func layouts(tag language.Tag) dateLayouts {
	for t := tag; ; t = t.Parent() {
		if l, ok := layoutTable[t.String()]; ok {
			return l
		}
		if t.Parent() == t {
			break
		}
	}
	if base, conf := tag.Base(); conf != language.No {
		if l, ok := layoutTable[base.String()]; ok {
			return l
		}
	}
	return layoutTable["en"]
}

// localizeMonths replaces standalone English month names in text with the
// table's names. The format engine emits months as standalone words, so a
// word-boundary guard keeps "Jan" from matching inside an already-localized
// "Januar".
func localizeMonths(text string, months *monthTable) string {
	if months == nil {
		return text
	}
	for i, name := range englishLongMonths {
		text = replaceWholeWord(text, name, months.long[i])
	}
	for i, name := range englishShortMonths {
		text = replaceWholeWord(text, name, months.short[i])
	}
	return text
}

// replaceWholeWord replaces every standalone occurrence of from in s with
// to; occurrences with a letter on either side are part of another word and
// stay untouched.
func replaceWholeWord(s, from, to string) string {
	var b strings.Builder
	for {
		idx := strings.Index(s, from)
		if idx < 0 {
			b.WriteString(s)
			return b.String()
		}
		end := idx + len(from)
		if wordBoundary(s, idx, end) {
			b.WriteString(s[:idx])
			b.WriteString(to)
			s = s[end:]
			continue
		}
		// Keep this occurrence and resume scanning after its first rune so
		// overlapping candidates are still found.
		_, width := utf8.DecodeRuneInString(s[idx:])
		b.WriteString(s[:idx+width])
		s = s[idx+width:]
	}
}

// wordBoundary reports whether s[before:after] is delimited by non-letters
// (string edges count).
func wordBoundary(s string, before, after int) bool {
	if before > 0 {
		if r, _ := utf8.DecodeLastRuneInString(s[:before]); unicode.IsLetter(r) {
			return false
		}
	}
	if after < len(s) {
		if r, _ := utf8.DecodeRuneInString(s[after:]); unicode.IsLetter(r) {
			return false
		}
	}
	return true
}
