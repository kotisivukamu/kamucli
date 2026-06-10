package dns

import (
	"context"
	"errors"

	"github.com/spf13/cobra"

	"github.com/kotisivukamu/kamu-cli/internal/command"
)

func New() *cobra.Command {
	cmd := command.New("dns", "Manage kamudns zones and records", "", nil)
	cmd.AddCommand(
		stub("zones", "List domains/zones", "M5"),
		stub("get", "Get domain details", "M5"),
		stub("search", "Check domain availability", "M5"),
		stub("records", "Manage DNS records", "M5"),
	)
	return cmd
}

func stub(use, short, milestone string) *cobra.Command {
	return command.New(use, short, "", func(ctx context.Context, _ []string) error {
		return errors.New("not implemented yet — tracked in kotisivukamu/kamu-cli#1 " + milestone)
	})
}
