package gitcredential

import (
	"strings"
	"testing"
)

func TestParseAttrs(t *testing.T) {
	t.Run("reads key=value lines", func(t *testing.T) {
		attrs, err := parseAttrs(strings.NewReader("protocol=https\nhost=kamusites-api.fly.dev\npath=git/o/r.git\n\n"))
		if err != nil {
			t.Fatal(err)
		}
		if attrs["protocol"] != "https" || attrs["host"] != "kamusites-api.fly.dev" || attrs["path"] != "git/o/r.git" {
			t.Fatalf("unexpected attrs: %v", attrs)
		}
	})

	t.Run("stops at the blank line", func(t *testing.T) {
		attrs, err := parseAttrs(strings.NewReader("host=a\n\nhost=b\n"))
		if err != nil {
			t.Fatal(err)
		}
		if attrs["host"] != "a" {
			t.Fatalf("attrs read past the blank terminator: %v", attrs)
		}
	})

	t.Run("value may contain =", func(t *testing.T) {
		attrs, err := parseAttrs(strings.NewReader("password=a=b=c\n"))
		if err != nil {
			t.Fatal(err)
		}
		if attrs["password"] != "a=b=c" {
			t.Fatalf("got %q", attrs["password"])
		}
	})

	t.Run("EOF without blank line is fine", func(t *testing.T) {
		attrs, err := parseAttrs(strings.NewReader("host=a"))
		if err != nil {
			t.Fatal(err)
		}
		if attrs["host"] != "a" {
			t.Fatalf("unexpected attrs: %v", attrs)
		}
	})

	t.Run("empty input is fine", func(t *testing.T) {
		attrs, err := parseAttrs(strings.NewReader(""))
		if err != nil {
			t.Fatal(err)
		}
		if len(attrs) != 0 {
			t.Fatalf("unexpected attrs: %v", attrs)
		}
	})

	t.Run("malformed line errors", func(t *testing.T) {
		if _, err := parseAttrs(strings.NewReader("notakeyvalue\n")); err == nil {
			t.Fatal("want error for malformed attribute line")
		}
	})
}

func TestHostMatches(t *testing.T) {
	const cloneURL = "https://kamusites-api.fly.dev/git/owner/repo.git"

	cases := []struct {
		name  string
		attrs map[string]string
		url   string
		want  bool
	}{
		{"same protocol and host", map[string]string{"protocol": "https", "host": "kamusites-api.fly.dev"}, cloneURL, true},
		{"different host", map[string]string{"protocol": "https", "host": "github.com"}, cloneURL, false},
		{"different protocol", map[string]string{"protocol": "http", "host": "kamusites-api.fly.dev"}, cloneURL, false},
		{"port must match", map[string]string{"protocol": "https", "host": "kamusites-api.fly.dev:8443"}, cloneURL, false},
		{"port matches when in clone url", map[string]string{"protocol": "https", "host": "localhost:8787"}, "https://localhost:8787/git/o/r.git", true},
		{"no attrs matches (site-id gate already passed)", map[string]string{}, cloneURL, true},
		{"unparseable clone url never matches", map[string]string{"host": "kamusites-api.fly.dev"}, "://bad", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := hostMatches(tc.attrs, tc.url); got != tc.want {
				t.Fatalf("hostMatches(%v, %q) = %v, want %v", tc.attrs, tc.url, got, tc.want)
			}
		})
	}
}
