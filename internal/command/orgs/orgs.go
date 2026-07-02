package orgs

import (
	"github.com/spf13/cobra"

	"github.com/kotisivukamu/kamucli/internal/command"
	"github.com/kotisivukamu/kamucli/internal/iostreams"
)

func New() *cobra.Command {
	cmd := command.New("orgs", "Manage the active organization", "", nil)
	cmd.AddCommand(newList(), newSwitch())
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
