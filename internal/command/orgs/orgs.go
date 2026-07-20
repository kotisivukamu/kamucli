package orgs

import (
	"github.com/spf13/cobra"

	"github.com/kotisivukamu/kamucli/internal/command"
	"github.com/kotisivukamu/kamucli/internal/iostreams"
)

// reloginNote is appended to the long help of commands that hit kamuid's
// /v1/rp org endpoints as the user (not with the kamuhub access key).
const reloginNote = "Authenticates as you against kamuid (not with the kamuhub access key). If your\n" +
	"stored login predates the org-management scopes (kamu.org.profile.read,\n" +
	"kamu.org.manage), the command fails asking you to run `kamu login` again."

func New() *cobra.Command {
	cmd := command.New("orgs", "Manage organizations",
		"Manage your organizations on kamuid: list them, switch the active one, and\n"+
			"create, inspect, delete, invite to, and remove members from them.\n\n"+reloginNote,
		nil)
	cmd.AddCommand(
		newList(),
		newSwitch(),
		newCreate(),
		newShow(),
		newDelete(),
		newInvite(),
		newRemove(),
	)
	cmd.RunE = func(c *cobra.Command, _ []string) error {
		ctx := c.Context()
		if !iostreams.FromContext(ctx).IsStdoutTTY() {
			return c.Help()
		}
		return runPicker(ctx)
	}
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	return cmd
}
