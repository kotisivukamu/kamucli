package kamuid

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type TokenSet struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
	IDToken      string `json:"id_token,omitempty"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
}

func (t *TokenSet) AccessExpiresAt() time.Time {
	if t.ExpiresIn <= 0 {
		return time.Time{}
	}
	return time.Now().Add(time.Duration(t.ExpiresIn) * time.Second)
}

// RefreshRequest parameterizes the refresh-token grant.
type RefreshRequest struct {
	ClientID     string
	RefreshToken string
	// Scope, when set, must be the same or a NARROWER set than the original
	// grant, or kamuid rejects with `invalid_scope` ("unable to issue scope X").
	// Careful: kamuid rotates the refresh token on every refresh and the rotated
	// token carries exactly the scopes requested here — narrowing permanently
	// narrows the stored grant. Empty = keep the original scope set.
	Scope string
	// Resource is an RFC 8707 resource indicator: when set (and valid per
	// kamuid's validAudiences) the access token comes back as an audience-bound
	// JWT instead of an opaque token.
	Resource string
}

// Refresh runs the refresh-token grant and returns the new TokenSet. The
// response's RefreshToken is a ROTATED replacement (the presented one is
// revoked server-side, and reusing it kills the whole token family) — callers
// must persist it.
func (c *Client) Refresh(ctx context.Context, r RefreshRequest) (*TokenSet, error) {
	disco, err := c.Discovery(ctx)
	if err != nil {
		return nil, err
	}
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", r.RefreshToken)
	form.Set("client_id", r.ClientID)
	if r.Scope != "" {
		form.Set("scope", r.Scope)
	}
	if r.Resource != "" {
		form.Set("resource", r.Resource)
	}

	req, err := http.NewRequest(http.MethodPost, disco.TokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("build refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.do(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("refresh: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, decodeOAuthError(resp)
	}

	var ts TokenSet
	if err := json.NewDecoder(resp.Body).Decode(&ts); err != nil {
		return nil, fmt.Errorf("decode refresh response: %w", err)
	}
	return &ts, nil
}

// Organization is a member of the `organizations` claim on the ID token.
type Organization struct {
	ID   string `json:"id"`
	Slug string `json:"slug"`
	Name string `json:"name"`
	Role string `json:"role"`
}

// IDTokenClaims are the subset of OIDC claims kamu-cli reads.
type IDTokenClaims struct {
	Issuer        string          `json:"iss"`
	Subject       string          `json:"sub"`
	Audience      json.RawMessage `json:"aud"`
	ExpiresAt     int64           `json:"exp"`
	IssuedAt      int64           `json:"iat"`
	Email         string          `json:"email,omitempty"`
	EmailVerified bool            `json:"email_verified,omitempty"`
	Name          string          `json:"name,omitempty"`
	Picture       string          `json:"picture,omitempty"`
	Organizations []Organization  `json:"organizations,omitempty"`
}

// ParseIDTokenClaims decodes the JWT payload WITHOUT verifying the signature.
// Use only for display (whoami) and never for authorization decisions.
func ParseIDTokenClaims(idToken string) (*IDTokenClaims, error) {
	parts := strings.Split(idToken, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid JWT: want 3 segments, got %d", len(parts))
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("decode JWT payload: %w", err)
	}
	var c IDTokenClaims
	if err := json.Unmarshal(payload, &c); err != nil {
		return nil, fmt.Errorf("parse JWT claims: %w", err)
	}
	return &c, nil
}
