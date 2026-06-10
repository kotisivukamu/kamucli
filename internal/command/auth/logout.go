package auth

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/kotisivukamu/kamu-cli/internal/command"
	"github.com/kotisivukamu/kamu-cli/internal/config"
	"github.com/kotisivukamu/kamu-cli/internal/iostreams"
)

func newLogout() *cobra.Command {
	return command.New("logout", "Remove stored credentials", "", func(ctx context.Context, _ []string) error {
		io := iostreams.FromContext(ctx)
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		if cfg.AccessToken == "" && cfg.RefreshToken == "" && cfg.IDToken == "" {
			fmt.Fprintln(io.ErrOut, "Not logged in.")
			return nil
		}
		cfg.AccessToken = ""
		cfg.RefreshToken = ""
		cfg.IDToken = ""
		cfg.ActiveOrg = ""
		if err := config.Save(cfg); err != nil {
			return fmt.Errorf("save config: %w", err)
		}
		fmt.Fprintln(io.ErrOut, "Logged out.")
		return nil
	})
}
