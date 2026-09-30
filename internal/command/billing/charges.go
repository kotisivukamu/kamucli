package billing

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/kotisivukamu/kamucli/internal/command"
	"github.com/kotisivukamu/kamucli/internal/iostreams"
	"github.com/kotisivukamu/kamucli/internal/render"
)

func newCharges() *cobra.Command {
	var f orgFlags
	var status string
	var limit int
	cmd := command.New("charges", "List the org's charges (immediate collections), newest first",
		"Charges are collected as they happen — a domain order, a one-off — as opposed to\n"+
			"invoices, which aggregate a billing period.",
		func(ctx context.Context, _ []string) error {
			ctx = ctxOrTodo(ctx)
			c, org, err := f.resolve(ctx)
			if err != nil {
				return err
			}
			charges, err := c.ListCharges(ctx, org, status, limit)
			if err != nil {
				return err
			}
			io := iostreams.FromContext(ctx)
			if f.asJSON {
				return render.JSON(io.Out, charges)
			}
			if len(charges) == 0 {
				fmt.Fprintln(io.Out, "No charges.")
				return nil
			}
			rows := make([][]string, 0, len(charges))
			for _, ch := range charges {
				rows = append(rows, []string{
					ch.ID, day(ch.CreatedAt), euros(ch.AmountCents, ch.Currency),
					ch.Status, ch.Product, dash(ch.Description),
				})
			}
			if err := render.Table(io.Out, []string{"ID", "DATE", "AMOUNT", "STATUS", "PRODUCT", "DESCRIPTION"}, rows); err != nil {
				return err
			}
			fmt.Fprintln(io.Out, "\n`kamu billing charge <id>` for the receipt link.")
			return nil
		})
	f.bind(cmd)
	fl := cmd.Flags()
	fl.StringVar(&status, "status", "", "Only this status (pending, paid, failed, refunded)")
	fl.IntVar(&limit, "limit", 50, "How many to list (max 200)")
	return cmd
}

func newCharge() *cobra.Command {
	var f orgFlags
	cmd := command.New("charge <id>", "Show one charge and its Stripe receipt link",
		"A charge that never reached Stripe (still pending, or a failed attempt) has no\n"+
			"receipt yet.",
		func(ctx context.Context, args []string) error {
			if len(args) != 1 {
				return errors.New("usage: kamu billing charge <id>")
			}
			ctx = ctxOrTodo(ctx)
			c, org, err := f.resolve(ctx)
			if err != nil {
				return err
			}
			ch, err := c.GetCharge(ctx, org, args[0])
			if err != nil {
				return err
			}
			io := iostreams.FromContext(ctx)
			if f.asJSON {
				return render.JSON(io.Out, ch)
			}
			return render.Table(io.Out, nil, [][]string{
				{"Charge", ch.ID},
				{"Status", ch.Status},
				{"Amount", euros(ch.AmountCents, ch.Currency)},
				{"Product", ch.Product},
				{"Description", dash(ch.Description)},
				{"Created", day(ch.CreatedAt)},
				{"Paid", dash(day(ch.PaidAt))},
				{"Receipt", dash(ch.ReceiptURL)},
			})
		})
	f.bind(cmd)
	return cmd
}
