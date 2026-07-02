package db

import (
	"context"
	"errors"

	"github.com/spf13/cobra"

	"github.com/kotisivukamu/kamucli/internal/command"
)

func New() *cobra.Command {
	cmd := command.New("db", "Manage kamudb databases", "", nil)
	cmd.AddCommand(
		stub("list", "List databases", "M3"),
		stub("get", "Get database details", "M3"),
		stub("create", "Create a database", "M3"),
		stub("delete", "Delete a database", "M3"),
		stub("suspend", "Suspend a database", "M3"),
		stub("resume", "Resume a database", "M3"),
		stub("connstr", "Print a connection string for a database", "M3"),
	)
	return cmd
}

func stub(use, short, milestone string) *cobra.Command {
	return command.New(use, short, "", func(ctx context.Context, _ []string) error {
		return errors.New("not implemented yet — tracked in kotisivukamu/kamucli#1 " + milestone)
	})
}
