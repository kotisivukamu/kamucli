// Package sites implements `kamu sites` — create and build websites on kamusites
// (sites.kamuhub.com) from the CLI. Auth is a kamuhub access key (a scoped,
// signed platform context); export KAMU_ACCESS_KEY or pass --key.
package sites

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/kotisivukamu/kamucli/internal/client/kamusites"
	"github.com/kotisivukamu/kamucli/internal/command"
	"github.com/kotisivukamu/kamucli/internal/iostreams"
	"github.com/kotisivukamu/kamucli/internal/render"
)

const (
	envKey = "KAMU_ACCESS_KEY"
	envURL = "KAMU_KAMUSITES_URL"
)

// brief holds every wizard field as a string (lists are comma-separated), shared
// by the flags and the interactive form. Mirrors the dashboard create wizard
// (NewSite.tsx): Basics, Details, Google visibility.
type brief struct {
	name         string // the site name; defaults to business
	business     string
	existingURL  string
	tagline      string
	industry     string
	description  string
	tone         string
	primaryColor string
	services     string // comma-separated
	pages        string // comma-separated
	email        string
	phone        string
	address      string
	businessType string
	openingHours string
	instructions string
	// Brand + reference materials (uploaded as files, not part of the brief).
	logoPath     string
	logoPrompt   string
	materials    []string // --material, repeatable
	materialsCSV string   // interactive: comma-separated paths
}

func New() *cobra.Command {
	cmd := command.New("sites", "Create and build websites on kamusites", "", nil)
	cmd.AddCommand(newCreate())
	cmd.AddCommand(newList())
	cmd.AddCommand(newDelete())
	return cmd
}

// resolveKey pulls the access key from --key or the env, with the same guidance
// the create wizard gives. Shared by list/delete.
func resolveKey(key string) (string, error) {
	if key == "" {
		key = os.Getenv(envKey)
	}
	if key == "" {
		return "", errors.New("no access key. Create one in the dashboard (Manage -> Access keys) and pass it:\n\n    export " + envKey + "=...\n\nor --key <token>")
	}
	return key, nil
}

func newList() *cobra.Command {
	var key string
	var asJSON bool
	cmd := command.New("list", "List sites you can access", "", func(ctx context.Context, _ []string) error {
		if ctx == nil {
			ctx = context.TODO()
		}
		io := iostreams.FromContext(ctx)
		k, err := resolveKey(key)
		if err != nil {
			return err
		}
		client := kamusites.New(os.Getenv(envURL), k)

		sites, err := client.Sites(ctx)
		if err != nil {
			return err
		}
		if asJSON {
			return render.JSON(io.Out, sites)
		}
		if len(sites) == 0 {
			fmt.Fprintln(io.ErrOut, "No sites.")
			return nil
		}
		// team_id -> org slug, so each row shows which org owns the site (the whole
		// point of a cross-org list — spotting one built in the wrong org).
		orgBy := map[string]string{}
		if teams, err := client.Teams(ctx); err == nil {
			for _, t := range teams {
				orgBy[t.ID] = t.Slug
			}
		}
		rows := make([][]string, 0, len(sites))
		for _, s := range sites {
			status := "live"
			if s.IsDraft {
				status = "draft"
			}
			rows = append(rows, []string{s.Name, s.Slug, orgBy[s.TeamID], status, s.Domain, s.ID})
		}
		return render.Table(io.Out, []string{"NAME", "SLUG", "ORG", "STATUS", "DOMAIN", "ID"}, rows)
	})
	f := cmd.Flags()
	f.StringVar(&key, "key", "", "kamuhub access key (or "+envKey+")")
	f.BoolVar(&asJSON, "json", false, "Output JSON")
	return cmd
}

func newDelete() *cobra.Command {
	var key string
	var yes bool
	cmd := command.New("delete", "Delete a site — archives its repo and takes it offline", "", func(ctx context.Context, args []string) error {
		if ctx == nil {
			ctx = context.TODO()
		}
		io := iostreams.FromContext(ctx)
		k, err := resolveKey(key)
		if err != nil {
			return err
		}
		client := kamusites.New(os.Getenv(envURL), k)

		// Resolve the arg (id or slug) against the sites the key can see, so a
		// friendly slug works and we can show what's about to be deleted.
		sites, err := client.Sites(ctx)
		if err != nil {
			return err
		}
		want := args[0]
		var target *kamusites.Site
		for i := range sites {
			if sites[i].ID == want || sites[i].Slug == want {
				target = &sites[i]
				break
			}
		}
		if target == nil {
			return fmt.Errorf("no site matches %q (try `kamu sites list`)", want)
		}
		label := target.Name
		if label == "" {
			label = target.Slug
		}

		if !yes {
			if !term.IsTerminal(int(os.Stdin.Fd())) {
				return errors.New("refusing to delete without confirmation; pass --yes")
			}
			ok := false
			if err := huh.NewConfirm().
				Title(fmt.Sprintf("Delete %s?", label)).
				Description("This permanently deletes the site, archives its repo, and takes it offline.").
				Affirmative("Delete").Negative("Cancel").
				Value(&ok).Run(); err != nil {
				return err
			}
			if !ok {
				fmt.Fprintln(io.ErrOut, "Cancelled.")
				return nil
			}
		}

		if err := client.DeleteSite(ctx, target.ID); err != nil {
			return err
		}
		fmt.Fprintf(io.Out, "Deleted %s.\n", target.Slug)
		return nil
	})
	cmd.Args = cobra.ExactArgs(1)
	f := cmd.Flags()
	f.StringVar(&key, "key", "", "kamuhub access key (or "+envKey+")")
	f.BoolVar(&yes, "yes", false, "skip the confirmation prompt")
	return cmd
}

func newCreate() *cobra.Command {
	var (
		key, team string
		noWatch   bool
		b         brief
	)
	cmd := command.New("create", "Create a website and start its build", "", func(ctx context.Context, _ []string) error {
		if ctx == nil {
			ctx = context.TODO()
		}
		io := iostreams.FromContext(ctx)

		if key == "" {
			key = os.Getenv(envKey)
		}
		if key == "" {
			return errors.New("no access key. Create one in the dashboard (Manage -> Access keys) and pass it:\n\n    export " + envKey + "=...\n\nor --key <token>")
		}
		client := kamusites.New(os.Getenv(envURL), key)

		// Resolve the team to build in: the access key is scoped to one org, so
		// match it to a kamusites team by the shared kamuid_org_id.
		teams, err := client.Teams(ctx)
		if err != nil {
			return err
		}
		t, err := pickTeam(teams, team, key)
		if err != nil {
			return err
		}

		// Interactive wizard when attached to a terminal and nothing was given on
		// the command line; otherwise flags drive it (so an agent/CI can run it
		// non-interactively).
		if b.name == "" && b.business == "" && term.IsTerminal(int(os.Stdin.Fd())) {
			if err := b.prompt(); err != nil {
				return err
			}
		}
		name := strings.TrimSpace(b.name)
		if name == "" {
			name = strings.TrimSpace(b.business)
		}
		if name == "" {
			return errors.New("a site name (or --business) is required")
		}

		fmt.Fprintf(io.Out, "Creating site %q in %s...\n", name, t.Slug)
		site, err := client.CreateSite(ctx, t.ID, name)
		if err != nil {
			return err
		}
		fmt.Fprintf(io.Out, "Created %s (preview: https://%s.kskamu.app)\n", site.Slug, site.Slug)

		// Upload logo + reference materials onto the draft BEFORE building — the
		// builder downloads them by site id (same as the dashboard wizard).
		if err := b.attachAssets(ctx, io.Out, client, site.ID); err != nil {
			return err
		}

		fmt.Fprintln(io.Out, "Starting build...")
		if _, err := client.TriggerBuild(ctx, site.ID, b.toBrief(name), b.instructions); err != nil {
			return err
		}
		if noWatch {
			fmt.Fprintf(io.Out, "Build queued. Track it in the dashboard; live at https://%s.kskamu.app when done.\n", site.Slug)
			return nil
		}
		return watchBuild(ctx, client, site)
	})

	f := cmd.Flags()
	f.StringVar(&key, "key", "", "kamuhub access key (or "+envKey+")")
	f.StringVar(&team, "team", "", "team/org slug (defaults to the key's org)")
	f.BoolVar(&noWatch, "no-watch", false, "don't wait for the build to finish")
	// Brief fields (all optional except a name/business). Mirror the wizard.
	f.StringVar(&b.name, "name", "", "site name (defaults to --business)")
	f.StringVar(&b.business, "business", "", "business name")
	f.StringVar(&b.existingURL, "existing-url", "", "an existing site to draw from")
	f.StringVar(&b.tagline, "tagline", "", "short tagline")
	f.StringVar(&b.industry, "industry", "", "industry (e.g. Construction, Restaurant)")
	f.StringVar(&b.description, "description", "", "what the business is and what the site needs")
	f.StringVar(&b.tone, "tone", "", "tone of voice")
	f.StringVar(&b.primaryColor, "primary-color", "", "primary brand colour (hex)")
	f.StringVar(&b.services, "services", "", "comma-separated services")
	f.StringVar(&b.pages, "pages", "", "comma-separated pages (e.g. Home,Services,Contact)")
	f.StringVar(&b.email, "email", "", "contact email")
	f.StringVar(&b.phone, "phone", "", "contact phone")
	f.StringVar(&b.address, "address", "", "address")
	f.StringVar(&b.businessType, "business-type", "", "Google business type (e.g. Restaurant, Dentist)")
	f.StringVar(&b.openingHours, "hours", "", "opening hours")
	f.StringVar(&b.instructions, "instructions", "", "anything to do differently")
	f.StringVar(&b.logoPath, "logo", "", "path to a logo image to upload")
	f.StringVar(&b.logoPrompt, "logo-prompt", "", "describe a logo to AI-generate (if no --logo)")
	f.StringArrayVar(&b.materials, "material", nil, "path to a reference file to upload (repeatable)")
	return cmd
}

// attachAssets uploads the logo (file or AI-generated) and reference materials
// onto the draft. The builder picks them up from materials/<siteId>/.
func (b *brief) attachAssets(ctx context.Context, out io.Writer, client *kamusites.Client, siteID string) error {
	// A logo is optional polish — never let it block the build. A bad local path
	// is a user typo (hard error); a server-side failure (e.g. logo generation
	// unavailable) just warns and proceeds.
	switch {
	case strings.TrimSpace(b.logoPath) != "":
		data, err := os.ReadFile(b.logoPath)
		if err != nil {
			return fmt.Errorf("read logo: %w", err)
		}
		name := "logo" + strings.ToLower(filepath.Ext(b.logoPath))
		if _, err := client.UploadMaterial(ctx, siteID, name, name, data); err != nil {
			fmt.Fprintf(out, "warning: logo upload failed, continuing without it: %v\n", err)
		} else {
			fmt.Fprintf(out, "Uploaded logo (%s)\n", name)
		}
	case strings.TrimSpace(b.logoPrompt) != "":
		if _, err := client.GenerateLogo(ctx, siteID, b.logoPrompt); err != nil {
			fmt.Fprintf(out, "warning: logo generation failed, continuing without it: %v\n", err)
		} else {
			fmt.Fprintln(out, "Generated logo")
		}
	}

	paths := append([]string{}, b.materials...)
	paths = append(paths, splitCSV(b.materialsCSV)...)
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			return fmt.Errorf("read material %s: %w", p, err)
		}
		name := filepath.Base(p)
		if _, err := client.UploadMaterial(ctx, siteID, name, "", data); err != nil {
			return err
		}
		fmt.Fprintf(out, "Uploaded material (%s)\n", name)
	}
	return nil
}

// prompt runs the interactive wizard, grouped like the dashboard create form.
func (b *brief) prompt() error {
	required := func(s string) error {
		if strings.TrimSpace(s) == "" {
			return errors.New("required")
		}
		return nil
	}
	return huh.NewForm(
		huh.NewGroup(
			huh.NewNote().Title("Basics"),
			huh.NewInput().Title("Business name").Placeholder("e.g. Virtanen Construction").
				Value(&b.business).Validate(required),
			huh.NewInput().Title("Existing website (optional)").Placeholder("www.business.com").
				Value(&b.existingURL),
		),
		huh.NewGroup(
			huh.NewNote().Title("Details"),
			huh.NewInput().Title("Tagline").Placeholder("e.g. Reliable building since 1995").Value(&b.tagline),
			huh.NewInput().Title("Industry").Placeholder("e.g. Construction, Restaurant, Hair salon").Value(&b.industry),
			huh.NewText().Title("Description").Placeholder("Briefly describe the business and what the site needs…").Value(&b.description),
			huh.NewInput().Title("Services").Description("Comma-separated").Placeholder("e.g. Renovation, New builds, Design").Value(&b.services),
			huh.NewInput().Title("Pages").Description("Comma-separated").Placeholder("e.g. Home, Services, Contact, About").Value(&b.pages),
			huh.NewInput().Title("Tone of voice").Value(&b.tone),
			huh.NewInput().Title("Primary colour (optional)").Placeholder("#4f46e5").Value(&b.primaryColor),
		),
		huh.NewGroup(
			huh.NewNote().Title("Contact"),
			huh.NewInput().Title("Email").Placeholder("info@business.com").Value(&b.email),
			huh.NewInput().Title("Phone").Placeholder("+358 40 123 4567").Value(&b.phone),
			huh.NewInput().Title("Address").Placeholder("Example St 1, 00100 Helsinki").Value(&b.address),
		),
		huh.NewGroup(
			huh.NewNote().Title("Google visibility"),
			huh.NewInput().Title("Business type").Placeholder("e.g. Restaurant, Dentist, Electrician").Value(&b.businessType),
			huh.NewInput().Title("Opening hours").Placeholder("e.g. Mon–Fri 8–16, Sat 10–14").Value(&b.openingHours),
			huh.NewText().Title("Anything to do differently? (optional)").Value(&b.instructions),
		),
		huh.NewGroup(
			huh.NewNote().Title("Brand & materials"),
			huh.NewInput().Title("Logo image path (optional)").Placeholder("./logo.png").Value(&b.logoPath),
			huh.NewInput().Title("Or describe a logo to generate (optional)").Placeholder("Describe a logo…").Value(&b.logoPrompt),
			huh.NewInput().Title("Reference files (optional)").Description("Comma-separated paths").Placeholder("./brochure.pdf, ./photos.zip").Value(&b.materialsCSV),
		),
	).Run()
}

func (b *brief) toBrief(siteName string) kamusites.Brief {
	business := strings.TrimSpace(b.business)
	if business == "" {
		business = siteName
	}
	out := kamusites.Brief{
		BusinessName:           business,
		BusinessType:           strings.TrimSpace(b.businessType),
		Tagline:                strings.TrimSpace(b.tagline),
		Industry:               strings.TrimSpace(b.industry),
		Description:            strings.TrimSpace(b.description),
		OpeningHours:           strings.TrimSpace(b.openingHours),
		Email:                  strings.TrimSpace(b.email),
		Phone:                  strings.TrimSpace(b.phone),
		Address:                strings.TrimSpace(b.address),
		ExistingURL:            strings.TrimSpace(b.existingURL),
		Services:               splitCSV(b.services),
		Pages:                  splitCSV(b.pages),
		Tone:                   strings.TrimSpace(b.tone),
		AdditionalInstructions: strings.TrimSpace(b.instructions),
	}
	if c := strings.TrimSpace(b.primaryColor); c != "" {
		out.Colors = &kamusites.BriefColors{Primary: c}
	}
	return out
}

func splitCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// pickTeam matches the access key's scoped org to a kamusites team.
func pickTeam(teams []kamusites.Team, want, key string) (*kamusites.Team, error) {
	if len(teams) == 0 {
		return nil, errors.New("the access key's user has no kamusites teams")
	}
	if want != "" {
		for i := range teams {
			if teams[i].Slug == want || teams[i].ID == want {
				return &teams[i], nil
			}
		}
		return nil, fmt.Errorf("no team matches %q", want)
	}
	if org := keyOrgID(key); org != "" {
		for i := range teams {
			if teams[i].KamuidOrgID == org {
				return &teams[i], nil
			}
		}
	}
	if len(teams) == 1 {
		return &teams[0], nil
	}
	return nil, errors.New("multiple teams; pass --team <slug>")
}

// keyOrgID reads the scoped org's kamuid_org_id from the access key payload
// (no verification — the server verifies; we only need to route).
func keyOrgID(key string) string {
	parts := strings.Split(key, ".")
	if len(parts) != 3 {
		return ""
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ""
	}
	var p struct {
		Orgs []struct {
			KamuidOrgID string `json:"kamuid_org_id"`
		} `json:"orgs"`
	}
	if json.Unmarshal(raw, &p) != nil || len(p.Orgs) == 0 {
		return ""
	}
	return p.Orgs[0].KamuidOrgID
}

// watchBuild polls the latest build to a terminal state, printing step changes.
func watchBuild(ctx context.Context, client *kamusites.Client, site *kamusites.Site) error {
	io := iostreams.FromContext(ctx)
	deadline := time.Now().Add(25 * time.Minute)
	lastStep := ""
	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for build; check the dashboard")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(4 * time.Second):
		}
		bd, err := client.LatestBuild(ctx, site.ID)
		if err != nil {
			return err
		}
		if bd == nil {
			continue
		}
		if bd.CurrentStep != "" && bd.CurrentStep != lastStep {
			lastStep = bd.CurrentStep
			fmt.Fprintf(io.Out, "  %s...\n", bd.CurrentStep)
		}
		switch bd.Status {
		case "success":
			fmt.Fprintf(io.Out, "\nDone. Live at https://%s.kskamu.app\n", site.Slug)
			return nil
		case "failed", "cancelled":
			if bd.ErrorMessage != "" {
				return fmt.Errorf("build %s: %s", bd.Status, bd.ErrorMessage)
			}
			return fmt.Errorf("build %s", bd.Status)
		}
	}
}
