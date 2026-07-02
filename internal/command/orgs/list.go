package orgs

import (
	"context"
	"errors"
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
	cmd := command.New("list", "List organizations on the current token", "", func(ctx context.Context, _ []string) error {
		io := iostreams.FromContext(ctx)
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		if cfg.IDToken == "" {
			return errors.New(`not logged in — run "kamu auth login"`)
		}
		claims, err := kamuid.ParseIDTokenClaims(cfg.IDToken)
		if err != nil {
			return fmt.Errorf("parse id_token: %w", err)
		}

		if asJSON {
			type entry struct {
				kamuid.Organization
				Active bool `json:"active"`
			}
			out := make([]entry, len(claims.Organizations))
			for i, o := range claims.Organizations {
				out[i] = entry{Organization: o, Active: o.Slug == cfg.ActiveOrg}
			}
			return render.JSON(io.Out, out)
		}

		if len(claims.Organizations) == 0 {
			fmt.Fprintln(io.ErrOut, "No organizations on this token.")
			return nil
		}

		rows := make([][]string, 0, len(claims.Organizations))
		for _, o := range claims.Organizations {
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
