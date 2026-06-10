package orgs

import (
	"context"
	"errors"

	"github.com/spf13/cobra"

	"github.com/kotisivukamu/kamu-cli/internal/command"
)

func New() *cobra.Command {
	cmd := command.New("orgs", "Manage the active organization", "", nil)
	cmd.AddCommand(list(), switchCmd())
	return cmd
}

func list() *cobra.Command {
	return command.New("list", "List organizations on the current token", "", func(ctx context.Context, _ []string) error {
		return errors.New("not implemented yet — tracked in kotisivukamu/kamu-cli#1 M2")
	})
}

func switchCmd() *cobra.Command {
	return command.New("switch", "Switch the active organization", "", func(ctx context.Context, args []string) error {
		return errors.New("not implemented yet — tracked in kotisivukamu/kamu-cli#1 M2")
	})
}
