package assets

import (
	"encoding/base64"
	"strings"
	"testing"
)

// fakeKey builds a JWT-shaped access key whose payload carries the given orgs
// (only the shape matters — nothing verifies the signature client-side).
func fakeKey(t *testing.T, payload string) string {
	t.Helper()
	return "eyJhbGciOiJub25lIn0." + base64.RawURLEncoding.EncodeToString([]byte(payload)) + ".sig"
}

func TestResolveOrgFlagWins(t *testing.T) {
	key := fakeKey(t, `{"orgs":[{"slug":"acme"}]}`)
	org, err := resolveOrg("other", key)
	if err != nil || org != "other" {
		t.Errorf("resolveOrg = %q, %v; want the flag value", org, err)
	}
}

func TestResolveOrgSingleOrgKey(t *testing.T) {
	key := fakeKey(t, `{"orgs":[{"kamuid_org_id":"org_1","slug":"acme"}]}`)
	org, err := resolveOrg("", key)
	if err != nil || org != "acme" {
		t.Errorf("resolveOrg = %q, %v; want acme from the key payload", org, err)
	}
}

func TestResolveOrgMultiOrgKeyNeedsFlag(t *testing.T) {
	key := fakeKey(t, `{"orgs":[{"slug":"acme"},{"slug":"globex"}]}`)
	if _, err := resolveOrg("", key); err == nil || !strings.Contains(err.Error(), "--org") {
		t.Errorf("resolveOrg err = %v, want a pass-the-flag error", err)
	}
}

func TestResolveOrgOpaqueKeySendsNothing(t *testing.T) {
	// Not JWT-shaped, or a payload without slugs: send no org and let the server
	// resolve a single-org context itself.
	for _, key := range []string{
		"not-a-jwt",
		fakeKey(t, `{"sub":"user_1"}`),
		fakeKey(t, `{"orgs":[]}`),
	} {
		org, err := resolveOrg("", key)
		if err != nil || org != "" {
			t.Errorf("resolveOrg(%q) = %q, %v; want empty org, no error", key, org, err)
		}
	}
}

func TestValidateUploadExtension(t *testing.T) {
	for _, path := range []string{"a.jpg", "b.JPEG", "c.png", "d.webp", "e.gif"} {
		if err := validateUpload(path, 10); err != nil {
			t.Errorf("validateUpload(%q) = %v, want ok", path, err)
		}
	}
	for _, path := range []string{"doc.pdf", "photo.tiff", "noext", "archive.zip"} {
		if err := validateUpload(path, 10); err == nil {
			t.Errorf("validateUpload(%q) = nil, want an unsupported-type error", path)
		}
	}
}

func TestValidateUploadSize(t *testing.T) {
	if err := validateUpload("big.png", maxUploadBytes+1); err == nil || !strings.Contains(err.Error(), "5 MB") {
		t.Errorf("validateUpload oversize err = %v, want a 5 MB limit error", err)
	}
	if err := validateUpload("ok.png", maxUploadBytes); err != nil {
		t.Errorf("validateUpload at the cap = %v, want ok", err)
	}
}

func TestHumanBytes(t *testing.T) {
	cases := map[int64]string{
		512:             "512 B",
		2048:            "2.0 kB",
		5 * 1024 * 1024: "5.0 MB",
		3 << 30:         "3.0 GB",
	}
	for n, want := range cases {
		if got := humanBytes(n); got != want {
			t.Errorf("humanBytes(%d) = %q, want %q", n, got, want)
		}
	}
}
