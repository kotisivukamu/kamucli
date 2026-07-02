// Package kamusites is a thin HTTP client for kamusites, reached THROUGH the
// kamuhub front door (app.kamuhub.com) so the request is journaled (and, later,
// gated) before it reaches the product. The CLI sends a kamuhub access key as
// the bearer; the BFF verifies it, checks revocation, and forwards to kamusites
// (which re-verifies via JWKS). Paths (/api/sites, /api/teams) already match the
// BFF's prefixes. Override the base with KAMU_KAMUSITES_URL to talk to kamusites
// directly (local dev / fallback).
package kamusites

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
)

const DefaultBaseURL = "https://app.kamuhub.com"

// APIError is a kamusites API failure. StatusCode lets callers map specific
// statuses to friendlier guidance (e.g. 409 from git-credentials = the site
// has no repo yet); Error() keeps the plain rendering everyone else prints.
type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("kamusites: %s (HTTP %d)", e.Message, e.StatusCode)
	}
	return fmt.Sprintf("kamusites: HTTP %d", e.StatusCode)
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

type Team struct {
	ID          string `json:"id"`
	KamuidOrgID string `json:"kamuid_org_id"`
	Name        string `json:"name"`
	Slug        string `json:"slug"`
}

type Site struct {
	ID      string `json:"id"`
	TeamID  string `json:"team_id"`
	Name    string `json:"name"`
	Slug    string `json:"slug"`
	Domain  string `json:"domain"`
	IsDraft bool   `json:"is_draft"`
}

// Brief is the customer brief the builder consumes (kamusites lib/brief.ts
// BriefInput, mirroring the dashboard's create wizard). All fields optional;
// business_name backstops to the site name.
type Brief struct {
	BusinessName           string       `json:"business_name,omitempty"`
	BusinessType           string       `json:"business_type,omitempty"`
	Tagline                string       `json:"tagline,omitempty"`
	Industry               string       `json:"industry,omitempty"`
	Description            string       `json:"description,omitempty"`
	OpeningHours           string       `json:"opening_hours,omitempty"`
	Email                  string       `json:"email,omitempty"`
	Phone                  string       `json:"phone,omitempty"`
	Address                string       `json:"address,omitempty"`
	ExistingURL            string       `json:"existing_url,omitempty"`
	Services               []string     `json:"services,omitempty"`
	Pages                  []string     `json:"pages,omitempty"`
	Tone                   string       `json:"tone,omitempty"`
	Colors                 *BriefColors `json:"colors,omitempty"`
	AdditionalInstructions string       `json:"additional_instructions,omitempty"`
}

type BriefColors struct {
	Primary string `json:"primary,omitempty"`
}

type Build struct {
	ID           string `json:"id"`
	Status       string `json:"status"` // pending | running | success | failed | cancelled
	CurrentStep  string `json:"current_step"`
	ErrorMessage string `json:"error_message"`
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
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+"/api"+path, rdr)
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

func (c *Client) Teams(ctx context.Context) ([]Team, error) {
	var r struct {
		Teams []Team `json:"teams"`
	}
	return r.Teams, c.do(ctx, "GET", "/teams", nil, &r)
}

// Sites lists every site the key can see (RLS already scopes it to the key's
// teams + guest grants across orgs), newest first as the API returns them.
func (c *Client) Sites(ctx context.Context) ([]Site, error) {
	var r struct {
		Sites []Site `json:"sites"`
	}
	return r.Sites, c.do(ctx, "GET", "/sites", nil, &r)
}

// DeleteSite removes a site by id: the API drops the DB row, archives the
// Forgejo repo, and takes it offline on the CDN. 404 when the id is unknown or
// the key isn't a team admin for it (RLS hides both cases identically).
func (c *Client) DeleteSite(ctx context.Context, id string) error {
	return c.do(ctx, "DELETE", "/sites/"+id, nil, nil)
}

func (c *Client) CreateSite(ctx context.Context, teamID, name string) (*Site, error) {
	var r struct {
		Site Site `json:"site"`
	}
	err := c.do(ctx, "POST", "/sites", map[string]string{"team_id": teamID, "name": name}, &r)
	if err != nil {
		return nil, err
	}
	return &r.Site, nil
}

func (c *Client) TriggerBuild(ctx context.Context, siteID string, brief Brief, instructions string) (*Build, error) {
	var r struct {
		Build Build `json:"build"`
	}
	body := map[string]any{"brief": brief}
	if instructions != "" {
		body["instructions"] = instructions
	}
	err := c.do(ctx, "POST", "/sites/"+siteID+"/builds", body, &r)
	if err != nil {
		return nil, err
	}
	return &r.Build, nil
}

// GitCredentials is a short-lived, repo-scoped credential for a site's git
// smart-HTTP remote, minted per operation by POST /sites/{id}/git-credentials.
type GitCredentials struct {
	CloneURL      string `json:"clone_url"`
	Username      string `json:"username"` // cosmetic; the proxy reads only the password
	Password      string `json:"password"` // ~2h repo-scoped JWT
	ExpiresAt     string `json:"expires_at"`
	DefaultBranch string `json:"default_branch"`
	RepoOwner     string `json:"repo_owner"`
	RepoName      string `json:"repo_name"`
}

// GitCredentials mints git credentials for the site's repo. Grant-checked
// server-side on sites.update (any credential the git proxy accepts can push).
// NEVER persist the password — hand it to git in memory and let it expire.
// 404 = site not in the key's scope; 409 = the site has no repo yet.
func (c *Client) GitCredentials(ctx context.Context, siteID string) (*GitCredentials, error) {
	var r GitCredentials
	if err := c.do(ctx, "POST", "/sites/"+siteID+"/git-credentials", nil, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

// UploadMaterial uploads one file to a site's materials (Tigris, keyed by site
// id). The builder downloads the whole materials/<siteId>/ prefix into the Astro
// project's media dir at build time. A logo is just a material named logo.<ext>:
// pass pin="logo.png" to fix the stored name so the builder's logo/favicon
// detection picks it up; pin="" keeps the original filename.
func (c *Client) UploadMaterial(ctx context.Context, siteID, fileName, pin string, data []byte) (string, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", fileName)
	if err != nil {
		return "", err
	}
	if _, err := fw.Write(data); err != nil {
		return "", err
	}
	if pin != "" {
		_ = mw.WriteField("filename", pin)
	}
	if err := mw.Close(); err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", c.BaseURL+"/api/sites/"+siteID+"/materials", &buf)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.Key)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("kamusites materials: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var r struct {
		Filename string `json:"filename"`
	}
	_ = json.Unmarshal(body, &r)
	return r.Filename, nil
}

// GenerateLogo asks kamusites to AI-generate a logo (stored as logo.png).
func (c *Client) GenerateLogo(ctx context.Context, siteID, prompt string) (string, error) {
	var r struct {
		Filename string `json:"filename"`
	}
	err := c.do(ctx, "POST", "/sites/"+siteID+"/logo/generate", map[string]string{"prompt": prompt}, &r)
	return r.Filename, err
}

func (c *Client) LatestBuild(ctx context.Context, siteID string) (*Build, error) {
	var r struct {
		Builds []Build `json:"builds"`
	}
	if err := c.do(ctx, "GET", "/sites/"+siteID+"/builds", nil, &r); err != nil {
		return nil, err
	}
	if len(r.Builds) == 0 {
		return nil, nil
	}
	return &r.Builds[0], nil
}
