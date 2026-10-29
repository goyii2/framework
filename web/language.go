package web

import (
	"net/http"

	"github.com/goyii2/framework/i18n"
)

// LanguageQueryParam is the query parameter selecting the display language
// for one request ("?lang=zh-CN"); the choice is persisted to the session,
// making it sticky for subsequent requests.
const LanguageQueryParam = "lang"

// LanguageSessionKey is the session slot the language middleware persists
// the chosen display language under. Applications offering an explicit
// language switcher write this key (or send the query parameter) and the
// next request picks the value up.
const LanguageSessionKey = "language"

// Language returns the middleware resolving each request's display language
// and carrying it in the request context for i18n.T and i18n.F. The
// preference chain: the lang query parameter, the session's stored choice,
// the Accept-Language header (quality-ordered, mapped onto the languages
// that actually carry translations), then the application's default. A
// request that names its language via the query parameter has the resolved
// language stored in the session, so the choice survives later requests.
//
// The HTTP entry point installs the middleware by default, inside the
// session middleware (whose session it consults). Applications assembling a
// custom chain add it explicitly after their session middleware.
func Language() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c := FromContext(r.Context())
			if c == nil {
				next.ServeHTTP(w, r)
				return
			}
			src := c.App().I18n()

			prefs := make([]string, 0, 3)
			requested := r.URL.Query().Get(LanguageQueryParam)
			if requested != "" {
				prefs = append(prefs, requested)
			}
			var sessionLang string
			if s := c.Session(); s != nil {
				sessionLang = s.GetString(LanguageSessionKey)
				if sessionLang != "" {
					prefs = append(prefs, sessionLang)
				}
			}
			prefs = append(prefs, i18n.AcceptLanguages(r.Header.Get("Accept-Language"))...)

			lang := src.Negotiate(prefs...)

			// The explicit request signal persists the resolved language, so
			// the switch holds for requests without the parameter.
			if requested != "" && c.Session() != nil && sessionLang != lang {
				c.Session().Set(LanguageSessionKey, lang)
			}

			next.ServeHTTP(w, r.WithContext(i18n.WithLanguage(r.Context(), lang)))
		})
	}
}
