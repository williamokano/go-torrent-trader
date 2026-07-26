package client

import (
	"context"
	"errors"
	"net/http"
	"time"
)

// Tokens is the token pair the site issues, mirroring the AuthTokens schema.
type Tokens struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	// ExpiresIn is seconds until the access token expires.
	ExpiresIn int64 `json:"expires_in"`
}

// ExpiresAt converts the relative lifetime into an absolute instant, so a
// credential written to disk stays meaningful across invocations.
func (t Tokens) ExpiresAt(now time.Time) time.Time {
	if t.ExpiresIn <= 0 {
		return time.Time{}
	}
	return now.Add(time.Duration(t.ExpiresIn) * time.Second).UTC()
}

// authEnvelope is the {user, tokens} wrapper both login and refresh return.
type authEnvelope struct {
	Tokens Tokens `json:"tokens"`
}

// ErrNoTokens reports that an auth call succeeded but returned nothing usable.
var ErrNoTokens = errors.New("site returned no access token")

// Login exchanges a username and password for a token pair.
//
// The password is used once and never stored — only the tokens are. This
// deliberately does not go through Do's refresh path: there is nothing to
// refresh yet, and a 401 here means the credentials were wrong.
func Login(ctx context.Context, baseURL, username, password string, opts ...Option) (Tokens, error) {
	c, err := New(baseURL, opts...)
	if err != nil {
		return Tokens{}, err
	}
	body := map[string]string{"username": username, "password": password}

	var env authEnvelope
	if err := c.Post(ctx, "/api/v1/auth/login", body, &env); err != nil {
		return Tokens{}, err
	}
	if env.Tokens.AccessToken == "" {
		return Tokens{}, ErrNoTokens
	}
	return env.Tokens, nil
}

// Refresh exchanges a refresh token for a new pair.
func Refresh(ctx context.Context, baseURL, refreshToken string, opts ...Option) (Tokens, error) {
	c, err := New(baseURL, opts...)
	if err != nil {
		return Tokens{}, err
	}
	body := map[string]string{"refresh_token": refreshToken}

	var env authEnvelope
	if err := c.Post(ctx, "/api/v1/auth/refresh", body, &env); err != nil {
		return Tokens{}, err
	}
	if env.Tokens.AccessToken == "" {
		return Tokens{}, ErrNoTokens
	}
	return env.Tokens, nil
}

// Logout invalidates the session behind the client's token. The endpoint
// answers 204, so there is nothing to decode.
func (c *Client) Logout(ctx context.Context) error {
	return c.Do(ctx, http.MethodPost, "/api/v1/auth/logout", nil, nil)
}
