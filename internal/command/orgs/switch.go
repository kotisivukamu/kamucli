package orgs

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/kotisivukamu/kamucli/internal/command"
	"github.com/kotisivukamu/kamucli/internal/config"
	"github.com/kotisivukamu/kamucli/internal/iostreams"
)

func newSwitch() *cobra.Command {
	cmd := command.New("switch <slug>", "Switch the active organization", "",
		func(ctx context.Context, args []string) error { return runSwitch(ctx, args) })
	cmd.Args = cobra.ExactArgs(1)
	return cmd
}

func runSwitch(ctx context.Context, args []string) error {
	io := iostreams.FromContext(ctx)
	target := args[0]

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	orgs, err := loadOrgs(ctx, io, cfg)
	if err != nil {
		return err
	}
	if len(orgs) == 0 {
		return fmt.Errorf("you are not a member of any organization")
	}

	match := matchOrg(orgs, target)
	if match == nil {
		return fmt.Errorf("no organization matches %q; available: %s", target, strings.Join(slugs(orgs), ", "))
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
