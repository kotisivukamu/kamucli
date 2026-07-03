// Package kamuhub is a thin client for the kamuhub front door (BFF). Its one job
// today is the `kamu auth login` exchange: trade a KamuID audience-bound JWT for
// a kamuhub access key (kamuhub ADR 0006), the credential products + git accept.
package kamuhub

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type Client struct {
	base string
	http *http.Client
}

func New(base string) *Client {
	return &Client{base: base, http: &http.Client{Timeout: 30 * time.Second}}
}

type LoginRequest struct {
	Org        string `json:"org,omitempty"`         // slug/id; "" = the user's first org
	TTLSeconds int    `json:"ttl_seconds,omitempty"` // <=0 => BFF default
	Label      string `json:"label,omitempty"`
}

type LoginResult struct {
	Token string `json:"token"` // the minted access key (shown once)
	Key   struct {
		ID        string `json:"id"`
		Org       string `json:"org"`
		ExpiresAt string `json:"expires_at"` // RFC3339; "" = permanent
		Status    string `json:"status"`
	} `json:"key"`
	User struct {
		ID    string `json:"id"`
		Sub   string `json:"sub"`
		Email string `json:"email"`
		Name  string `json:"name"`
	} `json:"user"`
	Org struct {
		Slug        string   `json:"slug"`
		KamuIDOrgID string   `json:"kamuid_org_id"`
		Grants      []string `json:"grants"`
	} `json:"org"`
}

// Login exchanges a KamuID token (the bearer) for a kamuhub access key via
// POST /api/cli/login. The BFF verifies the token is an audience-bound JWT minted
// for kamuhub before minting the key.
func (c *Client) Login(ctx context.Context, kamuidToken string, req LoginRequest) (*LoginResult, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/api/cli/login", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+kamuidToken)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("reach kamuhub (%s): %w", c.base, err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusCreated {
		return nil, apiError(resp.StatusCode, data)
	}
	var out LoginResult
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("decode login response: %w", err)
	}
	if out.Token == "" {
		return nil, fmt.Errorf("login response missing access key")
	}
	return &out, nil
}

// ExpiresAt parses Key.ExpiresAt; zero time means a permanent key.
func (r *LoginResult) ExpiresAt() time.Time {
	if r.Key.ExpiresAt == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, r.Key.ExpiresAt)
	if err != nil {
		return time.Time{}
	}
	return t
}

func apiError(status int, body []byte) error {
	var e struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(body, &e) == nil && e.Error != "" {
		return fmt.Errorf("kamuhub login failed (HTTP %d): %s", status, e.Error)
	}
	return fmt.Errorf("kamuhub login failed (HTTP %d)", status)
}
