package authclient

import (
	"context"
	"errors"
	"fmt"

	"golang.org/x/oauth2"
)

// googleEndpoint is Google's OAuth / OpenID Connect surface. The auth style
// is pinned to in-params: Google documents client credentials in the token
// request body, and pinning skips x/oauth2's auto-detection probing.
var googleEndpoint = oauth2.Endpoint{
	AuthURL:   "https://accounts.google.com/o/oauth2/auth",
	TokenURL:  "https://oauth2.googleapis.com/token",
	AuthStyle: oauth2.AuthStyleInParams,
}

// googleUserURL is Google's OpenID Connect userinfo endpoint. It answers
// the sub claim (the stable account id) plus name and email — email only
// when the email scope was granted, which the default scopes include.
const googleUserURL = "https://openidconnect.googleapis.com/v1/userinfo"

// Google is the Provider for Google accounts through OpenID Connect. The
// zero-value configuration falls back to the public endpoints and the
// openid/email/profile scope set, so only ClientID, ClientSecret and
// RedirectURL are mandatory.
type Google struct {
	oauth2.Config

	// UserURL overrides the userinfo endpoint; empty means
	// https://openidconnect.googleapis.com/v1/userinfo. Tests point it
	// elsewhere.
	UserURL string
}

// Name implements Provider.
func (g *Google) Name() string { return "google" }

// config returns a copy of the embedded configuration with the endpoint
// gaps and the scope default filled in.
func (g *Google) config() *oauth2.Config {
	cfg := g.Config
	if cfg.Endpoint.AuthURL == "" {
		cfg.Endpoint.AuthURL = googleEndpoint.AuthURL
		cfg.Endpoint.AuthStyle = googleEndpoint.AuthStyle
	}
	if cfg.Endpoint.TokenURL == "" {
		cfg.Endpoint.TokenURL = googleEndpoint.TokenURL
		cfg.Endpoint.AuthStyle = googleEndpoint.AuthStyle
	}
	if len(cfg.Scopes) == 0 {
		cfg.Scopes = []string{"openid", "email", "profile"}
	}
	return &cfg
}

// BuildAuthURL implements Provider.
func (g *Google) BuildAuthURL(state string) string {
	return g.config().AuthCodeURL(state)
}

// Exchange implements Provider.
func (g *Google) Exchange(ctx context.Context, code string) (*oauth2.Token, error) {
	token, err := g.config().Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("authclient: google: exchange code: %w", err)
	}
	return token, nil
}

// googleUser is the part of the userinfo answer the identity maps onto; the
// rest lives in Identity.Raw. sub is the account's stable identifier, name
// and email may be absent depending on the granted scopes.
type googleUser struct {
	Sub   string `json:"sub"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// FetchIdentity implements Provider.
func (g *Google) FetchIdentity(ctx context.Context, token *oauth2.Token) (*Identity, error) {
	if token == nil {
		return nil, errors.New("authclient: google: fetch identity: no token")
	}
	userURL := g.UserURL
	if userURL == "" {
		userURL = googleUserURL
	}
	user, raw, err := fetchUser[googleUser](ctx, g.config().Client(ctx, token), userURL)
	if err != nil {
		return nil, fmt.Errorf("authclient: google: %w", err)
	}
	if user.Sub == "" {
		return nil, errors.New("authclient: google: userinfo payload carries no sub claim")
	}
	return &Identity{
		Provider: g.Name(),
		ID:       user.Sub,
		Name:     user.Name,
		Email:    user.Email,
		Raw:      raw,
	}, nil
}

// The provider interface is part of the contract; keep the compiler honest.
var _ Provider = (*Google)(nil)
