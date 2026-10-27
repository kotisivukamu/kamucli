package status

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/kotisivukamu/kamucli/internal/client/kamustatus"
	"github.com/kotisivukamu/kamucli/internal/command"
	"github.com/kotisivukamu/kamucli/internal/iostreams"
	"github.com/kotisivukamu/kamucli/internal/render"
)

// Status pages are the managed side of a project's public status: which
// monitors it shows, its branding and its custom domain. `kamu status page
// <slug>` (singular) is the unauthenticated read of the rendered result.

func newPages() *cobra.Command {
	cmd := command.New("pages", "Manage status pages (monitors shown, branding, custom domain)", "", nil)
	cmd.AddCommand(
		newPagesList(),
		newPagesCreate(),
		newPagesShow(),
		newPagesUpdate(),
		newPagesDelete(),
	)
	return cmd
}

func newPagesList() *cobra.Command {
	var asJSON bool
	cmd := command.New("list", "List status pages for a project", "", func(ctx context.Context, args []string) error {
		c, err := client()
		if err != nil {
			return err
		}
		pages, err := c.ListStatusPages(ctxOrTodo(ctx), args[0])
		if err != nil {
			return err
		}
		io := iostreams.FromContext(ctx)
		if asJSON {
			return render.JSON(io.Out, pages)
		}
		if len(pages) == 0 {
			fmt.Fprintln(io.Out, "No status pages.")
			return nil
		}
		rows := make([][]string, 0, len(pages))
		for _, p := range pages {
			rows = append(rows, []string{
				p.ID, p.Name, p.Slug, yesNo(p.Public),
				fmt.Sprintf("%d", p.MonitorCount), domainState(&p),
			})
		}
		return render.Table(io.Out, []string{"ID", "NAME", "SLUG", "PUBLIC", "MONITORS", "CUSTOM DOMAIN"}, rows)
	})
	cmd.Args = cobra.ExactArgs(1)
	cmd.Use = "list <project-id>"
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output JSON")
	return cmd
}

func newPagesCreate() *cobra.Command {
	var (
		name, slug, title, logoURL, customDomain string
		privacyURL, footerText                   string
		monitors                                 []string
		asJSON                                   bool
	)
	cmd := command.New("create", "Create a status page", "", func(ctx context.Context, args []string) error {
		input := map[string]any{"name": name, "slug": slug}
		if len(monitors) > 0 {
			input["monitorIds"] = monitors
		}
		if title != "" {
			input["title"] = title
		}
		if logoURL != "" {
			input["logoUrl"] = logoURL
		}
		if customDomain != "" {
			input["customDomain"] = customDomain
		}
		if privacyURL != "" {
			input["privacyUrl"] = privacyURL
		}
		if footerText != "" {
			input["footerText"] = footerText
		}
		c, err := client()
		if err != nil {
			return err
		}
		ctx = ctxOrTodo(ctx)
		created, err := c.CreateStatusPage(ctx, args[0], input)
		if err != nil {
			return err
		}
		// The create response carries no monitor list; re-read for the full view.
		p, err := c.GetStatusPage(ctx, created.ID)
		if err != nil {
			return err
		}
		io := iostreams.FromContext(ctx)
		if asJSON {
			return render.JSON(io.Out, p)
		}
		fmt.Fprintf(io.Out, "Created status page %s\n\n", p.Name)
		return printPage(io.Out, p)
	})
	cmd.Args = cobra.ExactArgs(1)
	cmd.Use = "create <project-id>"
	cmd.Flags().StringVar(&name, "name", "", "Page name")
	cmd.Flags().StringVar(&slug, "slug", "", "URL slug; the page is served at https://<slug>.kamustatus.com")
	cmd.Flags().StringArrayVar(&monitors, "monitor", nil, "Monitor id to show (repeatable; shown in the given order)")
	cmd.Flags().StringVar(&title, "title", "", "Heading shown on the page (defaults to the name)")
	cmd.Flags().StringVar(&logoURL, "logo-url", "", "Logo image URL")
	cmd.Flags().StringVar(&customDomain, "custom-domain", "", "Custom hostname, e.g. status.example.com (CNAME it to "+kamustatus.PagesCNAMETarget+")")
	cmd.Flags().StringVar(&privacyURL, "privacy-url", "", "Privacy policy URL linked in the page footer (http/https)")
	cmd.Flags().StringVar(&footerText, "footer-text", "", "Text shown in the page footer (max 500 characters)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output JSON")
	_ = cmd.MarkFlagRequired("name")
	_ = cmd.MarkFlagRequired("slug")
	return cmd
}

func newPagesShow() *cobra.Command {
	var asJSON bool
	cmd := command.New("show", "Show a status page", "", func(ctx context.Context, args []string) error {
		c, err := client()
		if err != nil {
			return err
		}
		p, err := c.GetStatusPage(ctxOrTodo(ctx), args[0])
		if err != nil {
			return err
		}
		io := iostreams.FromContext(ctx)
		if asJSON {
			return render.JSON(io.Out, p)
		}
		return printPage(io.Out, p)
	})
	cmd.Args = cobra.ExactArgs(1)
	cmd.Use = "show <page-id>"
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output JSON")
	return cmd
}

func newPagesUpdate() *cobra.Command {
	var (
		name, title, logoURL, cssFile, customDomain string
		privacyURL, footerText                      string
		public, asJSON                              bool
		monitors                                    []string
	)
	// Captured so the runner can tell "not passed" from "passed empty": PATCH
	// semantics make them different requests (--custom-domain "" clears, as do
	// --privacy-url "" and --footer-text "").
	var cmd *cobra.Command
	cmd = command.New("update", "Change a status page", "", func(ctx context.Context, args []string) error {
		flags := cmd.Flags()
		updates := map[string]any{}
		for flag, key := range map[string]string{
			"name": "name", "title": "title", "logo-url": "logoUrl", "custom-domain": "customDomain",
			"privacy-url": "privacyUrl", "footer-text": "footerText",
		} {
			if flags.Changed(flag) {
				v, _ := flags.GetString(flag)
				updates[key] = v
			}
		}
		if flags.Changed("custom-css-file") {
			css, err := os.ReadFile(cssFile)
			if err != nil {
				return err
			}
			updates["customCss"] = string(css)
		}
		if flags.Changed("public") {
			updates["public"] = public
		}
		if flags.Changed("monitor") {
			if monitors == nil {
				monitors = []string{}
			}
			updates["monitorIds"] = monitors
		}
		if len(updates) == 0 {
			return fmt.Errorf("nothing to change: pass at least one flag (see --help)")
		}
		c, err := client()
		if err != nil {
			return err
		}
		ctx = ctxOrTodo(ctx)
		if _, err := c.UpdateStatusPage(ctx, args[0], updates); err != nil {
			return err
		}
		// The PATCH response omits monitors (and the verified state unless the
		// domain changed); re-read so the output always shows the whole page.
		p, err := c.GetStatusPage(ctx, args[0])
		if err != nil {
			return err
		}
		io := iostreams.FromContext(ctx)
		if asJSON {
			return render.JSON(io.Out, p)
		}
		fmt.Fprintf(io.Out, "Updated status page %s\n\n", p.Name)
		return printPage(io.Out, p)
	})
	cmd.Args = cobra.ExactArgs(1)
	cmd.Use = "update <page-id>"
	cmd.Aliases = []string{"set"}
	cmd.Flags().StringVar(&name, "name", "", "Page name")
	cmd.Flags().StringVar(&title, "title", "", "Heading shown on the page")
	cmd.Flags().StringVar(&logoURL, "logo-url", "", "Logo image URL")
	cmd.Flags().StringVar(&cssFile, "custom-css-file", "", "Read custom CSS for the page from this file")
	cmd.Flags().StringVar(&customDomain, "custom-domain", "", `Custom hostname (CNAME it to `+kamustatus.PagesCNAMETarget+`); "" removes it`)
	cmd.Flags().StringVar(&privacyURL, "privacy-url", "", `Privacy policy URL linked in the page footer (http/https); "" removes it`)
	cmd.Flags().StringVar(&footerText, "footer-text", "", `Text shown in the page footer (max 500 characters); "" removes it`)
	cmd.Flags().BoolVar(&public, "public", false, "Whether the page is publicly visible (--public=false hides it)")
	cmd.Flags().StringArrayVar(&monitors, "monitor", nil, "Monitor id to show (repeatable); replaces the whole set, in the given order")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output JSON")
	return cmd
}

func newPagesDelete() *cobra.Command {
	cmd := command.New("delete", "Delete a status page (and its custom domain)", "", func(ctx context.Context, args []string) error {
		c, err := client()
		if err != nil {
			return err
		}
		if err := c.DeleteStatusPage(ctxOrTodo(ctx), args[0]); err != nil {
			return err
		}
		fmt.Fprintln(iostreams.FromContext(ctx).Out, "Status page deleted.")
		return nil
	})
	cmd.Args = cobra.ExactArgs(1)
	cmd.Use = "delete <page-id>"
	return cmd
}

func printPage(w io.Writer, p *kamustatus.StatusPage) error {
	rows := [][]string{
		{"id", p.ID},
		{"name", p.Name},
		{"slug", p.Slug},
		{"public", yesNo(p.Public)},
		{"url", p.PlatformURL()},
	}
	if p.Title != nil && *p.Title != "" {
		rows = append(rows, []string{"title", *p.Title})
	}
	if p.LogoURL != nil && *p.LogoURL != "" {
		rows = append(rows, []string{"logo", *p.LogoURL})
	}
	rows = append(rows, []string{"custom domain", domainState(p)})
	if p.PrivacyURL != nil && *p.PrivacyURL != "" {
		rows = append(rows, []string{"privacy url", *p.PrivacyURL})
	}
	if p.FooterText != nil && *p.FooterText != "" {
		rows = append(rows, []string{"footer text", *p.FooterText})
	}
	if err := render.Table(w, nil, rows); err != nil {
		return err
	}
	if p.CustomDomain != nil && (p.CustomDomainVerified == nil || !*p.CustomDomainVerified) {
		fmt.Fprintf(w, "\nAdd a CNAME record: %s -> %s, then run `kamu status domains verify <domain-id>`.\n",
			*p.CustomDomain, kamustatus.PagesCNAMETarget)
	}
	if len(p.Monitors) == 0 {
		fmt.Fprintln(w, "\nNo monitors on this page.")
		return nil
	}
	fmt.Fprintln(w, "\nMonitors:")
	mrows := make([][]string, 0, len(p.Monitors))
	for _, m := range p.Monitors {
		mrows = append(mrows, []string{m.ID, m.Name, m.Type, m.Target})
	}
	return render.Table(w, []string{"ID", "NAME", "TYPE", "TARGET"}, mrows)
}

// domainState renders a page's custom domain with its DNS state.
func domainState(p *kamustatus.StatusPage) string {
	if p.CustomDomain == nil || *p.CustomDomain == "" {
		return "-"
	}
	return *p.CustomDomain + " (" + verifiedLabel(p.CustomDomainVerified != nil && *p.CustomDomainVerified) + ")"
}

func verifiedLabel(v bool) string {
	if v {
		return "Verified"
	}
	return "Pending DNS"
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}
