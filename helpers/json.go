package helpers

import "encoding/json"

// JsonMarshal encodes v as JSON with HTML escaping left on — encoding/json's
// default, kept here deliberately: "<", ">" and "&" come out as \u003c and
// friends, so the output can be embedded in HTML (script bodies, attribute
// values) without smuggling markup through JSON strings. This corresponds to
// Yii2's Json::htmlEncode safety posture, applied to plain encode.
//
// Errors are returned, not swallowed: unlike PHP's json_decode(NULL), a
// failure is loud.
func JsonMarshal(v any) ([]byte, error) {
	return json.Marshal(v)
}

// JsonMarshalString is JsonMarshal in string form, the convenience for
// embedding encoded data in templates, log lines or responses without a
// []byte-to-string conversion at every call site.
func JsonMarshalString(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// JsonUnmarshal decodes data into dest (a pointer, as with json.Unmarshal).
// It is the decode side of the helpers' JSON contract: strict — malformed
// input is an error rather than a silent zero value — and paired with
// JsonMarshal's HTML-escaping encode for round-trips.
func JsonUnmarshal(data []byte, dest any) error {
	return json.Unmarshal(data, dest)
}
