package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/kotisivukamu/kamucli/internal/command"
	"github.com/kotisivukamu/kamucli/internal/config"
	"github.com/kotisivukamu/kamucli/internal/iostreams"
)

func newToken() *cobra.Command {
	return command.New("token", "Print the current access token", "", func(ctx context.Context, _ []string) error {
		io := iostreams.FromContext(ctx)
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		if cfg.AccessToken == "" {
			return errors.New(`not logged in — run "kamu auth login"`)
		}
		fmt.Fprintln(io.Out, cfg.AccessToken)
		return nil
	})
}
