package orgs

import (
	"context"
	"errors"
	"fmt"

	"github.com/charmbracelet/huh"

	"github.com/kotisivukamu/kamu-cli/internal/client/kamuid"
	"github.com/kotisivukamu/kamu-cli/internal/config"
	"github.com/kotisivukamu/kamu-cli/internal/iostreams"
)

func runPicker(ctx context.Context) error {
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
	if len(claims.Organizations) == 0 {
		fmt.Fprintln(io.ErrOut, "No organizations on this token.")
		return nil
	}

	opts := make([]huh.Option[string], 0, len(claims.Organizations))
	for _, o := range claims.Organizations {
		label := fmt.Sprintf("%s — %s (%s)", o.Slug, o.Name, o.Role)
		if o.Slug == cfg.ActiveOrg {
			label += "  •  current"
		}
		opts = append(opts, huh.NewOption(label, o.Slug))
	}

	picked := cfg.ActiveOrg
	if picked == "" {
		picked = claims.Organizations[0].Slug
	}

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Active organization").
				Options(opts...).
				Value(&picked),
		),
	).WithShowHelp(true)

	if err := form.RunWithContext(ctx); err != nil {
		if errors.Is(err, huh.ErrUserAborted) || errors.Is(err, context.Canceled) {
			return nil
		}
		return fmt.Errorf("picker: %w", err)
	}

	if picked == cfg.ActiveOrg {
		fmt.Fprintf(io.ErrOut, "Already on %s.\n", picked)
		return nil
	}
	cfg.ActiveOrg = picked
	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	for _, o := range claims.Organizations {
		if o.Slug == picked {
			fmt.Fprintf(io.ErrOut, "Active organization set to %s (%s).\n", o.Slug, o.Name)
			return nil
		}
	}
	return nil
}
