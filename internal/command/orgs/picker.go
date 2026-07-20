package orgs

import (
	"context"
	"errors"
	"fmt"

	"github.com/kotisivukamu/kamucli/internal/config"
	"github.com/kotisivukamu/kamucli/internal/iostreams"
	"github.com/kotisivukamu/kamucli/internal/picker"
)

func runPicker(ctx context.Context) error {
	io := iostreams.FromContext(ctx)

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	orgs, err := loadOrgs(ctx, io, cfg)
	if err != nil {
		return err
	}
	if len(orgs) == 0 {
		fmt.Fprintln(io.ErrOut, "No organizations.")
		return nil
	}

	opts := make([]picker.Option[string], 0, len(orgs))
	for _, o := range orgs {
		label := fmt.Sprintf("%s — %s", o.Slug, o.Name)
		if o.Slug == cfg.ActiveOrg {
			label += "  •  current"
		}
		opts = append(opts, picker.Option[string]{
			Value:       o.Slug,
			Label:       label,
			Description: fmt.Sprintf("role: %s", o.Role),
		})
	}

	picked, err := picker.Pick(ctx, picker.Config[string]{
		Title:       "Active organization",
		Description: "Type to filter, enter to select, esc to cancel.",
		Options:     opts,
		Default:     cfg.ActiveOrg,
	})
	if errors.Is(err, picker.ErrCanceled) {
		return nil
	}
	if err != nil {
		return err
	}

	if picked == cfg.ActiveOrg {
		fmt.Fprintf(io.ErrOut, "Already on %s.\n", picked)
		return nil
	}
	cfg.ActiveOrg = picked
	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	for _, o := range orgs {
		if o.Slug == picked {
			fmt.Fprintf(io.ErrOut, "Active organization set to %s (%s).\n", o.Slug, o.Name)
			return nil
		}
	}
	return nil
}
