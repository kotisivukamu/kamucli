package status

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/kotisivukamu/kamucli/internal/iostreams"
)

type call struct {
	method, path string
	body         map[string]any
}

// run executes `kamu status <args>` against a fake kamustatus that answers
// every request with resp, returning the requests it saw and stdout.
func run(t *testing.T, resp string, args ...string) ([]call, string) {
	t.Helper()
	var calls []call
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c := call{method: r.Method, path: r.URL.Path}
		if b, _ := io.ReadAll(r.Body); len(b) > 0 {
			if err := json.Unmarshal(b, &c.body); err != nil {
				t.Errorf("request body is not JSON: %s", b)
			}
		}
		calls = append(calls, c)
		_, _ = io.WriteString(w, resp)
	}))
	t.Cleanup(srv.Close)
	t.Setenv(EnvURL, srv.URL)
	t.Setenv(EnvAccessKey, "test-key")
	keyFlag = ""

	var out bytes.Buffer
	ctx := iostreams.NewContext(context.Background(), &iostreams.IOStreams{Out: &out, ErrOut: io.Discard})
	cmd := New()
	cmd.SetArgs(args)
	if err := cmd.ExecuteContext(ctx); err != nil {
		t.Fatalf("kamu status %s: %v", strings.Join(args, " "), err)
	}
	return calls, out.String()
}

const page = `{"id":"sp_1","project_id":"p_1","name":"Kontakto","slug":"kontakto","public":true,
	"custom_domain":"status.kontakto.fi","custom_domain_verified":false,
	"monitors":[{"id":"m_1","name":"API","type":"http","target":"https://api.example.com/health"}]}`

func TestPagesUpdateSendsOnlyChangedFields(t *testing.T) {
	css := filepath.Join(t.TempDir(), "page.css")
	if err := os.WriteFile(css, []byte("body{color:red}"), 0o600); err != nil {
		t.Fatal(err)
	}
	calls, out := run(t, page, "pages", "update", "sp_1",
		"--custom-domain", "", "--public=false", "--monitor", "m_2", "--monitor", "m_1",
		"--custom-css-file", css)
	if len(calls) != 2 || calls[0].method != "PATCH" || calls[0].path != "/api/status/status-pages/sp_1" {
		t.Fatalf("calls = %+v, want PATCH then GET of the page", calls)
	}
	want := map[string]any{
		"customDomain": "", "public": false, "monitorIds": []any{"m_2", "m_1"},
		"customCss": "body{color:red}",
	}
	if !reflect.DeepEqual(calls[0].body, want) {
		t.Errorf("PATCH body = %v, want %v", calls[0].body, want)
	}
	if calls[1].method != "GET" || calls[1].path != "/api/status/status-pages/sp_1" {
		t.Errorf("re-read = %+v", calls[1])
	}
	for _, s := range []string{"https://kontakto.kamustatus.com", "status.kontakto.fi (Pending DNS)", "pages.kamustatus.com", "m_1"} {
		if !strings.Contains(out, s) {
			t.Errorf("output missing %q:\n%s", s, out)
		}
	}
}

func TestPagesCreateBody(t *testing.T) {
	calls, _ := run(t, page, "pages", "create", "p_1", "--name", "Kontakto", "--slug", "kontakto",
		"--monitor", "m_1", "--custom-domain", "status.kontakto.fi")
	want := map[string]any{
		"name": "Kontakto", "slug": "kontakto", "monitorIds": []any{"m_1"},
		"customDomain": "status.kontakto.fi",
	}
	if calls[0].method != "POST" || calls[0].path != "/api/status/projects/p_1/status-pages" ||
		!reflect.DeepEqual(calls[0].body, want) {
		t.Errorf("create = %+v, want POST with %v", calls[0], want)
	}
}

func TestMonitorsAssertionFlags(t *testing.T) {
	mon := `{"id":"m_1","name":"API","type":"http","target":"https://api.example.com/health"}`
	calls, _ := run(t, mon, "monitors", "add", "p_1", "--name", "API",
		"--target", "https://api.example.com/health",
		"--json-path", "$.status", "--json-expected", "ok", "--body-contains", "!error")
	want := map[string]any{
		"name": "API", "type": "http", "target": "https://api.example.com/health",
		"jsonPath": "$.status", "jsonExpected": "ok", "bodyContains": "!error",
	}
	if calls[0].path != "/api/status/projects/p_1/monitors" || !reflect.DeepEqual(calls[0].body, want) {
		t.Errorf("add = %+v, want body %v", calls[0], want)
	}

	// `update` is the alias of `set`; "" clears an assertion.
	calls, _ = run(t, mon, "monitors", "update", "m_1", "--body-regex", `^\{`, "--json-expected", "!=degraded", "--body-contains", "")
	want = map[string]any{"bodyRegex": `^\{`, "jsonExpected": "!=degraded", "bodyContains": ""}
	if calls[0].method != "PATCH" || calls[0].path != "/api/status/monitors/m_1" || !reflect.DeepEqual(calls[0].body, want) {
		t.Errorf("update = %+v, want PATCH body %v", calls[0], want)
	}
}
