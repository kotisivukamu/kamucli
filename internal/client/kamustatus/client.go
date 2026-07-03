// Package kamustatus is a thin HTTP client for kamustatus, reached THROUGH the
// kamuhub front door (app.kamuhub.com) so the request is journaled (and gated)
// before it reaches the product. The CLI sends a kamuhub access key as the
// bearer; the BFF verifies it, checks revocation, injects the signed
// X-Kamuhub-Authz context, and forwards to kamustatus (which re-verifies via
// JWKS). The BFF strips the /api/status prefix and forwards to kamustatus's bare
// /api (so /api/status/projects -> <kamustatus>/api/projects); the client bakes
// that prefix in. Override the base with KAMU_KAMUSTATUS_URL (local dev /
// fallback). Public status pages (/status/:slug) are served by kamustatus
// directly, NOT via the BFF — see PublicBaseURL.
package kamustatus

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const DefaultBaseURL = "https://app.kamuhub.com"

// PublicBaseURL is kamustatus itself. Public status pages (/status/:slug) are
// unauthenticated and served by kamustatus directly, not proxied by the BFF, so
// GetPublic targets this rather than the front door.
const PublicBaseURL = "https://kamustatus-api.fly.dev"

type Client struct {
	BaseURL    string
	Key        string // kamuhub access key, sent as the bearer
	HTTPClient *http.Client
}

func New(baseURL, key string) *Client {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	return &Client{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		Key:        key,
		HTTPClient: http.DefaultClient,
	}
}

// Do issues an authenticated API request and returns the raw response body.
// path must begin with "/" and is appended to BaseURL+"/api/status" — the BFF
// prefix it strips before forwarding to kamustatus's bare /api.
func (c *Client) Do(ctx context.Context, method, path string, body any) (json.RawMessage, error) {
	url := c.BaseURL + "/api/status" + path

	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal request: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+c.Key)
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		var apiErr struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(data, &apiErr) == nil && apiErr.Error != "" {
			return nil, fmt.Errorf("kamustatus: %s", apiErr.Error)
		}
		return nil, fmt.Errorf("kamustatus: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	return json.RawMessage(data), nil
}

// GetPublic fetches an unauthenticated endpoint (e.g. public status pages).
func (c *Client) GetPublic(ctx context.Context, path string) (json.RawMessage, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.BaseURL+path, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("kamustatus: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	return json.RawMessage(data), nil
}

// --- Domain types ---

type Project struct {
	ID                  string `json:"id"`
	Name                string `json:"name"`
	Slug                string `json:"slug"`
	KamuidOrgID         string `json:"kamuid_org_id"`
	PublicStatusEnabled bool   `json:"public_status_enabled"`
	CreatedAt           string `json:"created_at"`
}

type Monitor struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Type            string `json:"type"`
	Target          string `json:"target"`
	Method          string `json:"method"`
	IntervalSeconds int    `json:"interval_seconds"`
	Enabled         bool   `json:"enabled"`
	HeartbeatToken  string `json:"heartbeat_token,omitempty"`
	PingURL         string `json:"pingUrl,omitempty"`
}

// --- Endpoints ---

func (c *Client) ListProjects(ctx context.Context) ([]Project, error) {
	data, err := c.Do(ctx, "GET", "/projects", nil)
	if err != nil {
		return nil, err
	}
	var out []Project
	return out, json.Unmarshal(data, &out)
}

func (c *Client) GetProject(ctx context.Context, id string) (json.RawMessage, error) {
	return c.Do(ctx, "GET", "/projects/"+id, nil)
}

func (c *Client) CreateProject(ctx context.Context, name, slug, kamuidOrgID string) (*Project, error) {
	data, err := c.Do(ctx, "POST", "/projects", map[string]string{
		"name": name, "slug": slug, "kamuid_org_id": kamuidOrgID,
	})
	if err != nil {
		return nil, err
	}
	var p Project
	return &p, json.Unmarshal(data, &p)
}

func (c *Client) DeleteProject(ctx context.Context, id string) error {
	_, err := c.Do(ctx, "DELETE", "/projects/"+id, nil)
	return err
}

func (c *Client) ListMonitors(ctx context.Context, projectID string) ([]Monitor, error) {
	data, err := c.Do(ctx, "GET", "/projects/"+projectID+"/monitors", nil)
	if err != nil {
		return nil, err
	}
	var out []Monitor
	return out, json.Unmarshal(data, &out)
}

func (c *Client) CreateMonitor(ctx context.Context, projectID string, input map[string]any) (json.RawMessage, error) {
	return c.Do(ctx, "POST", "/projects/"+projectID+"/monitors", input)
}

func (c *Client) GetMonitor(ctx context.Context, id string) (json.RawMessage, error) {
	return c.Do(ctx, "GET", "/monitors/"+id, nil)
}

func (c *Client) UpdateMonitor(ctx context.Context, id string, updates map[string]any) (json.RawMessage, error) {
	return c.Do(ctx, "PATCH", "/monitors/"+id, updates)
}

func (c *Client) DeleteMonitor(ctx context.Context, id string) error {
	_, err := c.Do(ctx, "DELETE", "/monitors/"+id, nil)
	return err
}

func (c *Client) GetMonitorStats(ctx context.Context, id string, hours int) (json.RawMessage, error) {
	return c.Do(ctx, "GET", fmt.Sprintf("/monitors/%s/stats?hours=%d", id, hours), nil)
}

func (c *Client) ListAlerts(ctx context.Context, monitorID string) (json.RawMessage, error) {
	return c.Do(ctx, "GET", "/monitors/"+monitorID+"/alerts", nil)
}

func (c *Client) CreateAlert(ctx context.Context, monitorID string, input map[string]any) (json.RawMessage, error) {
	return c.Do(ctx, "POST", "/monitors/"+monitorID+"/alerts", input)
}

func (c *Client) DeleteAlert(ctx context.Context, id string) error {
	_, err := c.Do(ctx, "DELETE", "/alerts/"+id, nil)
	return err
}

func (c *Client) GetPublicStatus(ctx context.Context, slug string) (json.RawMessage, error) {
	return c.GetPublic(ctx, "/status/"+slug)
}
