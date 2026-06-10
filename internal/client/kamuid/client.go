// Package kamuid is a tiny client for the kamuid OAuth 2.0 / OIDC provider.
// It handles discovery, the device authorization grant (RFC 8628), token
// refresh, and unauthenticated ID-token claim parsing.
package kamuid

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"
)

const DefaultIssuer = "http://localhost:8000"

type Client struct {
	Issuer string
	HTTP   *http.Client

	mu    sync.Mutex
	disco *Discovery
}

func New(issuer string) *Client {
	if issuer == "" {
		issuer = DefaultIssuer
	}
	return &Client{
		Issuer: issuer,
		HTTP:   &http.Client{Timeout: 30 * time.Second},
	}
}

// OAuthError is a parsed OAuth 2.0 / RFC 8628 error envelope.
type OAuthError struct {
	Code        string `json:"error"`
	Description string `json:"error_description"`
	URI         string `json:"error_uri,omitempty"`
	Status      int    `json:"-"`
}

func (e *OAuthError) Error() string {
	if e.Description != "" {
		return fmt.Sprintf("%s: %s", e.Code, e.Description)
	}
	return e.Code
}

// IsOAuthCode reports whether err is an OAuthError with the given code.
func IsOAuthCode(err error, code string) bool {
	var o *OAuthError
	return errors.As(err, &o) && o.Code == code
}

func (c *Client) do(ctx context.Context, req *http.Request) (*http.Response, error) {
	req = req.WithContext(ctx)
	req.Header.Set("Accept", "application/json")
	return c.HTTP.Do(req)
}
