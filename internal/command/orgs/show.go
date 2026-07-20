package orgs

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/kotisivukamu/kamucli/internal/command"
	"github.com/kotisivukamu/kamucli/internal/iostreams"
	"github.com/kotisivukamu/kamucli/internal/render"
)

func newShow() *cobra.Command {
	var asJSON bool
	cmd := command.New("show [slug-or-id]", "Show an organization: info, members, pending invitations",
		"Show an organization's details, its members, and pending invitations.\n"+
			"Defaults to the active organization (see `kamu orgs switch`).\n\n"+reloginNote,
		func(ctx context.Context, args []string) error {
			io := iostreams.FromContext(ctx)
			arg := ""
			if len(args) == 1 {
				arg = args[0]
			}

			c, cfg, err := api(ctx)
			if err != nil {
				return err
			}
			org, err := resolveOrg(ctx, c, cfg, arg)
			if err != nil {
				return err
			}
			detail, err := c.GetOrg(ctx, org.ID)
			if err != nil {
				return friendly(err)
			}
			// The list row carries the caller's role; the detail's organization
			// object may not — prefer whichever is set.
			if detail.Organization.Role == "" {
				detail.Organization.Role = org.Role
			}

			if asJSON {
				return render.JSON(io.Out, detail)
			}

			o := detail.Organization
			info := [][]string{
				{"slug", o.Slug},
				{"name", o.Name},
				{"id", o.ID},
			}
			if o.Role != "" {
				info = append(info, []string{"your role", o.Role})
			}
			if o.Slug == cfg.ActiveOrg {
				info = append(info, []string{"active", "*"})
			}
			if err := render.Table(io.Out, nil, info); err != nil {
				return err
			}

			fmt.Fprintf(io.Out, "\nMembers (%d)\n", len(detail.Members))
			if len(detail.Members) > 0 {
				rows := make([][]string, 0, len(detail.Members))
				for _, m := range detail.Members {
					rows = append(rows, []string{m.Email, m.Name, m.Role, m.UserID})
				}
				if err := render.Table(io.Out, []string{"EMAIL", "NAME", "ROLE", "USER ID"}, rows); err != nil {
					return err
				}
			}

			if len(detail.Invitations) > 0 {
				fmt.Fprintf(io.Out, "\nPending invitations (%d)\n", len(detail.Invitations))
				rows := make([][]string, 0, len(detail.Invitations))
				for _, inv := range detail.Invitations {
					rows = append(rows, []string{inv.Email, inv.Role, expiresCol(inv.ExpiresAt)})
				}
				if err := render.Table(io.Out, []string{"EMAIL", "ROLE", "EXPIRES"}, rows); err != nil {
					return err
				}
			}
			return nil
		})
	cmd.Args = cobra.MaximumNArgs(1)
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output JSON")
	return cmd
}

func expiresCol(s string) string {
	if len(s) >= 10 {
		return s[:10]
	}
	if s == "" {
		return "-"
	}
	return s
}
