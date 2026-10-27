package publish

import (
	"encoding/base64"
	"encoding/json"
	"path/filepath"
	"testing"
)

// A key spanning several orgs must be told which one to publish for, rather than
// picking the first and surprising the user with a page in the wrong org.
func TestResolveOrg(t *testing.T) {
	if got, err := resolveOrg("explicit", fakeKey("a", "b")); err != nil || got != "explicit" {
		t.Errorf("--org should win: %q %v", got, err)
	}
	if got, err := resolveOrg("", fakeKey("solo")); err != nil || got != "solo" {
		t.Errorf("single-org key should resolve: %q %v", got, err)
	}
	if _, err := resolveOrg("", fakeKey("a", "b")); err == nil {
		t.Error("multi-org key without --org should error")
	}
	// An opaque/unparseable key sends no org and lets the server decide.
	if got, err := resolveOrg("", "not-a-jwt"); err != nil || got != "" {
		t.Errorf("opaque key: %q %v", got, err)
	}
}

// The state file is what makes a republish land on the same repo (and so the
// same URL), so round-tripping it is load-bearing behaviour, not bookkeeping.
func TestStateRoundTrip(t *testing.T) {
	t.Setenv("KAMU_CONFIG", filepath.Join(t.TempDir(), "config.yml"))

	s, err := loadState()
	if err != nil {
		t.Fatalf("loadState on a fresh home: %v", err)
	}
	if len(s.Files) != 0 {
		t.Fatalf("fresh state is not empty: %v", s.Files)
	}
	rec := Record{ID: "id1", Org: "acme", Repo: "acme/file-report-abc123", Branch: "main", Path: "dist/index.html"}
	s.Files["/home/u/report.html"] = rec
	if err := saveState(s); err != nil {
		t.Fatalf("saveState: %v", err)
	}

	again, err := loadState()
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got := again.Files["/home/u/report.html"]; got.Repo != rec.Repo {
		t.Errorf("republish would lose the repo: got %q, want %q", got.Repo, rec.Repo)
	}
	if _, ok := again.lookupByRepo("ACME/File-Report-abc123"); !ok {
		t.Error("lookupByRepo should be case-insensitive (forge names are)")
	}
	if _, ok := again.lookupByRepo("acme/other"); ok {
		t.Error("lookupByRepo matched an unrelated repo")
	}
}

func TestWorkdirIsUnderStateDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("KAMU_CONFIG", filepath.Join(dir, "config.yml"))
	got, err := workdir("acme/file-report")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, "publish", "acme", "file-report"); got != want {
		t.Errorf("workdir = %q, want %q", got, want)
	}
}

// fakeKey builds an unsigned JWT-shaped token carrying org slugs — the CLI only
// reads the payload to route, it never verifies (the server does).
func fakeKey(slugs ...string) string {
	type org struct {
		Slug string `json:"slug"`
	}
	payload := struct {
		Orgs []org `json:"orgs"`
	}{}
	for _, s := range slugs {
		payload.Orgs = append(payload.Orgs, org{Slug: s})
	}
	raw, _ := json.Marshal(payload)
	return "h." + base64.RawURLEncoding.EncodeToString(raw) + ".sig"
}
