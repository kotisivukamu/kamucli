package bee

import (
	"context"
	"errors"

	"github.com/spf13/cobra"

	"github.com/kotisivukamu/kamucli/internal/command"
)

func New() *cobra.Command {
	cmd := command.New("bee", "Manage kamubee apps and releases", "", nil)
	cmd.AddCommand(
		stub("apps", "List apps", "M4"),
		stub("deploy", "Deploy a release from kamu.toml", "M4"),
		stub("status", "Show app status", "M4"),
		newLogsCmd(),
		stub("destroy", "Destroy an app", "M4"),
	)
	return cmd
}

func stub(use, short, milestone string) *cobra.Command {
	return command.New(use, short, "", func(ctx context.Context, _ []string) error {
		return errors.New("not implemented yet — tracked in kotisivukamu/kamucli#1 " + milestone)
	})
}
