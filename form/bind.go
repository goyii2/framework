package form

import (
	"bytes"
	"context"
	"encoding"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"time"

	"github.com/goyii2/framework/i18n"
	"github.com/goyii2/framework/validators"
)

// ScenarioBind labels the validators.Errors binding produces: these messages
// report request values that failed to parse, before any validation rule has
// run.
const ScenarioBind validators.Scenario = "bind"

// Parse-failure message ids. Each doubles as the i18n id looked up in the
// validators' message category, exactly like rule messages.
const (
	msgInteger  = "not a valid integer"
	msgNumber   = "not a valid number"
	msgBoolean  = "not a valid boolean"
	msgDateTime = "not a valid date-time"
	msgValue    = "not a valid value"
	msgBody     = "malformed form body"
	msgJSONBody = "malformed JSON body"
)

var timeType = reflect.TypeOf(time.Time{})

// Bind parses the request body into dst — a pointer to a form model struct —
// dispatching on the body's declared content type: an application/json body
// goes through the JSON source, an x-www-form-urlencoded body through the form
// source. Fields opt in with a `form:"name"` tag; the tag is the whitelist:
// untagged exported fields and fields tagged "-" are never assigned, and
// request keys with no tagged field (hidden inputs like _csrf among them) are
// ignored. It returns nil when every submitted value parsed, and a
// *validators.Errors whose Fields map each failing key to its parse message
// otherwise. A body of any other content type binds nothing and reports no
// failure.
func Bind(r *http.Request, dst any) *validators.Errors {
	mediatype, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil {
		return nil
	}
	switch mediatype {
	case "application/json":
		return bindJSONBody(r, dst)
	case "application/x-www-form-urlencoded":
		if err := r.ParseForm(); err != nil {
			return rootBodyError(r.Context(), msgBody)
		}
		return bindValues(r.Context(), r.PostForm, dst)
	default:
		return nil
	}
}

// BindQuery parses the request's URL query into dst under the same rules as
// Bind, reading the query string as the sole source.
func BindQuery(r *http.Request, dst any) *validators.Errors {
	return bindValues(r.Context(), r.URL.Query(), dst)
}

// bindJSONBody reads the JSON body into a generic map first and assigns from
// that map through the same tag whitelist as the form source: never decoding
// the body straight into the model, so no JSON key can ever reach a field the
// model did not declare — encoding/json's case-insensitive field matching is
// precisely the over-posting hole this detour closes. An empty body submits
// no values, exactly like an empty form body; a body that is not one JSON
// object is reported under the record-level $root key.
func bindJSONBody(r *http.Request, dst any) *validators.Errors {
	data, err := io.ReadAll(r.Body)
	if err != nil {
		return rootBodyError(r.Context(), msgJSONBody)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return nil
	}
	var payload map[string]any
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := dec.Decode(&payload); err != nil {
		return rootBodyError(r.Context(), msgJSONBody)
	}
	if dec.More() {
		return rootBodyError(r.Context(), msgJSONBody)
	}
	return bindJSONValues(r.Context(), payload, dst)
}

// rootBodyError reports a body that cannot be decoded at all as a
// record-level failure instead of a field failure.
func rootBodyError(ctx context.Context, msg string) *validators.Errors {
	fields := map[string][]string{"$root": {msg}}
	translateMessages(ctx, fields)
	return &validators.Errors{Scenario: ScenarioBind, Fields: fields}
}

// bindJSONValues assigns from the decoded JSON payload: every tagged field
// whose exact key appears in the payload gets its value converted in, and the
// messages of values that failed to convert are collected per key.
func bindJSONValues(ctx context.Context, payload map[string]any, dst any) *validators.Errors {
	b := newBinder(ctx, dst)
	b.eachTagged(func(name string, fv reflect.Value) {
		value, ok := payload[name]
		if !ok {
			return
		}
		if msg := setJSONField(fv, value); msg != "" {
			b.fail(name, msg)
		}
	})
	return b.errs()
}

// bindValues assigns from form pairs: every tagged field whose key appears in
// values gets its value(s) parsed in, and the messages of values that failed
// to parse are collected per key.
func bindValues(ctx context.Context, values url.Values, dst any) *validators.Errors {
	b := newBinder(ctx, dst)
	b.eachTagged(func(name string, fv reflect.Value) {
		vals, ok := values[name]
		if !ok || len(vals) == 0 {
			return
		}
		if msg := setField(fv, vals); msg != "" {
			b.fail(name, msg)
		}
	})
	return b.errs()
}

// binder walks a destination's tagged fields and collects the per-key parse
// messages of values that failed to assign. Every source runs through it, so
// the whitelist walk and the error shape are identical wherever a value came
// from.
type binder struct {
	ctx    context.Context
	model  reflect.Value
	fields map[string][]string
}

// newBinder validates the destination — a non-nil pointer to a struct — and
// returns the walker over its tag-whitelisted fields.
func newBinder(ctx context.Context, dst any) *binder {
	v := reflect.ValueOf(dst)
	if v.Kind() != reflect.Pointer || v.IsNil() {
		panic("form: destination must be a non-nil pointer to a struct")
	}
	v = v.Elem()
	if v.Kind() != reflect.Struct {
		panic("form: destination must be a non-nil pointer to a struct")
	}
	return &binder{ctx: ctx, model: v, fields: map[string][]string{}}
}

// eachTagged visits every exported field that opted into binding with a form
// tag, handing the visitor the tag name and the addressable field.
func (b *binder) eachTagged(visit func(name string, fv reflect.Value)) {
	t := b.model.Type()
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}
		name := field.Tag.Get("form")
		if name == "" || name == "-" {
			continue
		}
		visit(name, b.model.Field(i))
	}
}

// fail records a parse message under the key that failed.
func (b *binder) fail(name, msg string) {
	b.fields[name] = append(b.fields[name], msg)
}

// errs translates whatever messages collected and returns the validators
// failure, or nil when every value parsed.
func (b *binder) errs() *validators.Errors {
	if len(b.fields) == 0 {
		return nil
	}
	translateMessages(b.ctx, b.fields)
	return &validators.Errors{Scenario: ScenarioBind, Fields: b.fields}
}

// translateMessages runs every field message through the i18n lookup for the
// request's language, exactly as rule validation messages are translated: the
// message is the id under the validators' message category, a miss keeps the
// source text, and contexts without a message source change nothing.
func translateMessages(ctx context.Context, fields map[string][]string) {
	if ctx == nil {
		return
	}
	if _, ok := i18n.SourceFromContext(ctx); !ok {
		return
	}
	for field, messages := range fields {
		translated := make([]string, len(messages))
		for i, msg := range messages {
			translated[i] = i18n.T(ctx, validators.MessageCategory, msg, nil)
		}
		fields[field] = translated
	}
}

// setJSONField assigns one decoded JSON value to the addressable field fv and
// returns "" on success, or the message id describing why the value did not
// fit. JSON carries typed values, so a value whose kind does not match the
// field — a string sent to an int field, a number to a text field — is a type
// mismatch reported with the message of what the field needed, exactly like an
// unparseable text in the form source. Numbers keep their literal wire form
// (json.Number), so an integer field never silently rounds 19.5 or a big
// literal. A null value binds nothing. A tagged field of a type outside the
// supported set is a programming error and panics when a value arrives for it.
func setJSONField(fv reflect.Value, value any) string {
	if value == nil {
		return ""
	}
	// A JSON empty string sent to a field that does not hold text reads as
	// "the input was left unfilled" — the form source's verdict on a lone
	// blank value — so the field keeps its current value; text fields take
	// the empty text below.
	if s, ok := value.(string); ok && s == "" && fv.Kind() != reflect.String {
		return ""
	}

	// time.Time gets its own message; the generic TextUnmarshaler branch
	// below would also parse it but report it as an opaque bad value.
	if fv.Type() == timeType {
		s, ok := value.(string)
		if !ok {
			return msgDateTime
		}
		parsed, err := time.Parse(time.RFC3339, s)
		if err != nil {
			return msgDateTime
		}
		fv.Set(reflect.ValueOf(parsed))
		return ""
	}
	// Custom parsing wins over the generic kinds, but only for text: a
	// TextUnmarshaler field decodes itself from the string form of its value.
	if u, ok := fv.Addr().Interface().(encoding.TextUnmarshaler); ok {
		s, ok := value.(string)
		if !ok {
			return msgValue
		}
		if err := u.UnmarshalText([]byte(s)); err != nil {
			return msgValue
		}
		return ""
	}

	switch fv.Kind() {
	case reflect.String:
		s, ok := value.(string)
		if !ok {
			return msgValue
		}
		fv.SetString(s)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, ok := value.(json.Number)
		if !ok {
			return msgInteger
		}
		parsed, err := strconv.ParseInt(n.String(), 10, fv.Type().Bits())
		if err != nil {
			return msgInteger
		}
		fv.SetInt(parsed)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n, ok := value.(json.Number)
		if !ok {
			return msgInteger
		}
		parsed, err := strconv.ParseUint(n.String(), 10, fv.Type().Bits())
		if err != nil {
			return msgInteger
		}
		fv.SetUint(parsed)
	case reflect.Float32, reflect.Float64:
		n, ok := value.(json.Number)
		if !ok {
			return msgNumber
		}
		parsed, err := strconv.ParseFloat(n.String(), fv.Type().Bits())
		if err != nil {
			return msgNumber
		}
		fv.SetFloat(parsed)
	case reflect.Bool:
		b, ok := value.(bool)
		if !ok {
			return msgBoolean
		}
		fv.SetBool(b)
	case reflect.Slice:
		if fv.Type().Elem().Kind() != reflect.String {
			panic(fmt.Sprintf("form: field of type %s is not supported by form binding", fv.Type()))
		}
		switch val := value.(type) {
		case string:
			// A lone string mirrors a form carrying a single submitted value.
			out := reflect.MakeSlice(fv.Type(), 1, 1)
			out.Index(0).SetString(val)
			fv.Set(out)
		case []any:
			out := reflect.MakeSlice(fv.Type(), len(val), len(val))
			for i, el := range val {
				s, ok := el.(string)
				if !ok {
					return msgValue
				}
				out.Index(i).SetString(s)
			}
			fv.Set(out)
		default:
			return msgValue
		}
	default:
		panic(fmt.Sprintf("form: field of type %s is not supported by form binding", fv.Type()))
	}
	return ""
}

// setField assigns vals to the addressable field fv and returns "" on
// success, or the message id describing why the value did not parse. The
// first value feeds scalar fields; a lone blank value means the input was
// left unfilled and leaves every field except text untouched. A tagged field
// of a type outside the supported set is a programming error and panics when
// a value arrives for it.
func setField(fv reflect.Value, vals []string) string {
	if fv.Kind() != reflect.String && len(vals) == 1 && vals[0] == "" {
		return ""
	}
	raw := vals[0]

	// time.Time gets its own message; the generic TextUnmarshaler branch
	// below would also parse it (RFC 3339 is time.Time's own wire format)
	// but report it as an opaque bad value.
	if fv.Type() == timeType {
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return msgDateTime
		}
		fv.Set(reflect.ValueOf(parsed))
		return ""
	}
	// Custom parsing wins over the generic kinds: a field whose type
	// implements encoding.TextUnmarshaler decodes itself.
	if u, ok := fv.Addr().Interface().(encoding.TextUnmarshaler); ok {
		if err := u.UnmarshalText([]byte(raw)); err != nil {
			return msgValue
		}
		return ""
	}

	switch fv.Kind() {
	case reflect.String:
		fv.SetString(raw)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, err := strconv.ParseInt(raw, 10, fv.Type().Bits())
		if err != nil {
			return msgInteger
		}
		fv.SetInt(n)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n, err := strconv.ParseUint(raw, 10, fv.Type().Bits())
		if err != nil {
			return msgInteger
		}
		fv.SetUint(n)
	case reflect.Float32, reflect.Float64:
		f, err := strconv.ParseFloat(raw, fv.Type().Bits())
		if err != nil {
			return msgNumber
		}
		fv.SetFloat(f)
	case reflect.Bool:
		b, err := strconv.ParseBool(raw)
		if err != nil {
			return msgBoolean
		}
		fv.SetBool(b)
	case reflect.Slice:
		if fv.Type().Elem().Kind() != reflect.String {
			panic(fmt.Sprintf("form: field of type %s is not supported by form binding", fv.Type()))
		}
		out := reflect.MakeSlice(fv.Type(), len(vals), len(vals))
		for i, val := range vals {
			out.Index(i).SetString(val)
		}
		fv.Set(out)
	default:
		panic(fmt.Sprintf("form: field of type %s is not supported by form binding", fv.Type()))
	}
	return ""
}
