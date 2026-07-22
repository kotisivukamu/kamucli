package assets

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewBaseURL(t *testing.T) {
	if c := New("", "k"); c.BaseURL != DefaultBaseURL {
		t.Errorf("BaseURL = %q, want %q", c.BaseURL, DefaultBaseURL)
	}
	if c := New("http://localhost:9999/", "k"); c.BaseURL != "http://localhost:9999" {
		t.Errorf("BaseURL = %q, want trailing slash trimmed", c.BaseURL)
	}
}

func TestUploadRoutesAndAuth(t *testing.T) {
	var gotPath, gotMethod, gotAuth, gotType, gotOrg string
	var gotBody []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		gotAuth = r.Header.Get("Authorization")
		gotType = r.Header.Get("Content-Type")
		gotOrg = r.URL.Query().Get("org")
		gotBody, _ = io.ReadAll(r.Body)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":       "ok",
			"name":         "abc123.png",
			"url":          "https://files.kskamu.app/a/abc123.png",
			"bytes":        4,
			"content_type": "image/png",
			"existing":     true,
		})
	}))
	defer srv.Close()

	c := New(srv.URL, "test-key")
	res, err := c.Upload(context.Background(), "acme", []byte("PNG!"))
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/api/assets" {
		t.Errorf("Upload hit %s %s, want POST /api/assets", gotMethod, gotPath)
	}
	if gotOrg != "acme" {
		t.Errorf("org query = %q, want acme", gotOrg)
	}
	if gotAuth != "Bearer test-key" {
		t.Errorf("Authorization = %q, want bearer key", gotAuth)
	}
	if gotType != "application/octet-stream" {
		t.Errorf("Content-Type = %q, want application/octet-stream", gotType)
	}
	if string(gotBody) != "PNG!" {
		t.Errorf("body = %q, want the raw bytes", gotBody)
	}
	if res.Name != "abc123.png" || res.URL != "https://files.kskamu.app/a/abc123.png" ||
		res.Bytes != 4 || res.ContentType != "image/png" || !res.Existing {
		t.Errorf("UploadResult = %+v", res)
	}
}

func TestUploadOmitsEmptyOrg(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok", "name": "x.png"})
	}))
	defer srv.Close()

	if _, err := New(srv.URL, "k").Upload(context.Background(), "", []byte("x")); err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if gotQuery != "" {
		t.Errorf("query = %q, want no org parameter", gotQuery)
	}
}

func TestUsage(t *testing.T) {
	var gotPath, gotMethod, gotOrg string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		gotOrg = r.URL.Query().Get("org")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"org":         "acme",
			"usage":       map[string]any{"bytes": 1234, "count": 3, "updated_at": "2026-07-01T00:00:00Z"},
			"limit_bytes": 524288000,
		})
	}))
	defer srv.Close()

	u, err := New(srv.URL, "k").Usage(context.Background(), "acme")
	if err != nil {
		t.Fatalf("Usage: %v", err)
	}
	if gotMethod != http.MethodGet || gotPath != "/api/assets/usage" {
		t.Errorf("Usage hit %s %s, want GET /api/assets/usage", gotMethod, gotPath)
	}
	if gotOrg != "acme" {
		t.Errorf("org query = %q, want acme", gotOrg)
	}
	if u.Org != "acme" || u.Usage.Bytes != 1234 || u.Usage.Count != 3 || u.LimitBytes != 524288000 {
		t.Errorf("UsageResult = %+v", u)
	}
}

func TestAPIErrorMapping(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusRequestEntityTooLarge)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": "org storage quota exceeded", "limit_bytes": 100, "used_bytes": 99,
		})
	}))
	defer srv.Close()

	_, err := New(srv.URL, "k").Upload(context.Background(), "acme", []byte("x"))
	var ae *APIError
	if !errors.As(err, &ae) {
		t.Fatalf("err = %v, want *APIError", err)
	}
	if ae.StatusCode != http.StatusRequestEntityTooLarge || ae.Message != "org storage quota exceeded" {
		t.Errorf("APIError = %+v", ae)
	}
}
