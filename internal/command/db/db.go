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
		stub("list", "List databases"),
		stub("get", "Get database details"),
		stub("create", "Create a database"),
		stub("delete", "Delete a database"),
		stub("suspend", "Suspend a database"),
		stub("resume", "Resume a database"),
		stub("connstr", "Print a connection string for a database"),
	)
	return cmd
}

func stub(use, short string) *cobra.Command {
	return command.New(use, short, "", func(ctx context.Context, _ []string) error {
		return errors.New("not implemented yet")
	})
}
