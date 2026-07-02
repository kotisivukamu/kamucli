package clone

import (
	"testing"

	"github.com/kotisivukamu/kamucli/internal/client/forge"
)

func TestMatchRepos(t *testing.T) {
	all := []forge.Repo{
		{Owner: "org-a", Name: "cafe-aalto", Description: "Cafe Aalto website"},
		{Owner: "org-b", Name: "cafe-aalto", Description: "another org, same name"},
		{Owner: "org-a", Name: "tepital", Description: "Tepital Oy marketing site"},
		{Owner: "org-a", Name: "api24", Description: ""},
		{Owner: "org-b", Name: "aalto", Description: "aalto holding"},
	}

	cases := []struct {
		name      string
		want      string
		wantFulls []string
	}{
		{"empty returns all", "", []string{"org-a/cafe-aalto", "org-b/cafe-aalto", "org-a/tepital", "org-a/api24", "org-b/aalto"}},
		{"owner/name match wins alone", "org-b/cafe-aalto", []string{"org-b/cafe-aalto"}},
		{"exact name single", "tepital", []string{"org-a/tepital"}},
		{"exact name is case-insensitive", "TEPITAL", []string{"org-a/tepital"}},
		{"exact name across owners is ambiguous", "cafe-aalto", []string{"org-a/cafe-aalto", "org-b/cafe-aalto"}},
		{"name substring", "api", []string{"org-a/api24"}},
		{"description substring", "marketing", []string{"org-a/tepital"}},
		{"substring is case-insensitive", "Oy MARKETING", []string{"org-a/tepital"}},
		{"substring may match several", "cafe", []string{"org-a/cafe-aalto", "org-b/cafe-aalto"}},
		{"no match", "nothing-here", nil},
		{"exact name beats substring hits elsewhere", "aalto", []string{"org-b/aalto"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := matchRepos(all, tc.want)
			if len(got) != len(tc.wantFulls) {
				t.Fatalf("matchRepos(%q) = %d repos, want %d (%v)", tc.want, len(got), len(tc.wantFulls), got)
			}
			for i, r := range got {
				if r.FullName() != tc.wantFulls[i] {
					t.Errorf("matchRepos(%q)[%d] = %s, want %s", tc.want, i, r.FullName(), tc.wantFulls[i])
				}
			}
		})
	}
}
