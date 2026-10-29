package authclient

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// mockOAuth is a stand-in authorization server: it serves the three
// endpoints an OAuth flow touches (authorize, token, userinfo) so the
// providers are exercised over real HTTP round trips, and records what each
// endpoint saw for the assertions.
//
// The response each endpoint gives is steerable through the status/body
// fields to drive the error paths; zero values mean "answer successfully".
type mockOAuth struct {
	t *testing.T

	// TokenStatus / TokenBody control the token endpoint's answer
	// (default 200 with a bearer token JSON).
	TokenStatus int
	TokenBody   string

	// UserStatus / UserBody control the userinfo endpoint's answer
	// (default 200 with a GitHub-shaped payload).
	UserStatus int
	UserBody   string

	// tokenForm holds every form field the token endpoint received.
	tokenForm []url.Values
	// tokenAuth holds the Authorization headers the token endpoint saw —
	// x/oauth2 sends client credentials as basic auth or as form fields
	// depending on the negotiated style.
	tokenAuth []string
	// userAuth holds the Authorization headers the userinfo endpoint saw.
	userAuth []string
	// userQuery holds the query strings the userinfo endpoint saw.
	userQuery []url.Values

	srv *httptest.Server
}

// newMockOAuth starts the server. Closing happens automatically on test
// cleanup.
func newMockOAuth(t *testing.T) *mockOAuth {
	t.Helper()
	m := &mockOAuth{t: t}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /authorize", func(w http.ResponseWriter, r *http.Request) {
		// Echo the flow forward like a real provider would: back to the
		// redirect_uri with a code and the caller's state.
		q := r.URL.Query()
		back, err := url.Parse(q.Get("redirect_uri"))
		if err != nil {
			http.Error(w, "bad redirect_uri", http.StatusBadRequest)
			return
		}
		grant := back.Query()
		grant.Set("code", "mock-code")
		grant.Set("state", q.Get("state"))
		back.RawQuery = grant.Encode()
		http.Redirect(w, r, back.String(), http.StatusFound)
	})
	mux.HandleFunc("POST /token", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		m.tokenForm = append(m.tokenForm, r.PostForm)
		m.tokenAuth = append(m.tokenAuth, r.Header.Get("Authorization"))
		status, body := m.TokenStatus, m.TokenBody
		if status == 0 {
			status, body = http.StatusOK,
				`{"access_token":"mock-access-token","token_type":"bearer","expires_in":3600}`
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	})
	mux.HandleFunc("GET /user", func(w http.ResponseWriter, r *http.Request) {
		m.userAuth = append(m.userAuth, r.Header.Get("Authorization"))
		m.userQuery = append(m.userQuery, r.URL.Query())
		status, body := m.UserStatus, m.UserBody
		if status == 0 {
			status = http.StatusOK
		}
		if body == "" {
			body = `{"id":583231,"login":"octocat","name":"The Octocat","email":"octocat@github.com"}`
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	})
	m.srv = httptest.NewServer(mux)
	t.Cleanup(m.srv.Close)
	return m
}

// URL returns the mock server's base URL.
func (m *mockOAuth) URL() string { return m.srv.URL }

// tokenFormValues flattens every field the token endpoint received.
func (m *mockOAuth) tokenFormValues() url.Values {
	out := url.Values{}
	for _, form := range m.tokenForm {
		for k, v := range form {
			out[k] = append(out[k], v...)
		}
	}
	return out
}

// tokenClientCredentials recovers the client credentials the token endpoint
// received, from whichever transport x/oauth2 chose: basic auth header, or
// form fields.
func (m *mockOAuth) tokenClientCredentials() (id, secret string) {
	if len(m.tokenAuth) == 0 {
		m.t.Fatal("token endpoint was never called")
	}
	header := m.tokenAuth[len(m.tokenAuth)-1]
	if encoded, ok := strings.CutPrefix(header, "Basic "); ok {
		decoded, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			m.t.Fatalf("token basic auth is not base64: %v", err)
		}
		id, secret, _ = strings.Cut(string(decoded), ":")
		return id, secret
	}
	form := m.tokenFormValues()
	return form.Get("client_id"), form.Get("client_secret")
}

// lastUserAuth returns the Authorization header of the latest userinfo call.
func (m *mockOAuth) lastUserAuth() string {
	if len(m.userAuth) == 0 {
		m.t.Fatal("userinfo endpoint was never called")
	}
	return m.userAuth[len(m.userAuth)-1]
}

// lastUserQuery returns the query of the latest userinfo call.
func (m *mockOAuth) lastUserQuery() url.Values {
	if len(m.userQuery) == 0 {
		m.t.Fatal("userinfo endpoint was never called")
	}
	return m.userQuery[len(m.userQuery)-1]
}
