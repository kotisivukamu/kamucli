package orgs

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/kotisivukamu/kamucli/internal/client/kamuid"
	"github.com/kotisivukamu/kamucli/internal/command"
	"github.com/kotisivukamu/kamucli/internal/iostreams"
)

func newCreate() *cobra.Command {
	var slug string
	cmd := command.New("create <name>", "Create an organization (you become its owner)",
		"Create an organization on kamuid. You become its owner.\n\n"+reloginNote,
		func(ctx context.Context, args []string) error {
			io := iostreams.FromContext(ctx)
			name := strings.TrimSpace(args[0])
			if name == "" {
				return fmt.Errorf("organization name must not be empty")
			}

			c, _, err := api(ctx)
			if err != nil {
				return err
			}
			org, err := c.CreateOrg(ctx, kamuid.CreateOrgInput{Name: name, Slug: strings.TrimSpace(slug)})
			if err != nil {
				return friendly(err)
			}

			fmt.Fprintf(io.Out, "Created organization %s (%s, id %s).\n", org.Slug, org.Name, org.ID)
			fmt.Fprintf(io.ErrOut, "Make it the active org with `kamu orgs switch %s`.\n", org.Slug)
			return nil
		})
	cmd.Args = cobra.ExactArgs(1)
	cmd.Flags().StringVar(&slug, "slug", "", "URL slug for the organization (default: derived from the name)")
	return cmd
}
