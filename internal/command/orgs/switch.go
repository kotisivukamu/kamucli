package orgs

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/kotisivukamu/kamucli/internal/client/kamuid"
	"github.com/kotisivukamu/kamucli/internal/command"
	"github.com/kotisivukamu/kamucli/internal/config"
	"github.com/kotisivukamu/kamucli/internal/iostreams"
)

func newSwitch() *cobra.Command {
	cmd := command.New("switch", "Switch the active organization", "",
		func(ctx context.Context, args []string) error { return runSwitch(ctx, args) })
	cmd.Args = cobra.ExactArgs(1)
	cmd.Use = "switch <slug>"
	return cmd
}

func runSwitch(ctx context.Context, args []string) error {
	io := iostreams.FromContext(ctx)
	target := args[0]

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

	var match *kamuid.Organization
	slugs := make([]string, 0, len(claims.Organizations))
	for i, o := range claims.Organizations {
		slugs = append(slugs, o.Slug)
		if o.Slug == target {
			match = &claims.Organizations[i]
		}
	}
	if match == nil {
		sort.Strings(slugs)
		if len(slugs) == 0 {
			return fmt.Errorf("no organizations on this token")
		}
		return fmt.Errorf("org %q not on this token; available: %s", target, strings.Join(slugs, ", "))
	}

	if cfg.ActiveOrg == match.Slug {
		fmt.Fprintf(io.ErrOut, "Already on %s (%s).\n", match.Slug, match.Name)
		return nil
	}
	cfg.ActiveOrg = match.Slug
	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	fmt.Fprintf(io.ErrOut, "Active organization set to %s (%s).\n", match.Slug, match.Name)
	return nil
}
