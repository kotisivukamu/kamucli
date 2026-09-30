// Package billing is a thin HTTP client for kamuhub's platform billing API,
// reached THROUGH the kamuhub front door (app.kamuhub.com) like every other
// product client: the CLI sends a kamuhub access key as the bearer, the BFF
// verifies it, journals the call and forwards to billing-api with the signed
// X-Kamuhub-Authz context. The BFF keeps the /api/billing prefix (the upstream
// base includes it), so the client bakes it in.
//
// Billing is org-scoped by ?org=<kamuid_org_id> -- the shared, non-divergent
// org key. Not a slug: billing rows are keyed on kamuid_org_id, and the
// command layer maps a slug to it from the access key payload.
package billing

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
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

// --- Domain types ---

type Card struct {
	Brand    string `json:"brand"`
	Last4    string `json:"last4"`
	ExpMonth int    `json:"exp_month"`
	ExpYear  int    `json:"exp_year"`
}

type Subscription struct {
	Product          string `json:"product"`
	PlanKey          string `json:"plan_key"`
	Status           string `json:"status"`
	CurrentPeriodEnd string `json:"current_period_end"`
}

type Summary struct {
	BillingMode   string         `json:"billing_mode"` // card | einvoice | none
	HasCustomer   bool           `json:"has_customer"`
	Card          *Card          `json:"card"`
	Subscriptions []Subscription `json:"subscriptions"`
	Charges       []Charge       `json:"charges"`
}

type Invoice struct {
	ID              string `json:"id"`
	PeriodStart     string `json:"period_start"`
	PeriodEnd       string `json:"period_end"`
	Rail            string `json:"rail"`   // card | einvoice
	Status          string `json:"status"` // draft | issued | paid | void | uncollectible
	TotalCents      int64  `json:"total_cents"`
	Currency        string `json:"currency"`
	StripeInvoiceID string `json:"stripe_invoice_id"`
	ProcountorRef   string `json:"procountor_ref"`
	IssuedAt        string `json:"issued_at"`
	PaidAt          string `json:"paid_at"`
	CreatedAt       string `json:"created_at"`
}

type LineItem struct {
	ID           string `json:"id"`
	Product      string `json:"product"`
	PriceKey     string `json:"price_key"`
	Description  string `json:"description"`
	Qty          string `json:"qty"` // NUMERIC: string, so 0.5 survives the trip
	AmountCents  int64  `json:"amount_cents"`
	Currency     string `json:"currency"`
	ChargeTiming string `json:"charge_timing"`
	Status       string `json:"status"`
	OccurredAt   string `json:"occurred_at"`
}

// InvoiceDetail adds the line items the invoice aggregates and Stripe's own
// documents. PDFURL/HostedURL are empty on the einvoice rail (Procountor
// issues that paperwork) and whenever Stripe is unconfigured.
type InvoiceDetail struct {
	Invoice
	LineItems []LineItem `json:"line_items"`
	PDFURL    string     `json:"pdf_url"`
	HostedURL string     `json:"hosted_url"`
}

type Charge struct {
	ID          string `json:"id"`
	Product     string `json:"product"`
	Description string `json:"description"`
	AmountCents int64  `json:"amount_cents"`
	Currency    string `json:"currency"`
	Status      string `json:"status"` // pending | paid | failed | refunded
	CreatedAt   string `json:"created_at"`
	PaidAt      string `json:"paid_at"`
}

// ChargeDetail adds Stripe's receipt page for the collection. ReceiptURL is
// empty for a charge that never reached Stripe (pending, or a failed attempt).
type ChargeDetail struct {
	Charge
	StripePaymentIntent string `json:"stripe_payment_intent"`
	ReceiptURL          string `json:"receipt_url"`
}

// --- Endpoints ---

func (c *Client) Summary(ctx context.Context, org string) (*Summary, error) {
	var out Summary
	return &out, c.get(ctx, "/summary", params{org: org}, &out)
}

func (c *Client) ListInvoices(ctx context.Context, org, status string, limit int) ([]Invoice, error) {
	var out struct {
		Invoices []Invoice `json:"invoices"`
	}
	err := c.get(ctx, "/invoices", params{org: org, status: status, limit: limit}, &out)
	return out.Invoices, err
}

func (c *Client) GetInvoice(ctx context.Context, org, id string) (*InvoiceDetail, error) {
	var out InvoiceDetail
	return &out, c.get(ctx, "/invoices/"+url.PathEscape(id), params{org: org}, &out)
}

func (c *Client) ListCharges(ctx context.Context, org, status string, limit int) ([]Charge, error) {
	var out struct {
		Charges []Charge `json:"charges"`
	}
	err := c.get(ctx, "/charges", params{org: org, status: status, limit: limit}, &out)
	return out.Charges, err
}

func (c *Client) GetCharge(ctx context.Context, org, id string) (*ChargeDetail, error) {
	var out ChargeDetail
	return &out, c.get(ctx, "/charges/"+url.PathEscape(id), params{org: org}, &out)
}

// Download fetches a document from the URL the API handed back. Deliberately
// unauthenticated: these are Stripe's own signed URLs, and sending a kamuhub
// access key to a third-party host would leak the credential.
func (c *Client) Download(ctx context.Context, docURL string) ([]byte, error) {
	u, err := url.Parse(docURL)
	if err != nil {
		return nil, fmt.Errorf("parse document url: %w", err)
	}
	if u.Scheme != "https" {
		return nil, fmt.Errorf("refusing to download over %s", u.Scheme)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, docURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("download: HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// --- Plumbing ---

type params struct {
	org    string
	status string
	limit  int
}

func (p params) encode() string {
	q := url.Values{}
	if p.org != "" {
		q.Set("org", p.org)
	}
	if p.status != "" {
		q.Set("status", p.status)
	}
	if p.limit > 0 {
		q.Set("limit", strconv.Itoa(p.limit))
	}
	if len(q) == 0 {
		return ""
	}
	return "?" + q.Encode()
}

func (c *Client) get(ctx context.Context, path string, p params, out any) error {
	data, err := c.do(ctx, http.MethodGet, path+p.encode())
	if err != nil {
		return err
	}
	return json.Unmarshal(data, out)
}

func (c *Client) do(ctx context.Context, method, path string) (json.RawMessage, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+"/api/billing"+path, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.Key)
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("reach kamuhub (%s): %w", c.BaseURL, err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode >= 400 {
		// billing-api answers with Hono's {"message": ...}; the BFF's own
		// denials use {"error": ...}. Surface whichever came back.
		var apiErr struct {
			Message string `json:"message"`
			Error   string `json:"error"`
		}
		if json.Unmarshal(data, &apiErr) == nil {
			if msg := apiErr.Message + apiErr.Error; msg != "" {
				return nil, fmt.Errorf("billing: %s", msg)
			}
		}
		return nil, fmt.Errorf("billing: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	return json.RawMessage(data), nil
}
