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
		stub("apps", "List apps"),
		stub("deploy", "Deploy a release from kamu.toml"),
		stub("status", "Show app status"),
		newLogsCmd(),
		stub("destroy", "Destroy an app"),
	)
	return cmd
}

func stub(use, short string) *cobra.Command {
	return command.New(use, short, "", func(ctx context.Context, _ []string) error {
		return errors.New("not implemented yet")
	})
}
