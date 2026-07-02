package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/kotisivukamu/kamucli/internal/client/kamuid"
	"github.com/kotisivukamu/kamucli/internal/command"
	"github.com/kotisivukamu/kamucli/internal/config"
	"github.com/kotisivukamu/kamucli/internal/iostreams"
	"github.com/kotisivukamu/kamucli/internal/render"
)

func newWhoami() *cobra.Command {
	var asJSON bool
	cmd := command.New("whoami", "Show the authenticated identity", "", func(ctx context.Context, _ []string) error {
		io := iostreams.FromContext(ctx)
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		if cfg.IDToken == "" {
			return errors.New(`not logged in — run "kamu auth login"`)
		}
		claims, err := kamuid.ParseIDTokenClaims(cfg.IDToken)
		if err != nil {
			return fmt.Errorf("parse id_token: %w", err)
		}
		if asJSON {
			return render.JSON(io.Out, claims)
		}
		rows := [][]string{
			{"subject", claims.Subject},
			{"email", claims.Email},
			{"issuer", claims.Issuer},
		}
		if claims.Name != "" {
			rows = append(rows, []string{"name", claims.Name})
		}
		if cfg.ActiveOrg != "" {
			rows = append(rows, []string{"active org", cfg.ActiveOrg})
		}
		rows = append(rows, []string{"orgs", fmt.Sprintf("%d", len(claims.Organizations))})
		return render.Table(io.Out, nil, rows)
	})
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output JSON")
	return cmd
}
