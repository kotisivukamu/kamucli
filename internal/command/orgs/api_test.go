package orgs

import (
	"testing"

	"github.com/kotisivukamu/kamucli/internal/client/kamuid"
)

func TestMatchOrg(t *testing.T) {
	orgs := []kamuid.Organization{
		{ID: "org_1", Slug: "acme", Name: "Acme Oy"},
		{ID: "org_2", Slug: "beta-corp", Name: "Beta Corp"},
		{ID: "org_3", Slug: "org_1", Name: "Slug that looks like an id"},
	}

	cases := []struct {
		name string
		arg  string
		want string // matched org ID; "" = no match
	}{
		{"by slug", "acme", "org_1"},
		{"by slug case-insensitive", "ACME", "org_1"},
		{"by slug with whitespace", "  beta-corp ", "org_2"},
		{"by id", "org_2", "org_2"},
		{"id beats identical slug", "org_1", "org_1"},
		{"no match", "nope", ""},
		{"empty", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := matchOrg(orgs, tc.arg)
			switch {
			case tc.want == "" && got != nil:
				t.Errorf("matchOrg(%q) = %+v, want nil", tc.arg, got)
			case tc.want != "" && got == nil:
				t.Errorf("matchOrg(%q) = nil, want id %s", tc.arg, tc.want)
			case tc.want != "" && got.ID != tc.want:
				t.Errorf("matchOrg(%q).ID = %s, want %s", tc.arg, got.ID, tc.want)
			}
		})
	}
}

func TestSlugsSorted(t *testing.T) {
	orgs := []kamuid.Organization{{Slug: "zeta"}, {Slug: "acme"}, {Slug: "mid"}}
	got := slugs(orgs)
	want := []string{"acme", "mid", "zeta"}
	if len(got) != len(want) {
		t.Fatalf("slugs = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("slugs = %v, want %v", got, want)
		}
	}
}

func TestMatchMember(t *testing.T) {
	members := []kamuid.Member{
		{UserID: "user_1", Email: "owner@example.com", Role: "owner"},
		{UserID: "user_2", Email: "Member@Example.com", Role: "member"},
	}

	cases := []struct {
		name string
		arg  string
		want string
	}{
		{"by user id", "user_2", "user_2"},
		{"by email", "owner@example.com", "user_1"},
		{"by email case-insensitive", "member@example.com", "user_2"},
		{"by email with whitespace", " owner@example.com ", "user_1"},
		{"no match", "ghost@example.com", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := matchMember(members, tc.arg)
			switch {
			case tc.want == "" && got != nil:
				t.Errorf("matchMember(%q) = %+v, want nil", tc.arg, got)
			case tc.want != "" && (got == nil || got.UserID != tc.want):
				t.Errorf("matchMember(%q) = %+v, want UserID %s", tc.arg, got, tc.want)
			}
		})
	}
}
