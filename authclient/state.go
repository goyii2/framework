package authclient

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
)

// GenerateState mints the state parameter for an authorization redirect: 32
// bytes of cryptographic randomness, URL-safe base64 encoded. Bind the value
// to the user's session before redirecting (how it is stored stays the
// application's choice — the package never touches session internals), then
// check the callback's state against it with ValidState before exchanging
// the code. A state the application did not generate means the callback was
// not provoked by its own redirect — the CSRF an attacker would need to
// forge.
func GenerateState() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		// The system CSPRNG failing means the host is out of entropy
		// sources; refusing to run beats issuing guessable states.
		panic("authclient: csprng unavailable: " + err.Error())
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// ValidState reports whether the state arriving on a callback matches the
// one GenerateState produced for the redirect. The comparison is constant
// time, and an empty value on either side is never valid: a missing
// submitted state and a missing stored state must both fail closed rather
// than compare equal as empty strings.
func ValidState(submitted, expected string) bool {
	if submitted == "" || expected == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(submitted), []byte(expected)) == 1
}
