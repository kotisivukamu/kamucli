package kamusites

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDestinationsRouteAndParse(t *testing.T) {
	var gotPath, gotMethod, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		gotAuth = r.Header.Get("Authorization")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"destinations": []map[string]any{
				{"id": "d1", "target_url": "https://a", "path": "/k/amu/relay/d1", "wired": true, "has_secret": true},
				{"id": "d2", "target_url": "https://b", "path": "/k/amu/relay/d2", "wired": false},
			},
		})
	}))
	defer srv.Close()

	dests, err := New(srv.URL, "test-key").Destinations(context.Background(), "site-1")
	if err != nil {
		t.Fatalf("Destinations: %v", err)
	}
	if gotMethod != http.MethodGet || gotPath != "/api/sites/site-1/destinations" {
		t.Errorf("hit %s %s, want GET /api/sites/site-1/destinations", gotMethod, gotPath)
	}
	if gotAuth != "Bearer test-key" {
		t.Errorf("Authorization = %q", gotAuth)
	}
	if len(dests) != 2 || !dests[0].Wired || dests[0].ID != "d1" || !dests[0].HasSecret {
		t.Errorf("dests = %+v", dests)
	}
	if dests[1].Wired {
		t.Errorf("d2 should be unwired: %+v", dests[1])
	}
}

func TestCreateDestinationBodyAndRoute(t *testing.T) {
	var gotPath, gotMethod string
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"destination": map[string]any{"id": "d9", "target_url": "https://hooks", "path": "/k/amu/relay/d9", "has_secret": true},
		})
	}))
	defer srv.Close()

	dest, err := New(srv.URL, "k").CreateDestination(context.Background(), "site-1", CreateDestinationInput{
		TargetURL: "https://hooks",
		Label:     "Contact",
		Secret:    "shh",
	})
	if err != nil {
		t.Fatalf("CreateDestination: %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/api/sites/site-1/destinations" {
		t.Errorf("hit %s %s, want POST /api/sites/site-1/destinations", gotMethod, gotPath)
	}
	if body["target_url"] != "https://hooks" || body["label"] != "Contact" || body["secret"] != "shh" {
		t.Errorf("body = %+v", body)
	}
	// Zero-valued numerics must be omitted so the relay applies its defaults.
	if _, ok := body["max_body_bytes"]; ok {
		t.Errorf("max_body_bytes should be omitted when 0: %+v", body)
	}
	if _, ok := body["rate_per_hour"]; ok {
		t.Errorf("rate_per_hour should be omitted when 0: %+v", body)
	}
	if dest.ID != "d9" || dest.Path != "/k/amu/relay/d9" || !dest.HasSecret {
		t.Errorf("dest = %+v", dest)
	}
}

func TestDeleteDestinationRoute(t *testing.T) {
	var gotPath, gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	if err := New(srv.URL, "k").DeleteDestination(context.Background(), "site-1", "d9"); err != nil {
		t.Fatalf("DeleteDestination: %v", err)
	}
	if gotMethod != http.MethodDelete || gotPath != "/api/sites/site-1/destinations/d9" {
		t.Errorf("hit %s %s, want DELETE /api/sites/site-1/destinations/d9", gotMethod, gotPath)
	}
}

func TestCreateDestinationRelay400Slug(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "invalid target_url: loopback address"})
	}))
	defer srv.Close()

	_, err := New(srv.URL, "k").CreateDestination(context.Background(), "s", CreateDestinationInput{TargetURL: "http://127.0.0.1"})
	var ae *APIError
	if !errors.As(err, &ae) {
		t.Fatalf("err = %v, want *APIError", err)
	}
	if ae.StatusCode != http.StatusBadRequest || ae.Message != "invalid target_url: loopback address" {
		t.Errorf("APIError = %+v", ae)
	}
}
