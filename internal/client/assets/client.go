// Package assets is a thin HTTP client for the org image store, reached THROUGH
// the kamuhub front door (app.kamuhub.com) so the request is journaled (and,
// later, gated) before it reaches storage. The CLI sends a kamuhub access key
// as the bearer; the BFF verifies it, checks revocation, and enforces the org
// quota. Uploads are raw image bytes — the server sniffs the real content type
// and dedupes by hash. Override the base with KAMU_ASSETS_URL (local dev /
// fallback).
package assets

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const DefaultBaseURL = "https://app.kamuhub.com"

// APIError is an assets API failure. StatusCode lets callers map specific
// statuses to friendlier guidance; Error() keeps the plain rendering everyone
// else prints.
type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("assets: %s (HTTP %d)", e.Message, e.StatusCode)
	}
	return fmt.Sprintf("assets: HTTP %d", e.StatusCode)
}

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

// UploadResult is the server's record of a stored image. Existing means the
// same bytes were already uploaded (content-addressed dedupe) — the URL is
// still valid, nothing new was stored.
type UploadResult struct {
	Status      string `json:"status"`
	Name        string `json:"name"` // <hash>.<ext>
	URL         string `json:"url"`
	Bytes       int64  `json:"bytes"`
	ContentType string `json:"content_type"`
	Existing    bool   `json:"existing"`
}

type UsageResult struct {
	Org        string `json:"org"`
	Usage      Usage  `json:"usage"`
	LimitBytes int64  `json:"limit_bytes"`
}

type Usage struct {
	Bytes     int64  `json:"bytes"`
	Count     int64  `json:"count"`
	UpdatedAt string `json:"updated_at"`
}

// Upload stores one image (jpeg/png/webp/gif, 5MB cap — the server decides by
// sniffing the bytes, not the name). org may be "" when the key's context holds
// exactly one org; the server resolves it then.
func (c *Client) Upload(ctx context.Context, org string, data []byte) (*UploadResult, error) {
	var r UploadResult
	if err := c.do(ctx, "POST", "/assets"+orgQuery(org), bytes.NewReader(data), "application/octet-stream", &r); err != nil {
		return nil, err
	}
	return &r, nil
}

func (c *Client) Usage(ctx context.Context, org string) (*UsageResult, error) {
	var r UsageResult
	if err := c.do(ctx, "GET", "/assets/usage"+orgQuery(org), nil, "", &r); err != nil {
		return nil, err
	}
	return &r, nil
}

func orgQuery(org string) string {
	if org == "" {
		return ""
	}
	return "?org=" + url.QueryEscape(org)
}

func (c *Client) do(ctx context.Context, method, path string, body io.Reader, contentType string, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+"/api"+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Key)
	req.Header.Set("Accept", "application/json")
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
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
		return &APIError{StatusCode: resp.StatusCode, Message: msg}
	}
	if out != nil {
		return json.Unmarshal(data, out)
	}
	return nil
}
