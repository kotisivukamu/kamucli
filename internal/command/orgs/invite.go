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

func newInvite() *cobra.Command {
	var (
		orgFlag string
		role    string
	)
	cmd := command.New("invite <email>", "Invite someone to an organization",
		"Invite an email address to an organization (owner/admin only).\n"+
			"Defaults to the active organization (see `kamu orgs switch`).\n\n"+reloginNote,
		func(ctx context.Context, args []string) error {
			io := iostreams.FromContext(ctx)
			email := strings.TrimSpace(args[0])
			if email == "" || !strings.Contains(email, "@") {
				return fmt.Errorf("%q does not look like an email address", args[0])
			}
			role = strings.ToLower(strings.TrimSpace(role))
			if role != "member" && role != "admin" {
				return fmt.Errorf("invalid --role %q: must be member or admin", role)
			}

			c, cfg, err := api(ctx)
			if err != nil {
				return err
			}
			org, err := resolveOrg(ctx, c, cfg, orgFlag)
			if err != nil {
				return err
			}

			inv, err := c.Invite(ctx, org.ID, kamuid.InviteInput{Email: email, Role: role})
			if err != nil {
				return friendly(err)
			}
			fmt.Fprintf(io.Out, "Invited %s to %s as %s.\n", inv.Email, org.Slug, inv.Role)
			return nil
		})
	cmd.Args = cobra.ExactArgs(1)
	f := cmd.Flags()
	f.StringVar(&orgFlag, "org", "", "organization slug or id (default: the active org)")
	f.StringVar(&role, "role", "member", "role for the invitee: member or admin")
	return cmd
}
