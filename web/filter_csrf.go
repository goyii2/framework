package web

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
)

// Csrf guards state-changing requests against cross-site request forgery
// using the double-submit cookie pattern: the server issues a random token
// cookie on safe requests, and every unsafe request must present the same
// value back — either in a header or in a form field. A token not matching
// the cookie (or a missing cookie or token) rejects the request with 400.
//
// The token is request-scoped state, not per-session: with a session store
// the check can move to a session-bound token later without changing the
// wire format. The application entry point installs this middleware in the
// global chain by default, and a global filter cannot be opted out of per
// mount — pure API applications turn it off once through the configuration
// (Web.DisableCsrf). Mounting the filter by hand (instead of the default)
// is where the per-filter Disabled switch applies.
type Csrf struct {
	// CookieName is the cookie carrying the token; default "_csrf".
	CookieName string

	// HeaderName is the request header carrying the token on unsafe
	// requests; default "X-Csrf-Token".
	HeaderName string

	// FormField is the form field carrying the token, consulted when the
	// header is absent; default "_csrf". Only the request body counts —
	// tokens in the query string are never accepted (they leak through
	// referrers).
	FormField string

	// Secure marks the cookie HTTPS-only. Default false; set true when the
	// application is served behind TLS.
	Secure bool

	// Disabled turns the filter into a pass-through, the per-mount form of
	// the global configuration switch.
	Disabled bool
}

// Middleware returns the CSRF validation as middleware.
func (f Csrf) Middleware() Middleware {
	cookieName := defaultString(f.CookieName, "_csrf")
	headerName := defaultString(f.HeaderName, "X-Csrf-Token")
	formField := defaultString(f.FormField, "_csrf")

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if f.Disabled {
				next.ServeHTTP(w, r)
				return
			}

			var cookieToken string
			if ck, err := r.Cookie(cookieName); err == nil {
				cookieToken = ck.Value
			}

			if isSafeMethod(r.Method) {
				token := cookieToken
				if token == "" {
					// First contact (or an expired cookie): issue a token so
					// form rendering can embed it; cookie and form field then
					// travel back together on the unsafe request.
					token = newCsrfToken()
					http.SetCookie(w, &http.Cookie{
						Name:     cookieName,
						Value:    token,
						Path:     "/",
						Secure:   f.Secure,
						HttpOnly: true,
						SameSite: http.SameSiteLaxMode,
					})
				}
				if c := FromContext(r.Context()); c != nil {
					c.csrf = token
				}
				next.ServeHTTP(w, r)
				return
			}

			// Unsafe method: the submitted token must match the cookie's.
			submitted := r.Header.Get(headerName)
			if submitted == "" {
				// ParseForm only reads the body for form-encoded requests;
				// the query string lands in r.Form but not in PostFormValue.
				submitted = r.PostFormValue(formField)
			}
			if submitted == "" || cookieToken == "" ||
				subtle.ConstantTimeCompare([]byte(submitted), []byte(cookieToken)) != 1 {
				writeError(w, r, BadRequest("unable to verify your data submission"))
				return
			}
			if c := FromContext(r.Context()); c != nil {
				c.csrf = cookieToken
			}
			next.ServeHTTP(w, r)
		})
	}
}

// isSafeMethod reports whether the verb is not expected to change server
// state, so it needs no token.
func isSafeMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
		return true
	}
	return false
}

// newCsrfToken mints 32 bytes of cryptographic randomness, URL-safe
// base64 encoded.
func newCsrfToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		// The system CSPRNG failing means the host is out of entropy
		// sources; refusing to run beats issuing guessable tokens.
		panic("web: csprng unavailable: " + err.Error())
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// defaultString falls back to def for an empty value.
func defaultString(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
