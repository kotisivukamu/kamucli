package status

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/kotisivukamu/kamucli/internal/command"
	"github.com/kotisivukamu/kamucli/internal/iostreams"
	"github.com/kotisivukamu/kamucli/internal/render"
)

func newAlerts() *cobra.Command {
	cmd := command.New("alerts", "Manage alert rules", "", nil)
	cmd.AddCommand(newAlertsList(), newAlertsAdd(), newAlertsDelete())
	return cmd
}

func newAlertsList() *cobra.Command {
	var asJSON bool
	cmd := command.New("list", "List alerts for a monitor", "", func(ctx context.Context, args []string) error {
		c, err := client()
		if err != nil {
			return err
		}
		data, err := c.ListAlerts(ctxOrTodo(ctx), args[0])
		if err != nil {
			return err
		}
		io := iostreams.FromContext(ctx)
		if asJSON {
			_, err := io.Out.Write(data)
			return err
		}
		var alerts []struct {
			ID                  string `json:"id"`
			Type                string `json:"type"`
			Target              string `json:"target"`
			ConsecutiveFailures int    `json:"consecutive_failures"`
		}
		if err := json.Unmarshal(data, &alerts); err != nil {
			return err
		}
		if len(alerts) == 0 {
			fmt.Fprintln(io.Out, "No alerts.")
			return nil
		}
		rows := make([][]string, 0, len(alerts))
		for _, a := range alerts {
			rows = append(rows, []string{a.ID, a.Type, a.Target, fmt.Sprintf("%d", a.ConsecutiveFailures)})
		}
		return render.Table(io.Out, []string{"ID", "TYPE", "TARGET", "AFTER N FAILS"}, rows)
	})
	cmd.Args = cobra.ExactArgs(1)
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output JSON")
	return cmd
}

func newAlertsAdd() *cobra.Command {
	var typ, target string
	var consecutive int
	cmd := command.New("add", "Add an alert rule", "", func(ctx context.Context, args []string) error {
		input := map[string]any{"type": typ, "target": target}
		if consecutive > 0 {
			input["consecutiveFailures"] = consecutive
		}
		c, err := client()
		if err != nil {
			return err
		}
		data, err := c.CreateAlert(ctxOrTodo(ctx), args[0], input)
		if err != nil {
			return err
		}
		var a struct {
			ID string `json:"id"`
		}
		_ = json.Unmarshal(data, &a)
		fmt.Fprintf(iostreams.FromContext(ctx).Out, "Created alert %s\n", a.ID)
		return nil
	})
	cmd.Args = cobra.ExactArgs(1)
	cmd.Flags().StringVar(&typ, "type", "email", "Alert type: email, slack, webhook")
	cmd.Flags().StringVar(&target, "target", "", "Target (email address, webhook URL, ...)")
	cmd.Flags().IntVar(&consecutive, "consecutive", 0, "Trigger after N consecutive failures")
	_ = cmd.MarkFlagRequired("target")
	return cmd
}

func newAlertsDelete() *cobra.Command {
	cmd := command.New("delete", "Delete an alert", "", func(ctx context.Context, args []string) error {
		c, err := client()
		if err != nil {
			return err
		}
		if err := c.DeleteAlert(ctxOrTodo(ctx), args[0]); err != nil {
			return err
		}
		fmt.Fprintln(iostreams.FromContext(ctx).Out, "Alert deleted.")
		return nil
	})
	cmd.Args = cobra.ExactArgs(1)
	return cmd
}
