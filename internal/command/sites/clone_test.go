package sites

import (
	"testing"

	"github.com/kotisivukamu/kamucli/internal/client/kamusites"
)

func TestMatchSites(t *testing.T) {
	all := []kamusites.Site{
		{ID: "id-1", Slug: "cafe-aalto", Name: "Cafe Aalto"},
		{ID: "id-2", Slug: "cafe-aalto-2", Name: "Cafe Aalto"},
		{ID: "id-3", Slug: "tepital", Name: "Tepital Oy"},
	}

	cases := []struct {
		name    string
		want    string
		wantIDs []string
	}{
		{"empty returns all", "", []string{"id-1", "id-2", "id-3"}},
		{"id match wins alone", "id-2", []string{"id-2"}},
		{"slug match wins alone", "tepital", []string{"id-3"}},
		{"exact name single", "Tepital Oy", []string{"id-3"}},
		{"name match is case-insensitive", "cafe aalto", []string{"id-1", "id-2"}},
		{"ambiguous name returns all matches", "Cafe Aalto", []string{"id-1", "id-2"}},
		{"no match", "nothing-here", nil},
		{"partial name does not match", "Cafe", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := matchSites(all, tc.want)
			if len(got) != len(tc.wantIDs) {
				t.Fatalf("matchSites(%q) = %d sites, want %d", tc.want, len(got), len(tc.wantIDs))
			}
			for i, s := range got {
				if s.ID != tc.wantIDs[i] {
					t.Errorf("matchSites(%q)[%d] = %s, want %s", tc.want, i, s.ID, tc.wantIDs[i])
				}
			}
		})
	}
}
