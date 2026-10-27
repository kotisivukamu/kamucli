package kamustatus

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

type recorded struct {
	method, path, auth string
	body               map[string]any
}

// recorder answers every request with resp and records what it was sent.
func recorder(t *testing.T, resp string) (*Client, *recorded) {
	t.Helper()
	rec := &recorded{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.method, rec.path, rec.auth = r.Method, r.URL.Path, r.Header.Get("Authorization")
		rec.body = nil
		if b, _ := io.ReadAll(r.Body); len(b) > 0 {
			if err := json.Unmarshal(b, &rec.body); err != nil {
				t.Errorf("request body is not JSON: %s", b)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, resp)
	}))
	t.Cleanup(srv.Close)
	return New(srv.URL, "test-key"), rec
}

func (r *recorded) expect(t *testing.T, method, path string) {
	t.Helper()
	if r.method != method || r.path != path {
		t.Errorf("hit %s %s, want %s %s", r.method, r.path, method, path)
	}
	if r.auth != "Bearer test-key" {
		t.Errorf("Authorization = %q, want the access key as bearer", r.auth)
	}
}

const pageJSON = `{"id":"sp_1","project_id":"p_1","name":"Kontakto","slug":"kontakto","public":true,
	"title":null,"logo_url":null,"custom_css":null,"custom_domain":"status.kontakto.fi",
	"custom_domain_verified":false,"cname":"pages.kamustatus.com",
	"monitors":[{"id":"m_1","name":"API","type":"http","target":"https://api.example.com/health","display_order":0}]}`

func TestListStatusPages(t *testing.T) {
	c, rec := recorder(t, `[`+pageJSON+`]`)
	pages, err := c.ListStatusPages(context.Background(), "p_1")
	if err != nil {
		t.Fatal(err)
	}
	rec.expect(t, "GET", "/api/status/projects/p_1/status-pages")
	if len(pages) != 1 || pages[0].Slug != "kontakto" {
		t.Errorf("pages = %+v", pages)
	}
}

func TestCreateStatusPage(t *testing.T) {
	c, rec := recorder(t, pageJSON)
	p, err := c.CreateStatusPage(context.Background(), "p_1", map[string]any{
		"name": "Kontakto", "slug": "kontakto", "monitorIds": []string{"m_1", "m_2"},
		"customDomain": "status.kontakto.fi",
	})
	if err != nil {
		t.Fatal(err)
	}
	rec.expect(t, "POST", "/api/status/projects/p_1/status-pages")
	want := map[string]any{
		"name": "Kontakto", "slug": "kontakto", "monitorIds": []any{"m_1", "m_2"},
		"customDomain": "status.kontakto.fi",
	}
	if !reflect.DeepEqual(rec.body, want) {
		t.Errorf("body = %v, want %v", rec.body, want)
	}
	if p.CustomDomain == nil || *p.CustomDomain != "status.kontakto.fi" {
		t.Errorf("custom domain = %v", p.CustomDomain)
	}
	if p.CustomDomainVerified == nil || *p.CustomDomainVerified {
		t.Errorf("verified = %v, want false", p.CustomDomainVerified)
	}
	if got := p.PlatformURL(); got != "https://kontakto.kamustatus.com" {
		t.Errorf("PlatformURL = %q", got)
	}
}

func TestGetUpdateDeleteStatusPage(t *testing.T) {
	c, rec := recorder(t, pageJSON)
	ctx := context.Background()

	p, err := c.GetStatusPage(ctx, "sp_1")
	if err != nil {
		t.Fatal(err)
	}
	rec.expect(t, "GET", "/api/status/status-pages/sp_1")
	if len(p.Monitors) != 1 || p.Monitors[0].ID != "m_1" {
		t.Errorf("monitors = %+v", p.Monitors)
	}

	// "" for customDomain is the clear signal and must reach the wire as "".
	if _, err := c.UpdateStatusPage(ctx, "sp_1", map[string]any{"customDomain": "", "public": false}); err != nil {
		t.Fatal(err)
	}
	rec.expect(t, "PATCH", "/api/status/status-pages/sp_1")
	if want := map[string]any{"customDomain": "", "public": false}; !reflect.DeepEqual(rec.body, want) {
		t.Errorf("body = %v, want %v", rec.body, want)
	}

	if err := c.DeleteStatusPage(ctx, "sp_1"); err != nil {
		t.Fatal(err)
	}
	rec.expect(t, "DELETE", "/api/status/status-pages/sp_1")
}

func TestDomains(t *testing.T) {
	const dom = `{"id":"d_1","project_id":"p_1","domain":"status.kontakto.fi","verified":true,
		"status_page_id":"sp_1","status_page_name":"Kontakto","status_page_slug":"kontakto",
		"cname":"pages.kamustatus.com"}`
	c, rec := recorder(t, `[`+dom+`]`)
	ds, err := c.ListDomains(context.Background(), "p_1")
	if err != nil {
		t.Fatal(err)
	}
	rec.expect(t, "GET", "/api/status/projects/p_1/domains")
	if len(ds) != 1 || !ds[0].Verified || ds[0].CNAME != PagesCNAMETarget {
		t.Errorf("domains = %+v", ds)
	}

	c, rec = recorder(t, dom)
	d, err := c.VerifyDomain(context.Background(), "d_1")
	if err != nil {
		t.Fatal(err)
	}
	rec.expect(t, "POST", "/api/status/domains/d_1/verify")
	if rec.body != nil {
		t.Errorf("verify sent a body: %v", rec.body)
	}
	if d.Domain != "status.kontakto.fi" || !d.Verified {
		t.Errorf("domain = %+v", d)
	}
}

func TestDomainConflictSurfacesServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = io.WriteString(w, `{"error":"Domain already in use"}`)
	}))
	defer srv.Close()
	_, err := New(srv.URL, "k").UpdateStatusPage(context.Background(), "sp_1", map[string]any{"customDomain": "status.taken.fi"})
	if err == nil || err.Error() != "kamustatus: Domain already in use" {
		t.Errorf("err = %v, want the server's 409 message", err)
	}
}
