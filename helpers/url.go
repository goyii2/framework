package helpers

import (
	neturl "net/url"
	"sort"
	"strings"

	"github.com/goyii2/framework/url"
)

// UrlQuery encodes params as a query string without the leading "?", with
// keys sorted so the output is deterministic regardless of map iteration
// order — the same guarantee the url package gives generated routes. It is a
// pure function over the parameter set: no managers, no global state.
func UrlQuery(params url.P) string {
	if len(params) == 0 {
		return ""
	}
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	vs := make(neturl.Values, len(params))
	for _, k := range keys {
		vs[k] = []string{params[k]}
	}
	return vs.Encode()
}

// UrlBuild appends params to path as a query string: path stays literal
// (slashes and pre-encoded segments are the caller's business), the
// parameters are percent-encoded with sorted keys. A path that already
// carries a query is extended with "&"; with no parameters path comes back
// unchanged. This is the route-name-free counterpart of url.To: it assembles
// a plain URL from a literal path and parameters.
func UrlBuild(path string, params url.P) string {
	q := UrlQuery(params)
	if q == "" {
		return path
	}
	if strings.ContainsRune(path, '?') {
		return path + "&" + q
	}
	return path + "?" + q
}

// UrlAddQuery merges params into an existing URL's query string and returns
// the rebuilt URL. Parameters in params replace any values the URL already
// carries under the same key; other existing keys keep their values and
// ordering stays sorted through neturl.Values.Encode. Scheme, host, path and
// fragment are preserved; a URL that does not parse is an error, not a
// best-effort concatenation.
func UrlAddQuery(rawURL string, params url.P) (string, error) {
	u, err := neturl.Parse(rawURL)
	if err != nil {
		return "", err
	}
	q := u.Query()
	for k, v := range params {
		q[k] = []string{v}
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}
