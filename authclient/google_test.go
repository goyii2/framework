package authclient

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"testing"

	"golang.org/x/oauth2"
)

// googleProvider builds a Google provider wired to the mock server.
func googleProvider(m *mockOAuth) *Google {
	return &Google{
		Config: oauth2.Config{
			ClientID:     "client-id-2",
			ClientSecret: "client-secret-2",
			RedirectURL:  m.URL() + "/callback",
			Endpoint: oauth2.Endpoint{
				AuthURL:  m.URL() + "/authorize",
				TokenURL: m.URL() + "/token",
			},
		},
		UserURL: m.URL() + "/user",
	}
}

func TestGoogleName(t *testing.T) {
	if got := (&Google{}).Name(); got != "google" {
		t.Fatalf("Name() = %q, want %q", got, "google")
	}
}

func TestGoogleBuildAuthURL(t *testing.T) {
	m := newMockOAuth(t)
	p := googleProvider(m)

	state := GenerateState()
	u, err := url.Parse(p.BuildAuthURL(state))
	if err != nil {
		t.Fatalf("BuildAuthURL produced an unparseable URL: %v", err)
	}
	q := u.Query()
	if got := u.Scheme + "://" + u.Host + u.Path; got != m.URL()+"/authorize" {
		t.Fatalf("auth URL points at %s, want the configured authorize endpoint", got)
	}
	if q.Get("response_type") != "code" {
		t.Errorf("response_type = %q, want code", q.Get("response_type"))
	}
	if q.Get("client_id") != "client-id-2" {
		t.Errorf("client_id = %q, want client-id-2", q.Get("client_id"))
	}
	if q.Get("redirect_uri") != m.URL()+"/callback" {
		t.Errorf("redirect_uri = %q, want %s/callback", q.Get("redirect_uri"), m.URL())
	}
	if q.Get("state") != state {
		t.Errorf("state = %q, want the state passed in (%q)", q.Get("state"), state)
	}
	if q.Get("scope") != "openid email profile" {
		t.Errorf("default scope = %q, want openid email profile", q.Get("scope"))
	}
}

func TestGoogleBuildAuthURLDefaultEndpoint(t *testing.T) {
	p := &Google{
		Config: oauth2.Config{
			ClientID:     "client-id-2",
			ClientSecret: "client-secret-2",
			RedirectURL:  "https://app.example.com/callback",
		},
	}
	u, err := url.Parse(p.BuildAuthURL("s"))
	if err != nil {
		t.Fatalf("BuildAuthURL produced an unparseable URL: %v", err)
	}
	if u.Host != "accounts.google.com" || u.Path != "/o/oauth2/auth" {
		t.Fatalf("default auth endpoint = https://%s%s, want https://accounts.google.com/o/oauth2/auth", u.Host, u.Path)
	}
}

func TestGoogleFullFlow(t *testing.T) {
	m := newMockOAuth(t)
	m.UserBody = `{"sub":"110169484474386276334","name":"Ada Lovelace","email":"ada@example.com","picture":"https://lh3.example/ada.png"}`
	p := googleProvider(m)
	ctx := context.Background()

	// Authorization redirect, code captured from the callback the mock
	// authorize endpoint sends back.
	state := GenerateState()
	noRedirect := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	resp, err := noRedirect.Get(p.BuildAuthURL(state))
	if err != nil {
		t.Fatalf("authorize request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("authorize answered %d, want 302", resp.StatusCode)
	}
	back, err := url.Parse(resp.Header.Get("Location"))
	if err != nil {
		t.Fatalf("authorize redirect target unparseable: %v", err)
	}
	if got := back.Query().Get("state"); got != state {
		t.Fatalf("callback state = %q, want the state the redirect started with", got)
	}
	code := back.Query().Get("code")

	// Code exchange.
	token, err := p.Exchange(ctx, code)
	if err != nil {
		t.Fatalf("Exchange: %v", err)
	}
	if token.AccessToken != "mock-access-token" {
		t.Errorf("access token = %q, want mock-access-token", token.AccessToken)
	}
	if id, secret := m.tokenClientCredentials(); id != "client-id-2" || secret != "client-secret-2" {
		t.Errorf("token client credentials = %q / %q, want client-id-2 / client-secret-2", id, secret)
	}

	// Identity fetch.
	idn, err := p.FetchIdentity(ctx, token)
	if err != nil {
		t.Fatalf("FetchIdentity: %v", err)
	}
	if idn.Provider != "google" {
		t.Errorf("identity Provider = %q, want google", idn.Provider)
	}
	if idn.ID != "110169484474386276334" {
		t.Errorf("identity ID = %q, want the sub claim 110169484474386276334", idn.ID)
	}
	if idn.Name != "Ada Lovelace" {
		t.Errorf("identity Name = %q, want Ada Lovelace", idn.Name)
	}
	if idn.Email != "ada@example.com" {
		t.Errorf("identity Email = %q, want ada@example.com", idn.Email)
	}
	if idn.Raw["picture"] != "https://lh3.example/ada.png" {
		t.Errorf("identity Raw[picture] = %v, want the provider payload's value", idn.Raw["picture"])
	}
	if got := m.lastUserAuth(); got != "Bearer mock-access-token" {
		t.Errorf("userinfo Authorization = %q, want Bearer mock-access-token", got)
	}
}

func TestGoogleExchangeTokenFailure(t *testing.T) {
	m := newMockOAuth(t)
	m.TokenStatus = http.StatusUnauthorized
	m.TokenBody = `{"error":"invalid_client"}`
	p := googleProvider(m)

	_, err := p.Exchange(context.Background(), "any-code")
	if err == nil {
		t.Fatal("Exchange with a rejected client returned no error")
	}
	var retr *oauth2.RetrieveError
	if !errors.As(err, &retr) {
		t.Fatalf("Exchange error is %T, want an oauth2.RetrieveError reachable through the wrap: %v", err, err)
	}
}

func TestGoogleFetchIdentityFailures(t *testing.T) {
	token := &oauth2.Token{AccessToken: "t"}

	m := newMockOAuth(t)
	m.UserStatus = http.StatusForbidden
	p := googleProvider(m)
	if _, err := p.FetchIdentity(context.Background(), token); err == nil {
		t.Fatal("FetchIdentity with a non-2xx userinfo answer returned no error")
	}

	m2 := newMockOAuth(t)
	m2.UserBody = `{"name":"Ada Lovelace"}`
	p2 := googleProvider(m2)
	if _, err := p2.FetchIdentity(context.Background(), token); err == nil {
		t.Fatal("FetchIdentity with a payload missing the sub claim returned no error")
	}

	if _, err := p2.FetchIdentity(context.Background(), nil); err == nil {
		t.Fatal("FetchIdentity with a nil token returned no error")
	}
}
