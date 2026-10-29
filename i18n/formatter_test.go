package i18n

import (
	"context"
	"testing"
	"time"
)

var refDate = time.Date(2006, time.January, 2, 15, 4, 5, 0, time.UTC)

func TestFormatterNumber(t *testing.T) {
	cases := map[string]string{
		"en-US": "1,234.5",
		"zh-CN": "1,234.5",
		"de-DE": "1.234,5",
	}
	for lang, want := range cases {
		if got := NewFormatter(lang).Number(1234.5); got != want {
			t.Errorf("Number(%s) = %q, want %q", lang, got, want)
		}
	}
}

func TestFormatterCurrency(t *testing.T) {
	cases := []struct {
		lang, iso, want string
	}{
		{"en-US", "USD", "$ 1,234.50"},
		{"zh-CN", "USD", "US$ 1,234.50"},
		{"zh-CN", "CNY", "￥ 1,234.50"},
		{"de-DE", "EUR", "€ 1.234,50"},
		{"en-US", "ZZZ", "ZZZ 1,234.5"}, // unknown code falls back to the raw prefix
	}
	for _, tc := range cases {
		if got := NewFormatter(tc.lang).Currency(tc.iso, 1234.5); got != tc.want {
			t.Errorf("Currency(%s, %s) = %q, want %q", tc.lang, tc.iso, got, tc.want)
		}
	}
}

func TestFormatterDates(t *testing.T) {
	cases := []struct {
		lang                                       string
		date, dateShort, dateLong, time_, dateTime string
	}{
		{"en-US", "Jan 2, 2006", "1/2/06", "January 2, 2006", "3:04 PM", "Jan 2, 2006 3:04 PM"},
		{"en-GB", "Jan 2, 2006", "1/2/06", "January 2, 2006", "3:04 PM", "Jan 2, 2006 3:04 PM"},
		{"zh-CN", "2006年1月2日", "2006/1/2", "2006年1月2日", "15:04", "2006年1月2日 15:04"},
		{"zh-TW", "2006年1月2日", "2006/1/2", "2006年1月2日", "15:04", "2006年1月2日 15:04"},
		{"ja-JP", "2006/01/02", "2006/01/02", "2006年1月2日", "15:04", "2006/01/02 15:04"},
		{"de-DE", "2. Jan. 2006", "02.01.06", "2. Januar 2006", "15:04", "2. Jan. 2006 15:04"},
		{"fr-FR", "2 janv. 2006", "02/01/2006", "2 janvier 2006", "15:04", "2 janv. 2006 15:04"},
		{"ru-RU", "Jan 2, 2006", "1/2/06", "January 2, 2006", "3:04 PM", "Jan 2, 2006 3:04 PM"}, // no table entry: English fallback
	}
	for _, tc := range cases {
		f := NewFormatter(tc.lang)
		if got := f.Date(refDate); got != tc.date {
			t.Errorf("Date(%s) = %q, want %q", tc.lang, got, tc.date)
		}
		if got := f.DateShort(refDate); got != tc.dateShort {
			t.Errorf("DateShort(%s) = %q, want %q", tc.lang, got, tc.dateShort)
		}
		if got := f.DateLong(refDate); got != tc.dateLong {
			t.Errorf("DateLong(%s) = %q, want %q", tc.lang, got, tc.dateLong)
		}
		if got := f.Time(refDate); got != tc.time_ {
			t.Errorf("Time(%s) = %q, want %q", tc.lang, got, tc.time_)
		}
		if got := f.DateTime(refDate); got != tc.dateTime {
			t.Errorf("DateTime(%s) = %q, want %q", tc.lang, got, tc.dateTime)
		}
	}
}

func TestFResolvesLanguageFromContext(t *testing.T) {
	if got := F(context.Background()).Number(1234.5); got != "1,234.5" {
		t.Errorf("F on a bare context = %q, want en-US formatting", got)
	}
	ctx := WithLanguage(context.Background(), "de-DE")
	if got := F(ctx).Number(1234.5); got != "1.234,5" {
		t.Errorf("F(de-DE) = %q, want German formatting", got)
	}
	// Without a request language the source's default applies.
	src := NewMessageSource(nil, WithDefaultLanguage("zh-CN"))
	if got := F(WithSource(context.Background(), src)).Date(refDate); got != "2006年1月2日" {
		t.Errorf("F with the source default = %q, want the zh date", got)
	}
}

func TestLocalizeMonthsGuardsWordBoundaries(t *testing.T) {
	// "Januar" must survive the short-name pass looking for "Jan".
	text := "2. Januar 2006"
	got := localizeMonths(text, layoutTable["de"].months)
	if got != text {
		t.Errorf("localizeMonths(%q) = %q, want unchanged", text, got)
	}
}
