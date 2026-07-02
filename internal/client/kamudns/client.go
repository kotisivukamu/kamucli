// Package kamudns is a thin HTTP client for kamudns, reached THROUGH the kamuhub
// front door (app.kamuhub.com) so the request is journaled (and, later, gated)
// before it reaches the product. The CLI sends a kamuhub access key as the
// bearer; the BFF verifies it, checks revocation, and forwards to kamudns (which
// re-verifies via JWKS). Paths use the BFF's DNS prefixes: /api/dns/zones and
// /api/dns/v1/... (the BFF maps /api/dns/v1 -> kamudns's /api/v1). Override the
// base with KAMU_KAMUDNS_URL to talk to kamudns directly (local dev / fallback).
package kamudns

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

// Zone is one managed domain (kamudns calls it a "zone" on the dashboard
// surface: GET /api/dns/zones).
type Zone struct {
	ID          string `json:"id"`
	Domain      string `json:"domain"`
	TeamID      string `json:"team_id"`
	KamuidOrgID string `json:"kamuid_org_id"`
	Status      string `json:"nordname_status"`
	ExpiresAt   string `json:"expires_at"`
	AutoRenew   bool   `json:"auto_renew"`
}

type Record struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Type    string `json:"type"`
	TTL     int    `json:"ttl"`
	Content string `json:"content"`
}

// RecordInput is the add-record body. Name defaults to "@" and TTL to 3600
// server-side when omitted.
type RecordInput struct {
	Type    string `json:"type"`
	Name    string `json:"name,omitempty"`
	Content string `json:"content"`
	TTL     int    `json:"ttl,omitempty"`
}

type SearchPrices struct {
	Register *float64 `json:"register"`
	Transfer *float64 `json:"transfer"`
	Renew    *float64 `json:"renew"`
}

type SearchItem struct {
	Domain    string        `json:"domain"`
	Available bool          `json:"available"`
	IsPremium bool          `json:"is_premium"`
	Prices    *SearchPrices `json:"prices"`
}

func (c *Client) do(ctx context.Context, method, path string, body any, out any) error {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, rdr)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Key)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
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
		if json.Unmarshal(data, &e) == nil && e.Error != "" {
			return fmt.Errorf("kamudns: %s (HTTP %d)", e.Error, resp.StatusCode)
		}
		return fmt.Errorf("kamudns: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	if out != nil {
		return json.Unmarshal(data, out)
	}
	return nil
}

// Zones lists the caller's managed domains across their org(s).
func (c *Client) Zones(ctx context.Context) ([]Zone, error) {
	var r struct {
		Zones []Zone `json:"zones"`
	}
	if err := c.do(ctx, "GET", "/api/dns/zones", nil, &r); err != nil {
		return nil, err
	}
	return r.Zones, nil
}

// Domain returns the raw domain detail JSON (kamudns syncs it from the registrar
// on read); we pretty-print it rather than model every field.
func (c *Client) Domain(ctx context.Context, domainID string) (json.RawMessage, error) {
	var raw json.RawMessage
	if err := c.do(ctx, "GET", "/api/dns/v1/domains/"+domainID, nil, &raw); err != nil {
		return nil, err
	}
	return raw, nil
}

// Records lists the DNS records of a managed domain.
func (c *Client) Records(ctx context.Context, domainID string) ([]Record, error) {
	var r struct {
		Records []Record `json:"records"`
	}
	if err := c.do(ctx, "GET", "/api/dns/v1/domains/"+domainID+"/dns", nil, &r); err != nil {
		return nil, err
	}
	return r.Records, nil
}

// AddRecord creates a DNS record and returns it.
func (c *Client) AddRecord(ctx context.Context, domainID string, in RecordInput) (*Record, error) {
	var rec Record
	if err := c.do(ctx, "POST", "/api/dns/v1/domains/"+domainID+"/dns", in, &rec); err != nil {
		return nil, err
	}
	return &rec, nil
}

// DeleteRecord removes a DNS record by id.
func (c *Client) DeleteRecord(ctx context.Context, domainID, recordID string) error {
	return c.do(ctx, "DELETE", "/api/dns/v1/domains/"+domainID+"/dns/"+recordID, nil, nil)
}

// Search checks domain availability across TLDs for a base query.
func (c *Client) Search(ctx context.Context, query string, limit int) ([]SearchItem, error) {
	body := map[string]any{"domain": query}
	if limit > 0 {
		body["limit"] = limit
	}
	var r struct {
		Results []SearchItem `json:"results"`
		Total   int          `json:"total"`
	}
	if err := c.do(ctx, "POST", "/api/dns/v1/domains/search", body, &r); err != nil {
		return nil, err
	}
	return r.Results, nil
}
