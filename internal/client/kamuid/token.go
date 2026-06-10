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

// Refresh exchanges a refresh token for a new TokenSet.
func (c *Client) Refresh(ctx context.Context, clientID, refreshToken string) (*TokenSet, error) {
	disco, err := c.Discovery(ctx)
	if err != nil {
		return nil, err
	}
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)
	form.Set("client_id", clientID)

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
	Issuer        string         `json:"iss"`
	Subject       string         `json:"sub"`
	Audience      json.RawMessage `json:"aud"`
	ExpiresAt     int64          `json:"exp"`
	IssuedAt      int64          `json:"iat"`
	Email         string         `json:"email,omitempty"`
	EmailVerified bool           `json:"email_verified,omitempty"`
	Name          string         `json:"name,omitempty"`
	Picture       string         `json:"picture,omitempty"`
	Organizations []Organization `json:"organizations,omitempty"`
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
