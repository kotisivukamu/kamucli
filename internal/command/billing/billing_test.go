package billing

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

func TestResolveOrgMapsSlugToKamuidOrgID(t *testing.T) {
	key := fakeKey(t, `{"orgs":[{"kamuid_org_id":"org_1","slug":"acme"},{"kamuid_org_id":"org_2","slug":"globex"}]}`)
	org, err := resolveOrg("globex", key)
	if err != nil || org != "org_2" {
		t.Errorf("resolveOrg = %q, %v; want org_2 (globex's kamuid org id)", org, err)
	}
}

func TestResolveOrgPassesUnknownFlagThrough(t *testing.T) {
	// A raw kamuid org id, or a slug the key does not describe: send it as-is
	// and let the server decide. Unlike `kamu assets`, billing cannot resolve
	// slugs server-side, so this is the escape hatch for an opaque key.
	key := fakeKey(t, `{"orgs":[{"kamuid_org_id":"org_1","slug":"acme"}]}`)
	org, err := resolveOrg("org_9", key)
	if err != nil || org != "org_9" {
		t.Errorf("resolveOrg = %q, %v; want the flag value passed through", org, err)
	}
}

func TestResolveOrgSingleOrgKey(t *testing.T) {
	key := fakeKey(t, `{"orgs":[{"kamuid_org_id":"org_1","slug":"acme"}]}`)
	org, err := resolveOrg("", key)
	if err != nil || org != "org_1" {
		t.Errorf("resolveOrg = %q, %v; want org_1 from the key payload", org, err)
	}
}

func TestResolveOrgMultiOrgKeyNeedsFlag(t *testing.T) {
	key := fakeKey(t, `{"orgs":[{"kamuid_org_id":"org_1","slug":"acme"},{"kamuid_org_id":"org_2","slug":"globex"}]}`)
	if _, err := resolveOrg("", key); err == nil || !strings.Contains(err.Error(), "--org") {
		t.Errorf("resolveOrg err = %v, want a pass-the-flag error", err)
	}
}

func TestResolveOrgOpaqueKeyErrors(t *testing.T) {
	// Billing needs the kamuid org id in the query string, so an unreadable key
	// is a hard stop here (assets can send nothing and let the server resolve).
	for _, key := range []string{
		"not-a-jwt",
		fakeKey(t, `{"sub":"user_1"}`),
		fakeKey(t, `{"orgs":[]}`),
	} {
		if _, err := resolveOrg("", key); err == nil || !strings.Contains(err.Error(), "--org") {
			t.Errorf("resolveOrg(%q) err = %v, want a pass-the-flag error", key, err)
		}
	}
}

func TestResolveOrgSlugOnlyKeyErrors(t *testing.T) {
	key := fakeKey(t, `{"orgs":[{"slug":"acme"}]}`)
	if _, err := resolveOrg("", key); err == nil || !strings.Contains(err.Error(), "kamuid org id") {
		t.Errorf("resolveOrg err = %v, want an error naming the missing kamuid org id", err)
	}
}

func TestEuros(t *testing.T) {
	cases := []struct {
		cents    int64
		currency string
		want     string
	}{
		{1363, "eur", "13,63 EUR"},
		{100, "eur", "1,00 EUR"},
		{5, "eur", "0,05 EUR"},
		{0, "eur", "0,00 EUR"},
		{-1217, "eur", "-12,17 EUR"},
		{29111, "EUR", "291,11 EUR"},
	}
	for _, tc := range cases {
		if got := euros(tc.cents, tc.currency); got != tc.want {
			t.Errorf("euros(%d, %q) = %q, want %q", tc.cents, tc.currency, got, tc.want)
		}
	}
}

func TestDayAndPeriod(t *testing.T) {
	if got := day("2026-06-10T14:03:11.123Z"); got != "2026-06-10" {
		t.Errorf("day = %q, want 2026-06-10", got)
	}
	if got := day("2026-06-10"); got != "2026-06-10" {
		t.Errorf("day of a bare date = %q, want it unchanged", got)
	}
	if got := day(""); got != "" {
		t.Errorf("day of empty = %q, want empty", got)
	}
	if got := period("", ""); got != "-" {
		t.Errorf("period of an ad-hoc invoice = %q, want -", got)
	}
	if got := period("2026-06-01", "2026-06-30"); got != "2026-06-01 - 2026-06-30" {
		t.Errorf("period = %q", got)
	}
}
