package kamuid

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

// OrgAPI is a client for kamuid's /v1/rp organization resource server — the
// bearer is a KamuID access token audience-bound to the RP API (RFC 8707
// resource = `${issuer}/api/v1/rp`), NOT a kamuhub access key. Reads require
// the kamu.org.profile.read scope, management the kamu.org.manage scope.
// Publicly the API lives under the issuer's /api prefix (the kamuid proxy
// strips it), so paths here are `${issuer}/api/v1/rp/...`.
type OrgAPI struct {
	BaseURL string // e.g. https://accounts.kamuhub.com/api
	Token   string
	HTTP    *http.Client
}

// NewOrgAPI builds an OrgAPI client from the kamuid issuer URL and a bearer
// token minted for the RP API audience.
func NewOrgAPI(issuer, token string) *OrgAPI {
	if issuer == "" {
		issuer = DefaultIssuer
	}
	return &OrgAPI{
		BaseURL: strings.TrimRight(issuer, "/") + "/api",
		Token:   token,
		HTTP:    &http.Client{Timeout: 30 * time.Second},
	}
}

// APIError is a kamuid resource-server error envelope ({error} or
// {error, error_description}).
type APIError struct {
	Status      int
	Code        string `json:"error"`
	Description string `json:"error_description"`
}

func (e *APIError) Error() string {
	if e.Description != "" {
		return fmt.Sprintf("kamuid: %s: %s (HTTP %d)", e.Code, e.Description, e.Status)
	}
	return fmt.Sprintf("kamuid: %s (HTTP %d)", e.Code, e.Status)
}

// Member is one member row of an organization detail.
type Member struct {
	UserID    string `json:"userId"`
	Email     string `json:"email"`
	Name      string `json:"name,omitempty"`
	Role      string `json:"role"`
	CreatedAt string `json:"createdAt,omitempty"`
}

// Invitation is one pending invitation of an organization detail.
type Invitation struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	Role      string `json:"role"`
	Status    string `json:"status,omitempty"`
	ExpiresAt string `json:"expiresAt,omitempty"`
}

// OrgDetail is the org + members + pending invitations shape of
// GET /v1/rp/organizations/{orgId}.
type OrgDetail struct {
	Organization Organization `json:"organization"`
	Members      []Member     `json:"members"`
	Invitations  []Invitation `json:"invitations"`
}

// CreateOrgInput is the POST /v1/rp/organizations body; Slug empty = server
// derives one from the name.
type CreateOrgInput struct {
	Name string `json:"name"`
	Slug string `json:"slug,omitempty"`
}

// InviteInput is the POST /v1/rp/organizations/{orgId}/invitations body.
// Role must be "admin" or "member".
type InviteInput struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

func (c *OrgAPI) do(ctx context.Context, method, path string, body any, out any) error {
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
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 400 {
		var e APIError
		if json.Unmarshal(data, &e) == nil && e.Code != "" {
			e.Status = resp.StatusCode
			return &e
		}
		return fmt.Errorf("kamuid: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("decode kamuid response: %w", err)
		}
	}
	return nil
}

// ListOrgs lists the caller's organizations (GET /v1/rp/organizations).
func (c *OrgAPI) ListOrgs(ctx context.Context) ([]Organization, error) {
	var r struct {
		Organizations []Organization `json:"organizations"`
	}
	if err := c.do(ctx, http.MethodGet, "/v1/rp/organizations", nil, &r); err != nil {
		return nil, err
	}
	return r.Organizations, nil
}

// CreateOrg creates an organization; the caller becomes its owner.
func (c *OrgAPI) CreateOrg(ctx context.Context, in CreateOrgInput) (*Organization, error) {
	var r struct {
		Organization Organization `json:"organization"`
	}
	if err := c.do(ctx, http.MethodPost, "/v1/rp/organizations", in, &r); err != nil {
		return nil, err
	}
	return &r.Organization, nil
}

// GetOrg returns an organization with its members and pending invitations
// (member only).
func (c *OrgAPI) GetOrg(ctx context.Context, orgID string) (*OrgDetail, error) {
	var d OrgDetail
	if err := c.do(ctx, http.MethodGet, "/v1/rp/organizations/"+url.PathEscape(orgID), nil, &d); err != nil {
		return nil, err
	}
	return &d, nil
}

// DeleteOrg deletes an organization (owner only; the server refuses to delete
// the caller's last org).
func (c *OrgAPI) DeleteOrg(ctx context.Context, orgID string) error {
	return c.do(ctx, http.MethodDelete, "/v1/rp/organizations/"+url.PathEscape(orgID), nil, nil)
}

// Invite invites an email to the organization (owner/admin only).
func (c *OrgAPI) Invite(ctx context.Context, orgID string, in InviteInput) (*Invitation, error) {
	var r struct {
		Invitation Invitation `json:"invitation"`
	}
	if err := c.do(ctx, http.MethodPost, "/v1/rp/organizations/"+url.PathEscape(orgID)+"/invitations", in, &r); err != nil {
		return nil, err
	}
	return &r.Invitation, nil
}

// RemoveMember removes a member by user id (owner/admin only; the server
// enforces owner-removal rules).
func (c *OrgAPI) RemoveMember(ctx context.Context, orgID, userID string) error {
	return c.do(ctx, http.MethodDelete,
		"/v1/rp/organizations/"+url.PathEscape(orgID)+"/members/"+url.PathEscape(userID), nil, nil)
}
