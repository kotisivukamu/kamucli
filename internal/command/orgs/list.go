package orgs

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/kotisivukamu/kamucli/internal/client/kamuid"
	"github.com/kotisivukamu/kamucli/internal/command"
	"github.com/kotisivukamu/kamucli/internal/config"
	"github.com/kotisivukamu/kamucli/internal/iostreams"
	"github.com/kotisivukamu/kamucli/internal/render"
)

func newList() *cobra.Command {
	var asJSON bool
	cmd := command.New("list", "List your organizations", "", func(ctx context.Context, _ []string) error {
		io := iostreams.FromContext(ctx)
		cfg, err := config.Load()
		if err != nil {
			return err
		}

		orgs, err := loadOrgs(ctx, io, cfg)
		if err != nil {
			return err
		}

		if asJSON {
			type entry struct {
				kamuid.Organization
				Active bool `json:"active"`
			}
			out := make([]entry, len(orgs))
			for i, o := range orgs {
				out[i] = entry{Organization: o, Active: o.Slug == cfg.ActiveOrg}
			}
			return render.JSON(io.Out, out)
		}

		if len(orgs) == 0 {
			fmt.Fprintln(io.ErrOut, "No organizations.")
			return nil
		}

		rows := make([][]string, 0, len(orgs))
		for _, o := range orgs {
			active := ""
			if o.Slug == cfg.ActiveOrg {
				active = "*"
			}
			rows = append(rows, []string{o.Slug, o.Name, o.Role, active})
		}
		return render.Table(io.Out, []string{"SLUG", "NAME", "ROLE", "ACTIVE"}, rows)
	})
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output JSON")
	return cmd
}
