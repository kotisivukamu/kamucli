package kamuid

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOrgAPIBaseFromIssuer(t *testing.T) {
	c := NewOrgAPI("https://accounts.kamuhub.com/", "tok")
	if c.BaseURL != "https://accounts.kamuhub.com/api" {
		t.Errorf("BaseURL = %q, want issuer + /api", c.BaseURL)
	}
	if d := NewOrgAPI("", "tok"); d.BaseURL != DefaultIssuer+"/api" {
		t.Errorf("default BaseURL = %q, want %q", d.BaseURL, DefaultIssuer+"/api")
	}
}

func TestOrgAPIRoutesAndAuth(t *testing.T) {
	var gotPath, gotMethod, gotAuth string
	var gotBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		gotAuth = r.Header.Get("Authorization")
		gotBody = nil
		if r.Body != nil {
			_ = json.NewDecoder(r.Body).Decode(&gotBody)
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/rp/organizations":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"organizations": []map[string]string{
					{"id": "org_1", "slug": "acme", "name": "Acme", "role": "owner"},
				},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/rp/organizations":
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"organization": map[string]string{"id": "org_2", "slug": "new-org", "name": "New Org", "role": "owner"},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/rp/organizations/org_1":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"organization": map[string]string{"id": "org_1", "slug": "acme", "name": "Acme"},
				"members": []map[string]string{
					{"userId": "user_1", "email": "a@example.com", "role": "owner"},
				},
				"invitations": []map[string]string{
					{"id": "inv_1", "email": "b@example.com", "role": "member"},
				},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/rp/organizations/org_1/invitations":
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"invitation": map[string]string{"id": "inv_2", "email": "c@example.com", "role": "admin"},
			})
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v1/rp/organizations/org_1/members/user_1":
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v1/rp/organizations/org_1":
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "not_found"})
		}
	}))
	defer srv.Close()

	ctx := context.Background()
	c := NewOrgAPI(srv.URL, "test-token")

	orgs, err := c.ListOrgs(ctx)
	if err != nil {
		t.Fatalf("ListOrgs: %v", err)
	}
	if len(orgs) != 1 || orgs[0].Slug != "acme" || orgs[0].Role != "owner" {
		t.Errorf("ListOrgs = %+v", orgs)
	}
	if gotAuth != "Bearer test-token" {
		t.Errorf("Authorization = %q, want bearer token", gotAuth)
	}

	org, err := c.CreateOrg(ctx, CreateOrgInput{Name: "New Org", Slug: "new-org"})
	if err != nil {
		t.Fatalf("CreateOrg: %v", err)
	}
	if org.ID != "org_2" || gotBody["name"] != "New Org" || gotBody["slug"] != "new-org" {
		t.Errorf("CreateOrg = %+v, body sent = %v", org, gotBody)
	}

	detail, err := c.GetOrg(ctx, "org_1")
	if err != nil {
		t.Fatalf("GetOrg: %v", err)
	}
	if detail.Organization.Slug != "acme" || len(detail.Members) != 1 || len(detail.Invitations) != 1 {
		t.Errorf("GetOrg = %+v", detail)
	}
	if detail.Members[0].UserID != "user_1" {
		t.Errorf("member userId = %q, want user_1", detail.Members[0].UserID)
	}

	inv, err := c.Invite(ctx, "org_1", InviteInput{Email: "c@example.com", Role: "admin"})
	if err != nil {
		t.Fatalf("Invite: %v", err)
	}
	if inv.Role != "admin" || gotBody["email"] != "c@example.com" || gotBody["role"] != "admin" {
		t.Errorf("Invite = %+v, body sent = %v", inv, gotBody)
	}

	if err := c.RemoveMember(ctx, "org_1", "user_1"); err != nil {
		t.Fatalf("RemoveMember: %v", err)
	}
	if gotMethod != http.MethodDelete || gotPath != "/api/v1/rp/organizations/org_1/members/user_1" {
		t.Errorf("RemoveMember hit %s %s", gotMethod, gotPath)
	}

	if err := c.DeleteOrg(ctx, "org_1"); err != nil {
		t.Fatalf("DeleteOrg: %v", err)
	}
	if gotMethod != http.MethodDelete || gotPath != "/api/v1/rp/organizations/org_1" {
		t.Errorf("DeleteOrg hit %s %s", gotMethod, gotPath)
	}
}

func TestOrgAPIErrorEnvelope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error":             "insufficient_scope",
			"error_description": "Required scope: kamu.org.manage",
		})
	}))
	defer srv.Close()

	_, err := NewOrgAPI(srv.URL, "tok").ListOrgs(context.Background())
	var ae *APIError
	if !errors.As(err, &ae) {
		t.Fatalf("err = %v, want *APIError", err)
	}
	if ae.Status != http.StatusForbidden || ae.Code != "insufficient_scope" {
		t.Errorf("APIError = %+v", ae)
	}
}
