package authclient

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"golang.org/x/oauth2"
)

// githubEndpoint is github.com's OAuth surface. GitHub Enterprise Server
// installs override it through the embedded Config's Endpoint.
var githubEndpoint = oauth2.Endpoint{
	AuthURL:  "https://github.com/login/oauth/authorize",
	TokenURL: "https://github.com/login/oauth/access_token",
}

// githubUserURL is the user endpoint of GitHub's API.
const githubUserURL = "https://api.github.com/user"

// GitHub is the Provider for github.com accounts. Zero-value configuration
// falls back to the public endpoints and the user:email scope, so only
// ClientID, ClientSecret and RedirectURL are mandatory.
//
// The GitHub-specific quirks are absorbed here: the token request carries
// Accept: application/json (x/oauth2's doing — GitHub's default answer is
// form-encoded otherwise), and the userinfo call presents the access token
// in the Authorization header, never as a URL parameter.
type GitHub struct {
	oauth2.Config

	// UserURL overrides the userinfo endpoint; empty means
	// https://api.github.com/user. Tests and GitHub Enterprise
	// installations point it elsewhere.
	UserURL string
}

// Name implements Provider.
func (g *GitHub) Name() string { return "github" }

// config returns a copy of the embedded configuration with the endpoint
// gaps and the scope default filled in.
func (g *GitHub) config() *oauth2.Config {
	cfg := g.Config
	if cfg.Endpoint.AuthURL == "" {
		cfg.Endpoint.AuthURL = githubEndpoint.AuthURL
	}
	if cfg.Endpoint.TokenURL == "" {
		cfg.Endpoint.TokenURL = githubEndpoint.TokenURL
	}
	if len(cfg.Scopes) == 0 {
		cfg.Scopes = []string{"user:email"}
	}
	return &cfg
}

// BuildAuthURL implements Provider.
func (g *GitHub) BuildAuthURL(state string) string {
	return g.config().AuthCodeURL(state)
}

// Exchange implements Provider.
func (g *GitHub) Exchange(ctx context.Context, code string) (*oauth2.Token, error) {
	token, err := g.config().Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("authclient: github: exchange code: %w", err)
	}
	return token, nil
}

// githubUser is the part of GET /user the identity maps onto; the rest
// lives in Identity.Raw. id is the account's stable numeric identifier,
// login the handle, name the display name (nullable in GitHub's answer),
// email the address (empty without the user:email scope or when the user
// has none exposed).
type githubUser struct {
	ID    int64  `json:"id"`
	Login string `json:"login"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// FetchIdentity implements Provider. The display name falls back to the
// login handle when the profile carries no name — GitHub answers null for
// users who never set one, and a blank name helps nobody's UI.
func (g *GitHub) FetchIdentity(ctx context.Context, token *oauth2.Token) (*Identity, error) {
	if token == nil {
		return nil, errors.New("authclient: github: fetch identity: no token")
	}
	user, raw, err := fetchUser[githubUser](ctx, g.config().Client(ctx, token), g.userURL())
	if err != nil {
		return nil, fmt.Errorf("authclient: github: %w", err)
	}
	if user.ID == 0 {
		return nil, errors.New("authclient: github: userinfo payload carries no user id")
	}
	name := user.Name
	if name == "" {
		name = user.Login
	}
	return &Identity{
		Provider: g.Name(),
		ID:       strconv.FormatInt(user.ID, 10),
		Name:     name,
		Email:    user.Email,
		Raw:      raw,
	}, nil
}

// userURL resolves the userinfo endpoint with the override default.
func (g *GitHub) userURL() string {
	if g.UserURL != "" {
		return g.UserURL
	}
	return githubUserURL
}

// The provider interface is part of the contract; keep the compiler honest.
var _ Provider = (*GitHub)(nil)
