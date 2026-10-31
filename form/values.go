package form

import (
	"encoding"
	"reflect"
	"strconv"
	"strings"
)

// Values exports a form model as the map view.ActiveForm refills from: one
// string per `form`-tagged field, keyed by the tag — the counterpart of
// binding, so a rejected submission or an edit form renders what the model
// holds without a hand-written map. model may be a struct or a pointer to
// one. Strings render as themselves, integers and floats in their shortest
// exact form, booleans as "true"/"false", and []string comma-joined (a nil
// or empty list renders ""). A field whose type formats itself through
// encoding.TextMarshaler renders that text — time.Time renders RFC 3339,
// fractional seconds preserved so an edit form re-submits the stored
// instant. A field whose type has no text form — an Unmarshaler without a
// Marshaler — is left out rather than guessed, and untagged fields never
// appear.
func Values(model any) map[string]string {
	v := reflect.ValueOf(model)
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			panic("form: model must be a struct or a non-nil pointer to a struct")
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		panic("form: model must be a struct or a non-nil pointer to a struct")
	}
	t := v.Type()
	values := map[string]string{}
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}
		name := field.Tag.Get("form")
		if name == "" || name == "-" {
			continue
		}
		if s, ok := exportString(v.Field(i)); ok {
			values[name] = s
		}
	}
	return values
}

// exportString renders one field as refill text and reports whether the
// field's type has a text form at all.
func exportString(fv reflect.Value) (string, bool) {
	// Self-formatting types win over the generic kinds, mirroring how
	// binding lets TextUnmarshaler fields parse themselves. The check runs
	// before the pointer branch so a type whose MarshalText sits on the
	// pointer receiver still renders through its non-nil pointer; a nil
	// pointer must skip it (MarshalText would run through nil).
	if fv.Kind() != reflect.Pointer || !fv.IsNil() {
		if m, ok := fv.Interface().(encoding.TextMarshaler); ok {
			text, err := m.MarshalText()
			if err != nil {
				return "", false
			}
			return string(text), true
		}
	}
	// A pointer field renders its pointee; nil — a NULL column — renders as
	// the empty text a fresh form input shows.
	if fv.Kind() == reflect.Pointer {
		if fv.IsNil() {
			return "", true
		}
		fv = fv.Elem()
	}
	switch fv.Kind() {
	case reflect.String:
		return fv.String(), true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(fv.Int(), 10), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return strconv.FormatUint(fv.Uint(), 10), true
	case reflect.Float32, reflect.Float64:
		return strconv.FormatFloat(fv.Float(), 'g', -1, fv.Type().Bits()), true
	case reflect.Bool:
		return strconv.FormatBool(fv.Bool()), true
	case reflect.Slice:
		if fv.Type().Elem().Kind() != reflect.String {
			return "", false
		}
		parts := make([]string, fv.Len())
		for i := range parts {
			parts[i] = fv.Index(i).String()
		}
		return strings.Join(parts, ","), true
	default:
		return "", false
	}
}
