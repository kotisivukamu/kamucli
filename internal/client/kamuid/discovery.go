package kamuid

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type Discovery struct {
	Issuer                      string   `json:"issuer"`
	AuthorizationEndpoint       string   `json:"authorization_endpoint"`
	TokenEndpoint               string   `json:"token_endpoint"`
	DeviceAuthorizationEndpoint string   `json:"device_authorization_endpoint"`
	UserinfoEndpoint            string   `json:"userinfo_endpoint"`
	JWKSURI                     string   `json:"jwks_uri"`
	ScopesSupported             []string `json:"scopes_supported"`
	GrantTypesSupported         []string `json:"grant_types_supported"`
}

func (c *Client) Discovery(ctx context.Context) (*Discovery, error) {
	c.mu.Lock()
	cached := c.disco
	c.mu.Unlock()
	if cached != nil {
		return cached, nil
	}

	url := strings.TrimRight(c.Issuer, "/") + "/.well-known/openid-configuration"
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build discovery request: %w", err)
	}
	resp, err := c.do(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("discovery: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("discovery: HTTP %d", resp.StatusCode)
	}
	var d Discovery
	if err := json.NewDecoder(resp.Body).Decode(&d); err != nil {
		return nil, fmt.Errorf("decode discovery: %w", err)
	}
	if d.TokenEndpoint == "" || d.DeviceAuthorizationEndpoint == "" {
		return nil, fmt.Errorf("discovery missing required endpoints")
	}

	c.mu.Lock()
	c.disco = &d
	c.mu.Unlock()
	return &d, nil
}
