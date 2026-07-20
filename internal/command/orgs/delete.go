package orgs

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/kotisivukamu/kamucli/internal/command"
	"github.com/kotisivukamu/kamucli/internal/config"
	"github.com/kotisivukamu/kamucli/internal/iostreams"
)

func newDelete() *cobra.Command {
	var yes bool
	cmd := command.New("delete <slug-or-id>", "Delete an organization (owner only)",
		"Delete an organization on kamuid. Owner only; the server refuses to delete\n"+
			"your last organization. This permanently removes the org, its memberships,\n"+
			"and pending invitations.\n\n"+reloginNote,
		func(ctx context.Context, args []string) error {
			io := iostreams.FromContext(ctx)

			c, cfg, err := api(ctx)
			if err != nil {
				return err
			}
			org, err := resolveOrg(ctx, c, cfg, args[0])
			if err != nil {
				return err
			}

			if !yes {
				if !term.IsTerminal(int(os.Stdin.Fd())) {
					return errors.New("refusing to delete without confirmation; pass --yes")
				}
				var typed string
				if err := huh.NewInput().
					Title(fmt.Sprintf("Delete organization %s (%s)?", org.Slug, org.Name)).
					Description(fmt.Sprintf("This permanently removes the org, its memberships, and pending invitations.\nType the slug %q to confirm.", org.Slug)).
					Value(&typed).Run(); err != nil {
					if errors.Is(err, huh.ErrUserAborted) {
						fmt.Fprintln(io.ErrOut, "Cancelled.")
						return nil
					}
					return err
				}
				if strings.TrimSpace(typed) != org.Slug {
					fmt.Fprintln(io.ErrOut, "Slug did not match — cancelled.")
					return nil
				}
			}

			if err := c.DeleteOrg(ctx, org.ID); err != nil {
				return friendly(err)
			}
			fmt.Fprintf(io.Out, "Deleted organization %s.\n", org.Slug)

			if cfg.ActiveOrg == org.Slug {
				cfg.ActiveOrg = ""
				if err := config.Save(cfg); err != nil {
					return fmt.Errorf("clear active org: %w", err)
				}
				fmt.Fprintln(io.ErrOut, "It was the active org — pick a new one with `kamu orgs switch <slug>`.")
			}
			return nil
		})
	cmd.Args = cobra.ExactArgs(1)
	cmd.Flags().BoolVar(&yes, "yes", false, "skip the confirmation prompt")
	return cmd
}
