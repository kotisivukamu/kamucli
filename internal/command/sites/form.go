package sites

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/kotisivukamu/kamucli/internal/client/kamusites"
	"github.com/kotisivukamu/kamucli/internal/command"
	"github.com/kotisivukamu/kamucli/internal/iostreams"
	"github.com/kotisivukamu/kamucli/internal/render"
)

// The `kamu sites form` group manages a site's KamuCDN relay destinations — the
// targets a live site can POST JSON to via /k/amu/relay/<id> on its own domain
// (contact forms, etc.). It is a noun subgroup under `sites` (verbs add/list/
// remove) rather than a flat `sites <verb> form`, so it groups cleanly alongside
// the other per-site management surfaces without colliding with sites' own
// create/list/delete verbs. `--site` identifies the site (id or slug) on every
// subcommand — uniform across the group; the destination is the primary
// argument on add (--target) and remove (positional id). "form" is the only
// kind the CLI creates (the server defaults kind='form').
func newForm() *cobra.Command {
	cmd := command.New("form", "Manage a site's relay form destinations", "", nil)
	cmd.AddCommand(newFormAdd())
	cmd.AddCommand(newFormList())
	cmd.AddCommand(newFormRemove())
	return cmd
}

// resolveSite maps a --site ref (id or slug) to a site the key can see. Same
// lookup `sites delete` does, so a friendly slug works.
func resolveSite(ctx context.Context, client *kamusites.Client, ref string) (*kamusites.Site, error) {
	if strings.TrimSpace(ref) == "" {
		return nil, errors.New("--site <id-or-slug> is required")
	}
	sites, err := client.Sites(ctx)
	if err != nil {
		return nil, err
	}
	for i := range sites {
		if sites[i].ID == ref || sites[i].Slug == ref {
			return &sites[i], nil
		}
	}
	return nil, fmt.Errorf("no site matches %q (try `kamu sites list`)", ref)
}

// explainDestErr turns the two actionable relay failures into plain guidance: a
// 400 carries the relay's SSRF slug (surfaced verbatim), a 503 means the relay
// isn't wired on this environment. Everything else passes through unchanged.
func explainDestErr(err error) error {
	var ae *kamusites.APIError
	if errors.As(err, &ae) {
		switch {
		case ae.StatusCode == 503 && strings.Contains(ae.Message, "relay_not_configured"):
			return errors.New("relay not enabled on this environment")
		case ae.StatusCode == 400:
			return fmt.Errorf("relay rejected the target: %s", ae.Message)
		}
	}
	return err
}

func newFormAdd() *cobra.Command {
	var (
		key, site, target, label, secret string
		ratePerHour, maxBody             int
	)
	cmd := command.New("add", "Create a relay form destination and wire it to a site", "", func(ctx context.Context, _ []string) error {
		if ctx == nil {
			ctx = context.TODO()
		}
		io := iostreams.FromContext(ctx)
		k, err := resolveKey(key)
		if err != nil {
			return err
		}
		if strings.TrimSpace(target) == "" {
			return errors.New("--target <url> is required")
		}
		client := kamusites.New(os.Getenv(envURL), k)
		s, err := resolveSite(ctx, client, site)
		if err != nil {
			return err
		}

		dest, err := client.CreateDestination(ctx, s.ID, kamusites.CreateDestinationInput{
			TargetURL:    strings.TrimSpace(target),
			Label:        strings.TrimSpace(label),
			Secret:       secret,
			MaxBodyBytes: maxBody,
			RatePerHour:  ratePerHour,
		})
		if err != nil {
			return explainDestErr(err)
		}

		fmt.Fprintf(io.Out, "Created form destination %s on %s\n", dest.ID, s.Slug)
		fmt.Fprintf(io.Out, "  ingest path: %s\n", dest.Path)
		if s.Domain != "" {
			fmt.Fprintf(io.Out, "  ingest URL:  https://%s%s\n", s.Domain, dest.Path)
		} else {
			fmt.Fprintln(io.Out, "  (connect a domain to the site to get its public ingest URL)")
		}
		fmt.Fprintf(io.Out, "  forwards to: %s\n", dest.TargetURL)
		if dest.HasSecret {
			fmt.Fprintln(io.Out, "  a secret is set; the relay delivers it to the target as the X-Kamu-Relay-Key header")
		}
		return nil
	})
	f := cmd.Flags()
	f.StringVar(&key, "key", "", "kamuhub access key (or "+envKey+")")
	f.StringVar(&site, "site", "", "site id or slug (required)")
	f.StringVar(&target, "target", "", "URL the relay forwards submissions to (required)")
	f.StringVar(&label, "label", "", "human-readable label")
	f.StringVar(&secret, "secret", "", "shared key delivered to the target as X-Kamu-Relay-Key")
	f.IntVar(&ratePerHour, "rate-per-hour", 0, "max submissions per hour (relay default when 0)")
	f.IntVar(&maxBody, "max-body", 0, "max request body in bytes (relay default when 0)")
	return cmd
}

func newFormList() *cobra.Command {
	var (
		key, site string
		asJSON    bool
	)
	cmd := command.New("list", "List a site's relay form destinations", "", func(ctx context.Context, _ []string) error {
		if ctx == nil {
			ctx = context.TODO()
		}
		io := iostreams.FromContext(ctx)
		k, err := resolveKey(key)
		if err != nil {
			return err
		}
		client := kamusites.New(os.Getenv(envURL), k)
		s, err := resolveSite(ctx, client, site)
		if err != nil {
			return err
		}

		dests, err := client.Destinations(ctx, s.ID)
		if err != nil {
			return explainDestErr(err)
		}
		if asJSON {
			return render.JSON(io.Out, dests)
		}
		if len(dests) == 0 {
			fmt.Fprintln(io.ErrOut, "No destinations for the site's org.")
			return nil
		}
		// The API already sorts this site's destinations first; the SCOPE column
		// distinguishes them ("site") from the org's other, unwired ones ("org").
		rows := make([][]string, 0, len(dests))
		for _, d := range dests {
			scope := "org"
			if d.Wired {
				scope = "site"
			}
			secret := "-"
			if d.HasSecret {
				secret = "yes"
			}
			rows = append(rows, []string{scope, d.ID, d.TargetURL, d.Label, d.Path, secret})
		}
		return render.Table(io.Out, []string{"SCOPE", "ID", "TARGET", "LABEL", "PATH", "SECRET"}, rows)
	})
	f := cmd.Flags()
	f.StringVar(&key, "key", "", "kamuhub access key (or "+envKey+")")
	f.StringVar(&site, "site", "", "site id or slug (required)")
	f.BoolVar(&asJSON, "json", false, "Output JSON")
	return cmd
}

func newFormRemove() *cobra.Command {
	var (
		key, site string
		yes       bool
	)
	cmd := command.New("remove", "Remove a relay form destination from a site", "", func(ctx context.Context, args []string) error {
		if ctx == nil {
			ctx = context.TODO()
		}
		io := iostreams.FromContext(ctx)
		k, err := resolveKey(key)
		if err != nil {
			return err
		}
		client := kamusites.New(os.Getenv(envURL), k)
		s, err := resolveSite(ctx, client, site)
		if err != nil {
			return err
		}
		destID := args[0]

		if !yes {
			if !term.IsTerminal(int(os.Stdin.Fd())) {
				return errors.New("refusing to remove without confirmation; pass --yes")
			}
			ok := false
			if err := huh.NewConfirm().
				Title(fmt.Sprintf("Remove destination %s from %s?", destID, s.Slug)).
				Description("This deletes the relay destination and unwires it from the site.").
				Affirmative("Remove").Negative("Cancel").
				Value(&ok).Run(); err != nil {
				return err
			}
			if !ok {
				fmt.Fprintln(io.ErrOut, "Cancelled.")
				return nil
			}
		}

		if err := client.DeleteDestination(ctx, s.ID, destID); err != nil {
			return explainDestErr(err)
		}
		fmt.Fprintf(io.Out, "Removed %s from %s.\n", destID, s.Slug)
		return nil
	})
	cmd.Args = cobra.ExactArgs(1)
	f := cmd.Flags()
	f.StringVar(&key, "key", "", "kamuhub access key (or "+envKey+")")
	f.StringVar(&site, "site", "", "site id or slug (required)")
	f.BoolVar(&yes, "yes", false, "skip the confirmation prompt")
	return cmd
}
