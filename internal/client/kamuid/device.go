package kamuid

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const DeviceCodeGrantType = "urn:ietf:params:oauth:grant-type:device_code"

type DeviceAuth struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
}

func (d *DeviceAuth) ExpiresAt() time.Time {
	return time.Now().Add(time.Duration(d.ExpiresIn) * time.Second)
}

func (d *DeviceAuth) PollInterval() time.Duration {
	if d.Interval <= 0 {
		return 5 * time.Second
	}
	return time.Duration(d.Interval) * time.Second
}

// StartDeviceAuth begins the RFC 8628 device authorization flow.
func (c *Client) StartDeviceAuth(ctx context.Context, clientID, scope string) (*DeviceAuth, error) {
	disco, err := c.Discovery(ctx)
	if err != nil {
		return nil, err
	}

	form := url.Values{}
	form.Set("client_id", clientID)
	if scope != "" {
		form.Set("scope", scope)
	}

	req, err := http.NewRequest(http.MethodPost, disco.DeviceAuthorizationEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("build device-auth request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.do(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("device-auth: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, decodeOAuthError(resp)
	}

	var d DeviceAuth
	if err := json.NewDecoder(resp.Body).Decode(&d); err != nil {
		return nil, fmt.Errorf("decode device-auth: %w", err)
	}
	if d.DeviceCode == "" {
		return nil, fmt.Errorf("device-auth response missing device_code")
	}
	return &d, nil
}

// PollDeviceToken polls the token endpoint at the device-auth's `interval`
// (bumped by 5s on `slow_down`) until tokens arrive, the user denies, the
// device_code expires, or ctx is cancelled. `resource` (RFC 8707) is forwarded
// to the token request: pass the kamuhub audience so the access token comes back
// as an audience-bound JWT the BFF can verify locally (kamuhub ADR 0006); "" for
// the default opaque token.
func (c *Client) PollDeviceToken(ctx context.Context, clientID, resource string, da *DeviceAuth) (*TokenSet, error) {
	disco, err := c.Discovery(ctx)
	if err != nil {
		return nil, err
	}

	interval := da.PollInterval()
	deadline := da.ExpiresAt()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(interval):
		}

		if time.Now().After(deadline) {
			return nil, &OAuthError{Code: "expired_token", Description: "device_code expired before approval"}
		}

		ts, err := c.exchangeDeviceCode(ctx, disco.TokenEndpoint, clientID, resource, da.DeviceCode)
		if err == nil {
			return ts, nil
		}
		switch {
		case IsOAuthCode(err, "authorization_pending"):
			continue
		case IsOAuthCode(err, "slow_down"):
			interval += 5 * time.Second
			continue
		default:
			return nil, err
		}
	}
}

func (c *Client) exchangeDeviceCode(ctx context.Context, tokenEndpoint, clientID, resource, deviceCode string) (*TokenSet, error) {
	form := url.Values{}
	form.Set("grant_type", DeviceCodeGrantType)
	form.Set("device_code", deviceCode)
	form.Set("client_id", clientID)
	if resource != "" {
		form.Set("resource", resource)
	}

	req, err := http.NewRequest(http.MethodPost, tokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.do(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, decodeOAuthError(resp)
	}

	var ts TokenSet
	if err := json.NewDecoder(resp.Body).Decode(&ts); err != nil {
		return nil, fmt.Errorf("decode token response: %w", err)
	}
	return &ts, nil
}

func decodeOAuthError(resp *http.Response) error {
	var e OAuthError
	if err := json.NewDecoder(resp.Body).Decode(&e); err != nil || e.Code == "" {
		return fmt.Errorf("HTTP %d from %s", resp.StatusCode, resp.Request.URL)
	}
	e.Status = resp.StatusCode
	return &e
}
