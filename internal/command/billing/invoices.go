package billing

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/kotisivukamu/kamucli/internal/command"
	"github.com/kotisivukamu/kamucli/internal/iostreams"
	"github.com/kotisivukamu/kamucli/internal/render"
)

func newInvoices() *cobra.Command {
	var f orgFlags
	var status string
	var limit int
	cmd := command.New("invoices", "List the org's invoices, newest first", "",
		func(ctx context.Context, _ []string) error {
			ctx = ctxOrTodo(ctx)
			c, org, err := f.resolve(ctx)
			if err != nil {
				return err
			}
			invoices, err := c.ListInvoices(ctx, org, status, limit)
			if err != nil {
				return err
			}
			io := iostreams.FromContext(ctx)
			if f.asJSON {
				return render.JSON(io.Out, invoices)
			}
			if len(invoices) == 0 {
				fmt.Fprintln(io.Out, "No invoices.")
				return nil
			}
			rows := make([][]string, 0, len(invoices))
			for _, inv := range invoices {
				rows = append(rows, []string{
					inv.ID,
					day(firstNonEmpty(inv.IssuedAt, inv.CreatedAt)),
					euros(inv.TotalCents, inv.Currency),
					inv.Status,
					inv.Rail,
					period(inv.PeriodStart, inv.PeriodEnd),
				})
			}
			if err := render.Table(io.Out, []string{"ID", "DATE", "TOTAL", "STATUS", "RAIL", "PERIOD"}, rows); err != nil {
				return err
			}
			fmt.Fprintln(io.Out, "\n`kamu billing invoice <id>` for the lines and the PDF.")
			return nil
		})
	f.bind(cmd)
	fl := cmd.Flags()
	fl.StringVar(&status, "status", "", "Only this status (draft, issued, paid, void, uncollectible)")
	fl.IntVar(&limit, "limit", 50, "How many to list (max 200)")
	return cmd
}

func newInvoice() *cobra.Command {
	var f orgFlags
	var out string
	cmd := command.New("invoice <id>", "Show one invoice with its lines, and download its PDF",
		"Shows the invoice, the line items it aggregates, and the links to Stripe's PDF\n"+
			"and hosted page. -o writes the PDF to a file.\n\n"+
			"An invoice on the e-invoice rail has no Stripe document — Procountor issues\n"+
			"that paperwork, and procountor_ref is where to look for it.",
		func(ctx context.Context, args []string) error {
			if len(args) != 1 {
				return errors.New("usage: kamu billing invoice <id> [-o file.pdf]")
			}
			ctx = ctxOrTodo(ctx)
			c, org, err := f.resolve(ctx)
			if err != nil {
				return err
			}
			inv, err := c.GetInvoice(ctx, org, args[0])
			if err != nil {
				return err
			}
			io := iostreams.FromContext(ctx)

			if out != "" {
				if inv.PDFURL == "" {
					return fmt.Errorf("invoice %s has no PDF (rail %q, stripe invoice %q)",
						inv.ID, inv.Rail, dash(inv.StripeInvoiceID))
				}
				pdf, err := c.Download(ctx, inv.PDFURL)
				if err != nil {
					return err
				}
				path := out
				if fi, err := os.Stat(path); err == nil && fi.IsDir() {
					path = filepath.Join(path, "invoice-"+inv.ID+".pdf")
				}
				if err := os.WriteFile(path, pdf, 0o600); err != nil {
					return err
				}
				fmt.Fprintf(io.Out, "Wrote %s (%d bytes)\n", path, len(pdf))
				return nil
			}

			if f.asJSON {
				return render.JSON(io.Out, inv)
			}

			if err := render.Table(io.Out, nil, [][]string{
				{"Invoice", inv.ID},
				{"Status", inv.Status},
				{"Rail", inv.Rail},
				{"Total", euros(inv.TotalCents, inv.Currency)},
				{"Period", period(inv.PeriodStart, inv.PeriodEnd)},
				{"Issued", dash(day(inv.IssuedAt))},
				{"Paid", dash(day(inv.PaidAt))},
				{"PDF", dash(inv.PDFURL)},
				{"Hosted", dash(inv.HostedURL)},
				{"Procountor", dash(inv.ProcountorRef)},
			}); err != nil {
				return err
			}

			if len(inv.LineItems) == 0 {
				return nil
			}
			fmt.Fprintln(io.Out, "\nLines:")
			rows := make([][]string, 0, len(inv.LineItems))
			for _, li := range inv.LineItems {
				rows = append(rows, []string{
					day(li.OccurredAt), li.Product, li.Description, li.Qty,
					euros(li.AmountCents, li.Currency), li.ChargeTiming,
				})
			}
			return render.Table(io.Out, []string{"DATE", "PRODUCT", "DESCRIPTION", "QTY", "AMOUNT", "TIMING"}, rows)
		})
	f.bind(cmd)
	cmd.Flags().StringVarP(&out, "out", "o", "", "Download the PDF to this file (or directory)")
	return cmd
}

// period renders the billing period, or "-" for an ad-hoc invoice (both dates
// NULL, which is what the schema uses for one that closes no period).
func period(start, end string) string {
	if start == "" && end == "" {
		return "-"
	}
	return day(start) + " - " + day(end)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
