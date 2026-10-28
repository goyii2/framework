package helpers

import (
	"crypto/rand"
	"strings"
	"unicode"
)

// strAlphabet is the character pool of StrRandom, matching Yii2's
// StringHelper::random default: letters and digits ("alnum").
const strAlphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// StrRandom returns a cryptographically random string of length n over
// letters and digits, the shape Yii2's StringHelper::random produces for
// tokens and passwords. It draws from crypto/rand: the output is meant for
// secrets, so the generator must be unpredictable. A failed read from the
// system entropy source panics — the process cannot safely manufacture
// "random" values, and callers have no good way to degrade.
func StrRandom(n int) string {
	if n <= 0 {
		return ""
	}
	buf := make([]byte, n)
	// Read fills the whole buffer or fails; with rejection sampling below the
	// bytes are consumed as indexes into strAlphabet.
	if _, err := rand.Read(buf); err != nil {
		panic("helpers: system entropy source failed: " + err.Error())
	}
	// 256 % 62 != 0, so taking buf[i] % 62 would favor the leading characters
	// of the alphabet; reject the biased tail instead. The bound is the
	// largest multiple of the alphabet size below 256.
	bound := byte(256 - (256 % len(strAlphabet)))
	out := make([]byte, 0, n)
	for len(out) < n {
		for _, b := range buf {
			if b >= bound {
				continue
			}
			out = append(out, strAlphabet[b%byte(len(strAlphabet))])
			if len(out) == n {
				break
			}
		}
		if len(out) < n {
			if _, err := rand.Read(buf); err != nil {
				panic("helpers: system entropy source failed: " + err.Error())
			}
		}
	}
	return string(out)
}

// StrCamel converts a "hello_world" / "hello-world" / "hello world" style
// identifier to camelCase ("helloWorld"), corresponding to Yii2's
// Inflector::camelize: separators start a new capitalized word, everything
// else — including capitals already present — is preserved, and the first
// character of the whole result is lowercased.
func StrCamel(s string) string {
	words := splitWords(s)
	if len(words) == 0 {
		return ""
	}
	var b strings.Builder
	for i, w := range words {
		if i == 0 {
			b.WriteString(lowerFirst(w))
			continue
		}
		b.WriteString(upperFirst(w))
	}
	return b.String()
}

// StrSnake converts an identifier to snake_case ("hello_world"),
// corresponding to Yii2's Inflector::underscore with better acronym
// handling: a boundary is inserted before a capital that follows a lowercase
// letter or digit ("helloWorld" -> "hello_world"), and between a capital run
// and a trailing word ("XMLParser" -> "xml_parser"), so consecutive capitals
// do not each get their own underscore. Existing '_', '-' and whitespace
// separators collapse to a single underscore and the result is lowercased.
func StrSnake(s string) string {
	runes := []rune(s)
	var b strings.Builder
	pendingSep := false // an unwritten '_' waits for the next real character
	prevUpper := false  // previous written character was an uppercase letter
	for i, r := range runes {
		if r == '_' || r == '-' || unicode.IsSpace(r) {
			if b.Len() > 0 {
				pendingSep = true
			}
			prevUpper = false
			continue
		}
		upper := unicode.IsUpper(r)
		if upper && b.Len() > 0 {
			lowerBefore := !prevUpper                                                   // "helloWorld": word boundary after lowercase
			acronymEnds := prevUpper && i+1 < len(runes) && unicode.IsLower(runes[i+1]) // "XMLParser": last capital of the run
			if lowerBefore || acronymEnds {
				pendingSep = true
			}
		}
		if pendingSep && b.Len() > 0 {
			b.WriteByte('_')
			pendingSep = false
		}
		b.WriteRune(unicode.ToLower(r))
		prevUpper = upper
	}
	return b.String()
}

// splitWords cuts s into words at '_', '-' and whitespace runs. Capitals are
// left in place: StrCamel, like Yii2's camelize, preserves the casing of
// what it is given and only capitalizes after separators.
func splitWords(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool {
		return r == '_' || r == '-' || unicode.IsSpace(r)
	})
}

// StrUpperFirst uppercases the first rune of s ("hello" -> "Hello"),
// corresponding to Yii2's ucfirst; an empty string is returned unchanged.
func StrUpperFirst(s string) string {
	return upperFirst(s)
}

// StrLowerFirst lowercases the first rune of s ("Hello" -> "hello"),
// corresponding to Yii2's lcfirst; an empty string is returned unchanged.
func StrLowerFirst(s string) string {
	return lowerFirst(s)
}

func upperFirst(s string) string {
	if s == "" {
		return ""
	}
	r := []rune(s)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

func lowerFirst(s string) string {
	if s == "" {
		return ""
	}
	r := []rune(s)
	r[0] = unicode.ToLower(r[0])
	return string(r)
}

// StrTruncate shortens s to at most length runes, counting the suffix inside
// that budget, corresponding to Yii2's StringHelper::truncate: text at or
// under the limit is returned untouched (no suffix appended), text past it is
// cut to make room for suffix, trailing whitespace at the cut is trimmed,
// then the suffix is appended. A length that cannot even fit the suffix
// yields just the suffix.
func StrTruncate(s string, length int, suffix string) string {
	if length < 0 {
		length = 0
	}
	runes := []rune(s)
	if len(runes) <= length {
		return s
	}
	cut := length - len([]rune(suffix))
	if cut <= 0 {
		return suffix
	}
	return strings.TrimRight(string(runes[:cut]), " \t\n\r") + suffix
}
