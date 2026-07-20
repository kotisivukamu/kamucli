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

	"github.com/kotisivukamu/kamucli/internal/client/kamuid"
	"github.com/kotisivukamu/kamucli/internal/command"
	"github.com/kotisivukamu/kamucli/internal/iostreams"
)

func newRemove() *cobra.Command {
	var (
		orgFlag string
		yes     bool
	)
	cmd := command.New("remove <email-or-user-id>", "Remove a member from an organization",
		"Remove a member from an organization (owner/admin only; the server enforces\n"+
			"owner-removal rules). Defaults to the active organization.\n\n"+reloginNote,
		func(ctx context.Context, args []string) error {
			io := iostreams.FromContext(ctx)

			c, cfg, err := api(ctx)
			if err != nil {
				return err
			}
			org, err := resolveOrg(ctx, c, cfg, orgFlag)
			if err != nil {
				return err
			}
			detail, err := c.GetOrg(ctx, org.ID)
			if err != nil {
				return friendly(err)
			}
			member := matchMember(detail.Members, args[0])
			if member == nil {
				return fmt.Errorf("no member of %s matches %q (see `kamu orgs show %s`)", org.Slug, args[0], org.Slug)
			}

			label := member.Email
			if label == "" {
				label = member.UserID
			}
			if !yes {
				if !term.IsTerminal(int(os.Stdin.Fd())) {
					return errors.New("refusing to remove without confirmation; pass --yes")
				}
				ok := false
				if err := huh.NewConfirm().
					Title(fmt.Sprintf("Remove %s from %s?", label, org.Slug)).
					Description(fmt.Sprintf("Role: %s. They lose access to the organization immediately.", member.Role)).
					Affirmative("Remove").Negative("Cancel").
					Value(&ok).Run(); err != nil {
					return err
				}
				if !ok {
					fmt.Fprintln(io.ErrOut, "Cancelled.")
					return nil
				}
			}

			if err := c.RemoveMember(ctx, org.ID, member.UserID); err != nil {
				return friendly(err)
			}
			fmt.Fprintf(io.Out, "Removed %s from %s.\n", label, org.Slug)
			return nil
		})
	cmd.Args = cobra.ExactArgs(1)
	f := cmd.Flags()
	f.StringVar(&orgFlag, "org", "", "organization slug or id (default: the active org)")
	f.BoolVar(&yes, "yes", false, "skip the confirmation prompt")
	return cmd
}

// matchMember finds arg among members by user id (exact) or email
// (case-insensitive).
func matchMember(members []kamuid.Member, arg string) *kamuid.Member {
	want := strings.ToLower(strings.TrimSpace(arg))
	for i := range members {
		if members[i].UserID == arg || strings.ToLower(members[i].Email) == want {
			return &members[i]
		}
	}
	return nil
}
