// Package helpers provides idiomatic helper functions for arrays, strings,
// files, JSON, URLs and HTML; it corresponds to Yii2's yii\helpers
// subsystem. Names are Go-idiomatic and package-qualified
// (helpers.ArrayGet, helpers.StrRandom, ...), not PHP underscore imports.
//
// Two contracts cut across the subdomains. JSON output is HTML-escaped by
// default so encoded values can be embedded in documents, and HTML builders
// escape all plain input — verbatim output requires the explicit HtmlRaw
// marker, the XSS posture Yii2 applies to rendering.
//
// Everything here is a pure function or a thin stdlib-bound operation: no
// global state, no framework services. URL generation from route names
// lives in the url package instead, which this package complements with
// literal-path assembly.
package helpers
