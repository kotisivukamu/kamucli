package status

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/kotisivukamu/kamucli/internal/client/kamustatus"
	"github.com/kotisivukamu/kamucli/internal/command"
	"github.com/kotisivukamu/kamucli/internal/config"
	"github.com/kotisivukamu/kamucli/internal/iostreams"
)

func newPage() *cobra.Command {
	var asJSON bool
	cmd := command.New("page", "Fetch a public status page by slug", "", func(ctx context.Context, args []string) error {
		// page is public, no API key needed — build a client from URL only.
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		baseURL := os.Getenv(EnvURL)
		if baseURL == "" {
			baseURL = cfg.Endpoints.Kamustatus
		}
		c := kamustatus.New(baseURL, "")
		data, err := c.GetPublicStatus(ctxOrTodo(ctx), args[0])
		if err != nil {
			return err
		}
		io := iostreams.FromContext(ctx)
		if asJSON {
			_, err := io.Out.Write(data)
			return err
		}
		var page struct {
			Name     string `json:"name"`
			Monitors []struct {
				Name   string  `json:"name"`
				Type   string  `json:"type"`
				Target string  `json:"target"`
				Uptime *string `json:"uptimePercent"`
				Status string  `json:"status"`
			} `json:"monitors"`
		}
		if err := json.Unmarshal(data, &page); err != nil {
			return err
		}
		fmt.Fprintf(io.Out, "%s\n\n", page.Name)
		for _, m := range page.Monitors {
			uptime := "—"
			if m.Uptime != nil {
				uptime = *m.Uptime + "%"
			}
			fmt.Fprintf(io.Out, "  [%s] %-30s %s  uptime %s\n", m.Status, m.Name, m.Target, uptime)
		}
		return nil
	})
	cmd.Args = cobra.ExactArgs(1)
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output JSON")
	return cmd
}
