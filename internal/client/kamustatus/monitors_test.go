package kamustatus

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// The monitors route is paginated; the client must unwrap the envelope and
// walk every page.
func TestListMonitorsWalksPages(t *testing.T) {
	const total = 250
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/status/projects/p1/monitors" {
			t.Errorf("path = %s", r.URL.Path)
		}
		var limit, offset int
		fmt.Sscan(r.URL.Query().Get("limit"), &limit)
		fmt.Sscan(r.URL.Query().Get("offset"), &offset)
		var page []Monitor
		for i := offset; i < total && i < offset+limit; i++ {
			page = append(page, Monitor{ID: fmt.Sprintf("m_%d", i)})
		}
		json.NewEncoder(w).Encode(map[string]any{"monitors": page, "total": total, "limit": limit, "offset": offset})
	}))
	defer srv.Close()

	c := New(srv.URL, "key")
	got, err := c.ListMonitors(context.Background(), "p1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != total {
		t.Fatalf("got %d monitors, want %d", len(got), total)
	}
	if got[0].ID != "m_0" || got[total-1].ID != fmt.Sprintf("m_%d", total-1) {
		t.Errorf("order wrong: first %s last %s", got[0].ID, got[total-1].ID)
	}
}
