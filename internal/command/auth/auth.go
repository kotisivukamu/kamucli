package auth

import (
	"context"
	"errors"

	"github.com/spf13/cobra"

	"github.com/kotisivukamu/kamu-cli/internal/command"
)

func New() *cobra.Command {
	cmd := command.New("auth", "Authenticate with kamuid", "", nil)
	cmd.AddCommand(
		login(),
		logout(),
		whoami(),
		token(),
	)
	return cmd
}

func login() *cobra.Command {
	return command.New("login", "Log in to kamuid", "", func(ctx context.Context, _ []string) error {
		return errors.New("not implemented yet — tracked in kotisivukamu/kamu-cli#1 M1")
	})
}

func logout() *cobra.Command {
	return command.New("logout", "Remove stored credentials", "", func(ctx context.Context, _ []string) error {
		return errors.New("not implemented yet — tracked in kotisivukamu/kamu-cli#1 M1")
	})
}

func whoami() *cobra.Command {
	return command.New("whoami", "Show the authenticated identity", "", func(ctx context.Context, _ []string) error {
		return errors.New("not implemented yet — tracked in kotisivukamu/kamu-cli#1 M1")
	})
}

func token() *cobra.Command {
	return command.New("token", "Print the current access token", "", func(ctx context.Context, _ []string) error {
		return errors.New("not implemented yet — tracked in kotisivukamu/kamu-cli#1 M1")
	})
}
