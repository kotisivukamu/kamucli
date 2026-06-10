package auth

import (
	"github.com/spf13/cobra"

	"github.com/kotisivukamu/kamu-cli/internal/command"
)

func New() *cobra.Command {
	cmd := command.New("auth", "Authenticate with kamuid", "", nil)
	cmd.AddCommand(
		newLogin(),
		newLogout(),
		newWhoami(),
		newToken(),
	)
	return cmd
}
