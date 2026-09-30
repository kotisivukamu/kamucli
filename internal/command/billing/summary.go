package billing

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/kotisivukamu/kamucli/internal/command"
	"github.com/kotisivukamu/kamucli/internal/iostreams"
	"github.com/kotisivukamu/kamucli/internal/render"
)

func newSummary() *cobra.Command {
	var f orgFlags
	cmd := command.New("summary", "Show the billing rail, card on file and subscriptions", "",
		func(ctx context.Context, _ []string) error {
			ctx = ctxOrTodo(ctx)
			c, org, err := f.resolve(ctx)
			if err != nil {
				return err
			}
			s, err := c.Summary(ctx, org)
			if err != nil {
				return err
			}
			io := iostreams.FromContext(ctx)
			if f.asJSON {
				return render.JSON(io.Out, s)
			}

			card := "none"
			if s.Card != nil {
				card = fmt.Sprintf("%s ****%s (%02d/%d)", s.Card.Brand, s.Card.Last4, s.Card.ExpMonth, s.Card.ExpYear)
			}
			rows := [][]string{
				// 'none' means billing onboarding never started, which reads
				// better spelled out than as a bare enum value.
				{"Rail", railLabel(s.BillingMode)},
				{"Card", card},
			}
			for _, sub := range s.Subscriptions {
				rows = append(rows, []string{
					"Subscription",
					fmt.Sprintf("%s %s (%s, renews %s)", sub.Product, sub.PlanKey, sub.Status, day(sub.CurrentPeriodEnd)),
				})
			}
			if err := render.Table(io.Out, nil, rows); err != nil {
				return err
			}
			if len(s.Charges) > 0 {
				fmt.Fprintf(io.Out, "\nLatest charges (%d) — `kamu billing charges` for the full list:\n", len(s.Charges))
				crows := make([][]string, 0, len(s.Charges))
				for _, ch := range s.Charges {
					crows = append(crows, []string{
						day(ch.CreatedAt), euros(ch.AmountCents, ch.Currency), ch.Status, ch.Product, dash(ch.Description),
					})
				}
				return render.Table(io.Out, []string{"DATE", "AMOUNT", "STATUS", "PRODUCT", "DESCRIPTION"}, crows)
			}
			return nil
		})
	f.bind(cmd)
	return cmd
}

func railLabel(mode string) string {
	switch mode {
	case "card":
		return "card (autopay)"
	case "einvoice":
		return "e-invoice (Procountor)"
	case "none":
		return "none (billing not set up)"
	default:
		return mode
	}
}
