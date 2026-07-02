// kamu bee logs — pull recent log lines for a kamubee app, optionally
// follow live via Server-Sent Events. Talks to kamubee's
// /v1/apps/{name}/logs endpoint; auth comes from `kamu auth login`
// (kamuid access token).

package bee

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/kotisivukamu/kamucli/internal/command"
	"github.com/kotisivukamu/kamucli/internal/config"
)

const defaultKamubeeBase = "https://api.staging.kamubee.dev"

func newLogsCmd() *cobra.Command {
	var (
		app    string
		follow bool
		tail   int
	)
	c := command.New("logs", "Tail app logs (use -a to pick the app)", "", func(ctx context.Context, _ []string) error {
		if app == "" {
			return fmt.Errorf("--app/-a is required (the app's name on kamubee)")
		}
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		return runLogs(ctx, cfg, app, tail, follow)
	})
	c.Flags().StringVarP(&app, "app", "a", "", "App name on kamubee (required)")
	c.Flags().IntVarP(&tail, "tail", "n", 200, "Number of historical lines to show before following")
	c.Flags().BoolVarP(&follow, "follow", "f", false, "Live-tail (SSE); Ctrl-C to stop")
	return c
}

func runLogs(ctx context.Context, cfg *config.Config, app string, tail int, follow bool) error {
	base := strings.TrimRight(resolveKamubee(cfg), "/")
	u := fmt.Sprintf("%s/v1/apps/%s/logs?tail=%d", base, url.PathEscape(app), tail)
	if follow {
		u += "&follow=true"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	tok := strings.TrimSpace(os.Getenv(config.EnvAccessToken))
	if tok == "" {
		tok = strings.TrimSpace(cfg.AccessToken)
	}
	if tok == "" {
		return fmt.Errorf("no kamuid access token — run `kamu auth login` first")
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	if cfg.ActiveOrg != "" {
		req.Header.Set("X-Kamu-Org", cfg.ActiveOrg)
	}
	hc := &http.Client{Timeout: 0} // no overall timeout for follow
	resp, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("kamubee: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	ct := resp.Header.Get("Content-Type")
	if !follow || !strings.HasPrefix(ct, "text/event-stream") {
		// History-only mode (or server fell back to plain text).
		_, err := io.Copy(os.Stdout, resp.Body)
		return err
	}
	return streamSSE(resp.Body)
}

// streamSSE parses lines of `data: {...}\n\n` and prints a tidy
// representation of each Envelope.
func streamSSE(body io.Reader) error {
	sc := bufio.NewScanner(body)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var env struct {
			TS        string `json:"ts"`
			App       string `json:"app"`
			MachineID string `json:"machine_id"`
			Host      string `json:"host"`
			Stream    string `json:"stream"`
			Line      string `json:"line"`
		}
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &env); err != nil {
			continue
		}
		ts := env.TS
		if t, err := time.Parse(time.RFC3339Nano, env.TS); err == nil {
			ts = t.Local().Format("15:04:05.000")
		}
		fmt.Fprintf(os.Stdout, "%s %s/%s %s\n", ts, env.MachineID, env.Stream, env.Line)
	}
	return sc.Err()
}

func resolveKamubee(cfg *config.Config) string {
	if v := strings.TrimSpace(os.Getenv("KAMU_KAMUBEE_URL")); v != "" {
		return v
	}
	if cfg != nil && strings.TrimSpace(cfg.Endpoints.Kamubee) != "" {
		return cfg.Endpoints.Kamubee
	}
	return defaultKamubeeBase
}
