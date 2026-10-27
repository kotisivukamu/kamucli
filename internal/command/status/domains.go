package status

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/kotisivukamu/kamucli/internal/client/kamustatus"
	"github.com/kotisivukamu/kamucli/internal/command"
	"github.com/kotisivukamu/kamucli/internal/iostreams"
	"github.com/kotisivukamu/kamucli/internal/render"
)

// A custom domain belongs to a status page and is set with
// `kamu status pages update --custom-domain`. This noun is the project-level
// view of those domains and their DNS verification; reading re-checks DNS.

func newDomains() *cobra.Command {
	cmd := command.New("domains", "List and verify custom status-page domains", "", nil)
	cmd.AddCommand(newDomainsList(), newDomainsVerify())
	return cmd
}

func newDomainsList() *cobra.Command {
	var asJSON bool
	cmd := command.New("list", "List a project's custom domains (re-checks DNS)", "", func(ctx context.Context, args []string) error {
		c, err := client()
		if err != nil {
			return err
		}
		domains, err := c.ListDomains(ctxOrTodo(ctx), args[0])
		if err != nil {
			return err
		}
		io := iostreams.FromContext(ctx)
		if asJSON {
			return render.JSON(io.Out, domains)
		}
		if len(domains) == 0 {
			fmt.Fprintln(io.Out, "No custom domains. Set one with `kamu status pages update <page-id> --custom-domain <host>`.")
			return nil
		}
		rows := make([][]string, 0, len(domains))
		pending := false
		for _, d := range domains {
			page := "-"
			if d.StatusPageSlug != nil {
				page = *d.StatusPageSlug
			}
			pending = pending || !d.Verified
			rows = append(rows, []string{d.ID, d.Domain, page, verifiedLabel(d.Verified)})
		}
		if err := render.Table(io.Out, []string{"ID", "DOMAIN", "PAGE", "DNS"}, rows); err != nil {
			return err
		}
		if pending {
			fmt.Fprintf(io.Out, "\nPoint each pending domain at %s with a CNAME record.\n", kamustatus.PagesCNAMETarget)
		}
		return nil
	})
	cmd.Args = cobra.ExactArgs(1)
	cmd.Use = "list <project-id>"
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output JSON")
	return cmd
}

func newDomainsVerify() *cobra.Command {
	var asJSON bool
	cmd := command.New("verify", "Re-check a custom domain's DNS now", "", func(ctx context.Context, args []string) error {
		c, err := client()
		if err != nil {
			return err
		}
		d, err := c.VerifyDomain(ctxOrTodo(ctx), args[0])
		if err != nil {
			return err
		}
		io := iostreams.FromContext(ctx)
		if asJSON {
			return render.JSON(io.Out, d)
		}
		fmt.Fprintf(io.Out, "%s: %s\n", d.Domain, verifiedLabel(d.Verified))
		if !d.Verified {
			fmt.Fprintf(io.Out, "Add a CNAME record: %s -> %s (DNS changes can take a while to propagate).\n", d.Domain, kamustatus.PagesCNAMETarget)
		}
		return nil
	})
	cmd.Args = cobra.ExactArgs(1)
	cmd.Use = "verify <domain-id>"
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output JSON")
	return cmd
}
