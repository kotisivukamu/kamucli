package kamustatus

import (
	"context"
	"encoding/json"
)

// PagesCNAMETarget is where a status page's custom domain must point. It is
// the kamustatus pages service; the API echoes it as `cname` on domain reads.
const PagesCNAMETarget = "pages.kamustatus.com"

// StatusPage is a project's status page. CustomDomain is nil when none is set;
// CustomDomainVerified is nil when there is no domain, else whether its DNS
// currently points at PagesCNAMETarget.
type StatusPage struct {
	ID                   string              `json:"id"`
	ProjectID            string              `json:"project_id"`
	Name                 string              `json:"name"`
	Slug                 string              `json:"slug"`
	Public               bool                `json:"public"`
	Title                *string             `json:"title"`
	LogoURL              *string             `json:"logo_url"`
	CustomCSS            *string             `json:"custom_css"`
	CustomDomain         *string             `json:"custom_domain"`
	CustomDomainVerified *bool               `json:"custom_domain_verified"`
	PrivacyURL           *string             `json:"privacy_url"`
	FooterText           *string             `json:"footer_text"`
	CNAME                string              `json:"cname,omitempty"`
	MonitorCount         int                 `json:"monitor_count,omitempty"`
	Monitors             []StatusPageMonitor `json:"monitors,omitempty"`
	CreatedAt            string              `json:"created_at"`
	UpdatedAt            string              `json:"updated_at"`
}

// StatusPageMonitor is a monitor as listed on a single status-page read, in
// display order.
type StatusPageMonitor struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Type         string `json:"type"`
	Target       string `json:"target"`
	DisplayOrder int    `json:"display_order"`
}

// PlatformURL is the page's always-available address, independent of any
// custom domain.
func (p *StatusPage) PlatformURL() string {
	return "https://" + p.Slug + ".kamustatus.com"
}

// Domain is a custom status-page domain and its DNS verification state.
// Reading domains re-checks DNS server-side, so Verified is current.
type Domain struct {
	ID             string  `json:"id"`
	ProjectID      string  `json:"project_id"`
	Domain         string  `json:"domain"`
	Verified       bool    `json:"verified"`
	StatusPageID   *string `json:"status_page_id"`
	StatusPageName *string `json:"status_page_name"`
	StatusPageSlug *string `json:"status_page_slug"`
	CNAME          string  `json:"cname"`
	CreatedAt      string  `json:"created_at"`
}

func (c *Client) ListStatusPages(ctx context.Context, projectID string) ([]StatusPage, error) {
	data, err := c.Do(ctx, "GET", "/projects/"+projectID+"/status-pages", nil)
	if err != nil {
		return nil, err
	}
	var out []StatusPage
	return out, json.Unmarshal(data, &out)
}

// CreateStatusPage takes the API's camelCase body (name, slug, title, logoUrl,
// customCss, customDomain, privacyUrl, footerText, monitorIds).
func (c *Client) CreateStatusPage(ctx context.Context, projectID string, input map[string]any) (*StatusPage, error) {
	return c.pageCall(ctx, "POST", "/projects/"+projectID+"/status-pages", input)
}

func (c *Client) GetStatusPage(ctx context.Context, id string) (*StatusPage, error) {
	return c.pageCall(ctx, "GET", "/status-pages/"+id, nil)
}

// UpdateStatusPage PATCHes a page. customDomain, privacyUrl and footerText ""
// clear the value;
// monitorIds, when present, replaces the whole set in the given order.
func (c *Client) UpdateStatusPage(ctx context.Context, id string, updates map[string]any) (*StatusPage, error) {
	return c.pageCall(ctx, "PATCH", "/status-pages/"+id, updates)
}

func (c *Client) DeleteStatusPage(ctx context.Context, id string) error {
	_, err := c.Do(ctx, "DELETE", "/status-pages/"+id, nil)
	return err
}

func (c *Client) pageCall(ctx context.Context, method, path string, body any) (*StatusPage, error) {
	data, err := c.Do(ctx, method, path, body)
	if err != nil {
		return nil, err
	}
	var p StatusPage
	return &p, json.Unmarshal(data, &p)
}

func (c *Client) ListDomains(ctx context.Context, projectID string) ([]Domain, error) {
	data, err := c.Do(ctx, "GET", "/projects/"+projectID+"/domains", nil)
	if err != nil {
		return nil, err
	}
	var out []Domain
	return out, json.Unmarshal(data, &out)
}

// VerifyDomain re-checks one domain's DNS now and returns its fresh state.
func (c *Client) VerifyDomain(ctx context.Context, id string) (*Domain, error) {
	data, err := c.Do(ctx, "POST", "/domains/"+id+"/verify", nil)
	if err != nil {
		return nil, err
	}
	var d Domain
	return &d, json.Unmarshal(data, &d)
}
