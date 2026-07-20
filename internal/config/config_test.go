package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSaveLoadRoundTripRPAPIFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	t.Setenv("KAMU_CONFIG", path)

	exp := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	in := &Config{
		ClientID:            "kamu-cli",
		RefreshToken:        "refresh-1",
		RPAPIToken:          "rp-token-1",
		RPAPITokenExpiresAt: exp,
		ActiveOrg:           "acme",
	}
	if err := Save(in); err != nil {
		t.Fatalf("Save: %v", err)
	}

	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if got := fi.Mode().Perm(); got != 0o600 {
		t.Errorf("config perms = %o, want 600", got)
	}

	out, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if out.RPAPIToken != in.RPAPIToken {
		t.Errorf("RPAPIToken = %q, want %q", out.RPAPIToken, in.RPAPIToken)
	}
	if !out.RPAPITokenExpiresAt.Equal(exp) {
		t.Errorf("RPAPITokenExpiresAt = %v, want %v", out.RPAPITokenExpiresAt, exp)
	}
	if out.RefreshToken != in.RefreshToken || out.ActiveOrg != in.ActiveOrg {
		t.Errorf("round-trip mismatch: %+v", out)
	}
}

func TestRPAPIFieldsOmittedWhenEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	t.Setenv("KAMU_CONFIG", path)

	if err := Save(&Config{RefreshToken: "r"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	for _, key := range []string{"rp_api_token", "rp_api_token_expires_at"} {
		if strings.Contains(string(data), key) {
			t.Errorf("empty field %q serialized:\n%s", key, data)
		}
	}
}

func TestResolveRPAPIAudience(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
		env  string
		want string
	}{
		{"default issuer", Config{}, "", "https://accounts.kamuhub.com/api/v1/rp"},
		{"config issuer trailing slash", Config{Endpoints: Endpoints{Kamuid: "https://id.example.test/"}}, "", "https://id.example.test/api/v1/rp"},
		{"env wins", Config{Endpoints: Endpoints{Kamuid: "https://id.example.test"}}, "http://localhost:8000", "http://localhost:8000/api/v1/rp"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(EnvIssuer, tc.env)
			if got := tc.cfg.ResolveRPAPIAudience(); got != tc.want {
				t.Errorf("ResolveRPAPIAudience() = %q, want %q", got, tc.want)
			}
		})
	}
}
