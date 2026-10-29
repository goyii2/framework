package authclient

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"testing"

	"golang.org/x/oauth2"
)

// githubProvider builds a GitHub provider wired to the mock server.
func githubProvider(m *mockOAuth) *GitHub {
	return &GitHub{
		Config: oauth2.Config{
			ClientID:     "client-id-1",
			ClientSecret: "client-secret-1",
			RedirectURL:  m.URL() + "/callback",
			Endpoint: oauth2.Endpoint{
				AuthURL:  m.URL() + "/authorize",
				TokenURL: m.URL() + "/token",
			},
		},
		UserURL: m.URL() + "/user",
	}
}

func TestGitHubName(t *testing.T) {
	if got := (&GitHub{}).Name(); got != "github" {
		t.Fatalf("Name() = %q, want %q", got, "github")
	}
}

func TestGitHubBuildAuthURL(t *testing.T) {
	m := newMockOAuth(t)
	p := githubProvider(m)

	state := GenerateState()
	raw := p.BuildAuthURL(state)

	u, err := url.Parse(raw)
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
	if q.Get("client_id") != "client-id-1" {
		t.Errorf("client_id = %q, want client-id-1", q.Get("client_id"))
	}
	if q.Get("redirect_uri") != m.URL()+"/callback" {
		t.Errorf("redirect_uri = %q, want %s/callback", q.Get("redirect_uri"), m.URL())
	}
	if q.Get("state") != state {
		t.Errorf("state = %q, want the state passed in (%q)", q.Get("state"), state)
	}
	if q.Get("scope") != "user:email" {
		t.Errorf("default scope = %q, want user:email", q.Get("scope"))
	}
}

func TestGitHubBuildAuthURLCustomScopesAndDefaultEndpoint(t *testing.T) {
	// A provider with no Endpoint and no Scopes falls back to the
	// production github.com endpoints and the default scope.
	p := &GitHub{
		Config: oauth2.Config{
			ClientID:     "client-id-1",
			ClientSecret: "client-secret-1",
			RedirectURL:  "https://app.example.com/callback",
			Scopes:       []string{"read:user", "user:email"},
		},
	}
	u, err := url.Parse(p.BuildAuthURL("s"))
	if err != nil {
		t.Fatalf("BuildAuthURL produced an unparseable URL: %v", err)
	}
	if u.Scheme != "https" || u.Host != "github.com" || u.Path != "/login/oauth/authorize" {
		t.Fatalf("default auth endpoint = %s://%s%s, want https://github.com/login/oauth/authorize",
			u.Scheme, u.Host, u.Path)
	}
	if got := u.Query().Get("scope"); got != "read:user user:email" {
		t.Errorf("scope = %q, want space-joined custom scopes", got)
	}
}

func TestGitHubFullFlow(t *testing.T) {
	m := newMockOAuth(t)
	p := githubProvider(m)
	ctx := context.Background()

	// The authorization redirect: the provider builds the URL, the mock
	// authorize endpoint answers with a redirect carrying code and state.
	// The client does not follow the redirect — its target is the
	// application's callback, which this mock does not serve.
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
	if !token.Valid() {
		t.Error("exchanged token is not valid")
	}
	form := m.tokenFormValues()
	if got := form.Get("grant_type"); got != "authorization_code" {
		t.Errorf("token grant_type = %q, want authorization_code", got)
	}
	if got := form.Get("code"); got != code {
		t.Errorf("token code = %q, want %q", got, code)
	}
	if got := form.Get("redirect_uri"); got != m.URL()+"/callback" {
		t.Errorf("token redirect_uri = %q, want %s/callback", got, m.URL())
	}
	// The client credentials reach the endpoint via basic auth or form
	// fields, whichever style x/oauth2 settled on; either way both must
	// arrive.
	if id, secret := m.tokenClientCredentials(); id != "client-id-1" || secret != "client-secret-1" {
		t.Errorf("token client credentials = %q / %q, want client-id-1 / client-secret-1", id, secret)
	}

	// Identity fetch.
	id, err := p.FetchIdentity(ctx, token)
	if err != nil {
		t.Fatalf("FetchIdentity: %v", err)
	}
	if id.Provider != "github" {
		t.Errorf("identity Provider = %q, want github", id.Provider)
	}
	if id.ID != "583231" {
		t.Errorf("identity ID = %q, want 583231", id.ID)
	}
	if id.Name != "The Octocat" {
		t.Errorf("identity Name = %q, want The Octocat", id.Name)
	}
	if id.Email != "octocat@github.com" {
		t.Errorf("identity Email = %q, want octocat@github.com", id.Email)
	}
	if id.Raw["login"] != "octocat" {
		t.Errorf("identity Raw[login] = %v, want octocat", id.Raw["login"])
	}
	// GitHub's API takes the token in the Authorization header, never as a
	// URL parameter.
	if got := m.lastUserAuth(); got != "Bearer mock-access-token" {
		t.Errorf("userinfo Authorization = %q, want Bearer mock-access-token", got)
	}
	if _, present := m.lastUserQuery()["access_token"]; present {
		t.Error("userinfo request carried the token as a URL parameter")
	}
}

func TestGitHubNameFallsBackToLogin(t *testing.T) {
	m := newMockOAuth(t)
	m.UserBody = `{"id":583231,"login":"octocat","name":null,"email":null}`
	p := githubProvider(m)

	id, err := p.FetchIdentity(context.Background(), &oauth2.Token{AccessToken: "t"})
	if err != nil {
		t.Fatalf("FetchIdentity: %v", err)
	}
	if id.Name != "octocat" {
		t.Errorf("identity Name = %q, want login fallback octocat", id.Name)
	}
	if id.Email != "" {
		t.Errorf("identity Email = %q, want empty when the provider has none", id.Email)
	}
}

func TestGitHubExchangeTokenFailure(t *testing.T) {
	m := newMockOAuth(t)
	m.TokenStatus = http.StatusBadRequest
	m.TokenBody = `{"error":"bad_verification_code"}`
	p := githubProvider(m)

	_, err := p.Exchange(context.Background(), "wrong-code")
	if err == nil {
		t.Fatal("Exchange with a rejected code returned no error")
	}
	var retr *oauth2.RetrieveError
	if !errors.As(err, &retr) {
		t.Fatalf("Exchange error is %T, want an oauth2.RetrieveError reachable through the wrap: %v", err, err)
	}
	if retr.Response == nil || retr.Response.StatusCode != http.StatusBadRequest {
		t.Fatalf("wrapped RetrieveError does not carry the 400 response")
	}
}

func TestGitHubFetchIdentityFailures(t *testing.T) {
	m := newMockOAuth(t)
	m.UserStatus = http.StatusUnauthorized
	m.UserBody = `{"message":"Bad credentials"}`
	p := githubProvider(m)
	token := &oauth2.Token{AccessToken: "expired"}

	if _, err := p.FetchIdentity(context.Background(), token); err == nil {
		t.Fatal("FetchIdentity with a rejected token returned no error")
	}

	m2 := newMockOAuth(t)
	m2.UserBody = `{"login":"octocat"}`
	p2 := githubProvider(m2)
	if _, err := p2.FetchIdentity(context.Background(), token); err == nil {
		t.Fatal("FetchIdentity with a payload missing the user id returned no error")
	}

	m3 := newMockOAuth(t)
	m3.UserBody = `{"id":583231, not json`
	p3 := githubProvider(m3)
	if _, err := p3.FetchIdentity(context.Background(), token); err == nil {
		t.Fatal("FetchIdentity with a malformed payload returned no error")
	}

	if _, err := p3.FetchIdentity(context.Background(), nil); err == nil {
		t.Fatal("FetchIdentity with a nil token returned no error")
	}
}
