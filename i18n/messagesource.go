package i18n

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"sort"
	"strings"

	"golang.org/x/text/language"
)

// MessageSource loads translations from <lang>/<category>.json files and
// translates message ids — strings written in the source language — into the
// requested display language. Sources are immutable once built: all files are
// read eagerly at construction, so translation is a lock-free map walk safe
// for concurrent use.
//
// The file tree may be an embedded FS passed as-is (paths then read
// "messages/<lang>/<category>.json") or any FS rooted at the messages
// directory ("zh-CN/app.json"). A source built without a filesystem, or from
// a missing directory, simply has no translations: every lookup falls back to
// the message id, which is the source-language text.
type MessageSource struct {
	source   language.Tag // language the message ids are written in
	fallback language.Tag // display language when a request signals no preference

	// messages maps each available language (canonical tag string) to its
	// categories, each category to its id -> entry table.
	messages map[string]map[string]map[string]entry

	matcher language.Matcher
	// matched carries the canonical language key for every matcher index,
	// so negotiation and lookups report directory names rather than the
	// matcher's decorated tags (region preferences and the like).
	matched []string
}

// entry is one translated message: either a plain string or a set of plural
// forms keyed "zero" | "one" | "two" | "few" | "many" | "other".
type entry struct {
	text  string
	forms map[string]string
}

// Option configures a MessageSource at construction.
type Option func(*MessageSource)

// WithSourceLanguage sets the language the message ids are written in
// (default "en-US"). Translating into the source language is an identity
// operation: the id itself is the message.
func WithSourceLanguage(lang string) Option {
	return func(m *MessageSource) {
		if tag, err := language.Parse(lang); err == nil {
			m.source = tag
		}
	}
}

// WithDefaultLanguage sets the display language used when a request carries
// no preference (default: the source language).
func WithDefaultLanguage(lang string) Option {
	return func(m *MessageSource) {
		if tag, err := language.Parse(lang); err == nil {
			m.fallback = tag
		}
	}
}

// NewMessageSource builds a source from the .json files under messages. The
// FS may be the messages root itself or an embed.FS whose tree starts at a
// "messages" directory (the prefix is accepted transparently). Files that
// fail to parse are skipped with a warning rather than failing the whole
// application; a missing root means no translations at all.
func NewMessageSource(messages fs.FS, opts ...Option) *MessageSource {
	m := &MessageSource{
		source:   language.MustParse("en-US"),
		messages: map[string]map[string]map[string]entry{},
	}
	for _, opt := range opts {
		opt(m)
	}
	// Without an explicit default, display falls back to the source language.
	if m.fallback == (language.Tag{}) {
		m.fallback = m.source
	}
	if messages != nil {
		m.load(messages)
	}
	m.rebuildMatcher()
	return m
}

// load walks the file tree and fills m.messages. Only paths of the shape
// [messages/]<lang>/<category>.json contribute; everything else (READMEs,
// nested helper files) is ignored.
func (m *MessageSource) load(messages fs.FS) {
	_ = fs.WalkDir(messages, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// A missing or unreadable root means no translations; individual
			// unreadable entries are skipped and the walk continues.
			if path != "." {
				slog.Warn("i18n: skipping unreadable message path", "path", path, "err", err.Error())
			}
			return nil
		}
		if d.IsDir() || !strings.HasSuffix(path, ".json") {
			return nil
		}
		lang, category, ok := splitMessagePath(path)
		if !ok {
			return nil
		}
		tag, err := language.Parse(lang)
		if err != nil {
			slog.Warn("i18n: skipping message file under unrecognized language directory", "path", path)
			return nil
		}
		entries, err := readCategory(messages, path)
		if err != nil {
			slog.Warn("i18n: skipping unparsable message file", "path", path, "err", err.Error())
			return nil
		}
		bucket, ok := m.messages[tag.String()]
		if !ok {
			bucket = map[string]map[string]entry{}
			m.messages[tag.String()] = bucket
		}
		bucket[category] = entries
		return nil
	})
}

// splitMessagePath breaks one walked path into language and category.
func splitMessagePath(path string) (lang, category string, ok bool) {
	parts := strings.Split(strings.TrimPrefix(path, "messages/"), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], strings.TrimSuffix(parts[1], ".json"), true
}

// readCategory decodes one category file. Values are either plain strings or
// objects of plural forms.
func readCategory(messages fs.FS, path string) (map[string]entry, error) {
	data, err := fs.ReadFile(messages, path)
	if err != nil {
		return nil, err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	entries := make(map[string]entry, len(raw))
	for id, value := range raw {
		var text string
		if err := json.Unmarshal(value, &text); err == nil {
			entries[id] = entry{text: text}
			continue
		}
		var forms map[string]string
		if err := json.Unmarshal(value, &forms); err == nil && len(forms) > 0 {
			entries[id] = entry{forms: forms}
			continue
		}
		return nil, fmt.Errorf("message %q: value must be a string or an object of plural forms", id)
	}
	return entries, nil
}

// rebuildMatcher prepares the language matcher over the default plus every
// available translation language. The default comes first so requests no
// available language matches resolve to it (x/text matcher convention).
func (m *MessageSource) rebuildMatcher() {
	type supported struct {
		tag language.Tag
		key string
	}
	list := []supported{{m.fallback, m.fallback.String()}}
	for key := range m.messages {
		if key == m.fallback.String() {
			continue
		}
		tag, err := language.Parse(key)
		if err != nil {
			continue
		}
		list = append(list, supported{tag, key})
	}
	sort.Slice(list[1:], func(i, j int) bool { return list[1:][i].key < list[1:][j].key })
	tags := make([]language.Tag, len(list))
	m.matched = make([]string, len(list))
	for i, s := range list {
		tags[i] = s.tag
		m.matched[i] = s.key
	}
	m.matcher = language.NewMatcher(tags)
}

// SourceLanguage returns the language the message ids are written in.
func (m *MessageSource) SourceLanguage() string { return m.source.String() }

// DefaultLanguage returns the display language for requests that signal no
// preference.
func (m *MessageSource) DefaultLanguage() string { return m.fallback.String() }

// Languages returns the languages that carry at least one category file, in
// canonical tag order.
func (m *MessageSource) Languages() []string {
	langs := make([]string, 0, len(m.messages))
	for lang := range m.messages {
		langs = append(langs, lang)
	}
	sort.Strings(langs)
	return langs
}

// Translate renders msgID in category for the display language lang. The id
// is looked up under the language the matcher resolves lang to, then down the
// parent chain (zh-CN before zh); a miss at every level returns the id
// itself. Named {placeholders} in the result are replaced with params.
func (m *MessageSource) Translate(lang, category, msgID string, params map[string]any) string {
	tag := m.fallback
	if lang != "" {
		if parsed, err := language.Parse(lang); err == nil {
			tag = parsed
		}
	}
	text, ok := m.lookup(tag, category, msgID, params)
	if !ok {
		text = msgID
	}
	return renderMessage(tag, text, params)
}

// lookup walks the fallback chain for msgID: the matched language first,
// then each parent (zh-CN -> zh) until und. Plural values pick their form
// with the plural rules of the language the entry was found under.
func (m *MessageSource) lookup(tag language.Tag, category, msgID string, params map[string]any) (string, bool) {
	_, idx, _ := m.matcher.Match(tag)
	start, err := language.Parse(m.matched[idx])
	if err != nil {
		return "", false
	}
	for t := start; ; t = t.Parent() {
		if bucket, ok := m.messages[t.String()]; ok {
			if categories, ok := bucket[category]; ok {
				if e, ok := categories[msgID]; ok {
					return e.resolve(t, params), true
				}
			}
		}
		if t.Parent() == t {
			return "", false
		}
	}
}

// resolve picks the plural form for params, defaulting to the plain text.
// The numeric param "n" drives CLDR cardinal form selection; without it the
// "other" form renders.
func (e entry) resolve(tag language.Tag, params map[string]any) string {
	if e.forms == nil {
		return e.text
	}
	if n, ok := pluralCount(params); ok {
		if s, ok := e.forms[cardinalForm(tag, n)]; ok {
			return s
		}
	}
	return e.forms["other"]
}
