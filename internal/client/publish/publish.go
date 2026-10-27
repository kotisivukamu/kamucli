// Package publish is a thin client for kamuhub's file-publish provisioning
// routes (/api/files/publishes). Publishing a file means kamuhub creates a forge
// repo wired to the kamucdn files plane; this client only asks for that wiring —
// the bytes themselves never pass through kamuhub, they go out over git.
//
// Auth is a kamuhub access key sent as the bearer, like every other product
// command.
package publish

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	base string
	key  string
	http *http.Client
}

func New(base, key string) *Client {
	return &Client{
		base: strings.TrimRight(base, "/"),
		key:  key,
		http: &http.Client{Timeout: 60 * time.Second},
	}
}

// APIError carries the status so a caller can map an expected condition to
// better guidance (401 = re-login, 403 = the key lacks files.manage).
type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("kamuhub: %s (HTTP %d)", e.Message, e.StatusCode)
	}
	return fmt.Sprintf("kamuhub: HTTP %d", e.StatusCode)
}

// Publish is one published file as kamuhub reports it. Path is where in the repo
// the page must live for the repo's workflow to deploy it — read from the server
// rather than assumed, so the repo layout can change without a new CLI.
type Publish struct {
	ID        string    `json:"id"`
	Org       string    `json:"org"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	FileID    string    `json:"file_id"`
	Repo      string    `json:"repo"`
	CloneURL  string    `json:"clone_url"`
	PublicURL string    `json:"public_url"`
	Branch    string    `json:"default_branch"`
	Path      string    `json:"path"`
	CreatedAt time.Time `json:"created_at"`
}

func (c *Client) do(ctx context.Context, method, path string, body any, out any) error {
	var rdr io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, rdr)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.key)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("reach kamuhub (%s): %w", c.base, err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 400 {
		var e struct {
			Error string `json:"error"`
		}
		msg := strings.TrimSpace(string(data))
		if json.Unmarshal(data, &e) == nil && e.Error != "" {
			msg = e.Error
		}
		return &APIError{StatusCode: resp.StatusCode, Message: msg}
	}
	if out != nil && len(data) > 0 {
		return json.Unmarshal(data, out)
	}
	return nil
}

// orgQuery renders the optional ?org=<slug>. An empty slug lets the server
// resolve a single-org key itself.
func orgQuery(org string) string {
	if org == "" {
		return ""
	}
	return "?org=" + url.QueryEscape(org)
}

// Create provisions a publish repo for one file and returns where it lives.
func (c *Client) Create(ctx context.Context, org, name string) (*Publish, error) {
	var out Publish
	err := c.do(ctx, http.MethodPost, "/api/files/publishes"+orgQuery(org),
		map[string]string{"name": name}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// List returns the org's published files, newest first.
func (c *Client) List(ctx context.Context, org string) ([]Publish, error) {
	var out struct {
		Publishes []Publish `json:"publishes"`
	}
	if err := c.do(ctx, http.MethodGet, "/api/files/publishes"+orgQuery(org), nil, &out); err != nil {
		return nil, err
	}
	return out.Publishes, nil
}

// Protect sets (or, with an empty password, clears) the password gate on an
// already-deployed page. It only works once the artifact is live, which is why
// the command calls it after the push rather than at create time.
func (c *Client) Protect(ctx context.Context, org, id, password string) error {
	return c.do(ctx, http.MethodPost,
		"/api/files/publishes/"+url.PathEscape(id)+"/protect"+orgQuery(org),
		map[string]string{"password": password}, nil)
}
