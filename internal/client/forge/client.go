// Package forge is a thin HTTP client for the platform forge-api gateway at
// git.kamuhub.com. Git access is a platform capability (kamuhub ADR 0005): the
// forge hosts repos for ALL project types, so this client is not scoped to any
// product. Auth is a kamuhub access key sent as the bearer; the gateway scopes
// the repo listing to the key's org server-side. The same access key is also
// what git smart-HTTP accepts as the Basic-auth password (no minting step).
// Override the base with KAMU_GIT_URL to talk to a local gateway (dev).
package forge

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

const (
	DefaultBaseURL = "https://git.kamuhub.com"
	// EnvURL overrides the gateway base URL (same pattern as KAMU_KAMUSITES_URL).
	EnvURL = "KAMU_GIT_URL"
)

// BaseURL resolves the gateway base: KAMU_GIT_URL when set, else the default.
// Shared by `kamu clone` (listing) and `kamu git-credential` (host matching) so
// both always agree on which host the access key may be presented to.
func BaseURL() string {
	if v := os.Getenv(EnvURL); v != "" {
		return v
	}
	return DefaultBaseURL
}

// APIError is a forge gateway failure. StatusCode lets callers map specific
// statuses to friendlier guidance (e.g. 401 = invalid/revoked access key);
// Error() keeps the plain rendering everyone else prints.
type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("forge: %s (HTTP %d)", e.Message, e.StatusCode)
	}
	return fmt.Sprintf("forge: HTTP %d", e.StatusCode)
}

type Client struct {
	BaseURL    string
	Key        string // kamuhub access key, sent as the bearer
	HTTPClient *http.Client
}

func New(baseURL, key string) *Client {
	if baseURL == "" {
		baseURL = BaseURL()
	}
	return &Client{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		Key:        key,
		HTTPClient: http.DefaultClient,
	}
}

// Repo is one repository the access key can see, as the gateway lists it.
type Repo struct {
	Owner         string `json:"owner"`
	Name          string `json:"name"`
	Description   string `json:"description"`
	DefaultBranch string `json:"default_branch"`
	CloneURL      string `json:"clone_url"`
}

// FullName is the owner/name form used for `git config kamu.project` and for
// exact matching of a <project> argument.
func (r Repo) FullName() string {
	return r.Owner + "/" + r.Name
}

// Repos lists every repo the key can see (scoped server-side to the key's org).
func (c *Client) Repos(ctx context.Context) ([]Repo, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.BaseURL+"/api/repos", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Key)
	req.Header.Set("Accept", "application/json")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		var e struct {
			Error string `json:"error"`
		}
		msg := strings.TrimSpace(string(data))
		if json.Unmarshal(data, &e) == nil && e.Error != "" {
			msg = e.Error
		}
		return nil, &APIError{StatusCode: resp.StatusCode, Message: msg}
	}
	var r struct {
		Repos []Repo `json:"repos"`
	}
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, err
	}
	return r.Repos, nil
}
