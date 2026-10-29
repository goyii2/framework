package authclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"golang.org/x/oauth2"
)

// Identity is the third-party identity a provider reports for the user who
// just authorized the application: who they are at that provider, not who
// they are locally. It is deliberately not web.Identity — turning a provider
// identity into the application's own user model (finding or creating the
// account behind Provider+ID, then logging in through the regular web.User
// path) is application policy and stays in the application layer.
type Identity struct {
	// Provider names the provider the identity comes from (the Provider
	// interface's Name, e.g. "github"). Together with ID it forms the
	// stable composite key an account-linking table keys on.
	Provider string

	// ID is the user's identifier inside the provider, stable across name
	// and email changes (the numeric id for GitHub, the sub claim for
	// Google).
	ID string

	// Name is the provider-reported display name; it may be empty when the
	// provider has none on file.
	Name string

	// Email is the provider-reported address; it may be empty (scope not
	// granted, no address on file). Nothing here proves the address
	// belongs to anyone beyond the provider's own verification.
	Email string

	// Raw is the provider's userinfo payload, decoded as JSON, for fields
	// the struct does not surface (avatar URL, profile URL, ...).
	Raw map[string]any
}

// Provider is one third-party authorization server, thin over an
// oauth2.Config: the interface covers the three steps of a redirect flow —
// build the authorization URL, exchange the code for a token, fetch who the
// token belongs to. Implementations for GitHub and Google ship with the
// package; an application wires instances at startup and reaches them from
// its login and callback actions.
type Provider interface {
	// Name returns the provider's identifier ("github", "google", ...),
	// echoed on every Identity it produces.
	Name() string

	// BuildAuthURL returns the URL to redirect the user agent to for
	// authorization; state is the CSRF token the callback must present
	// back (GenerateState / ValidState).
	BuildAuthURL(state string) string

	// Exchange trades the authorization code the callback received for
	// the provider's tokens.
	Exchange(ctx context.Context, code string) (*oauth2.Token, error)

	// FetchIdentity asks the provider's userinfo endpoint who token
	// belongs to.
	FetchIdentity(ctx context.Context, token *oauth2.Token) (*Identity, error)
}

// maxUserinfoBytes bounds how much of a userinfo response is read; a
// provider endpoint should never answer with megabytes, and a runaway or
// hostile endpoint must not be able to drain memory through the reader.
const maxUserinfoBytes = 1 << 20

// fetchUser GETs the userinfo endpoint with client — an oauth2 client, so
// the access token travels in the Authorization header — and decodes the
// answer into both typed fields and the raw payload map. A non-2xx answer
// is an error carrying the status and a trimmed excerpt of the body.
func fetchUser[T any](ctx context.Context, client *http.Client, url string) (user T, raw map[string]any, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return user, nil, fmt.Errorf("build userinfo request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return user, nil, fmt.Errorf("request userinfo: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxUserinfoBytes))
	if err != nil {
		return user, nil, fmt.Errorf("read userinfo response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		excerpt := string(body)
		if len(excerpt) > 200 {
			excerpt = excerpt[:200]
		}
		return user, nil, fmt.Errorf("userinfo endpoint returned %s: %s", resp.Status, excerpt)
	}
	if err := json.Unmarshal(body, &user); err != nil {
		return user, nil, fmt.Errorf("decode userinfo payload: %w", err)
	}
	// A payload that is not a JSON object (an array, a bare string) is not
	// usable as Raw; the typed decode above has already judged the shape.
	_ = json.Unmarshal(body, &raw)
	return user, raw, nil
}
