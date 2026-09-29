package status

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/kotisivukamu/kamucli/internal/client/kamustatus"
	"github.com/kotisivukamu/kamucli/internal/command"
	"github.com/kotisivukamu/kamucli/internal/iostreams"
	"github.com/kotisivukamu/kamucli/internal/render"
)

// Properties are the origin-level unit kamustatus hangs error (and later
// security/performance) data off: a project has properties, a property has
// monitors. Monitors link to one with `kamu status monitors set --property`.

func newProperties() *cobra.Command {
	cmd := command.New("properties", "Manage properties (site origins)", "", nil)
	cmd.Aliases = []string{"props", "property"}
	cmd.AddCommand(
		newPropertiesList(),
		newPropertiesAdd(),
		newPropertiesShow(),
		newPropertiesSet(),
		newPropertiesRotateKey(),
		newPropertiesDelete(),
	)
	return cmd
}

// printIngestSnippet renders the browser install snippet for a property. The
// ingest key is public by design (it authorizes writes to this one property and
// reads nothing), so printing it to a terminal is safe — it is meant to be
// pasted into page source. Shape per packages/telemetry/README.md.
func printIngestSnippet(w io.Writer, p *kamustatus.Property) {
	if p.IngestKey == "" || p.IngestURL == "" {
		return
	}
	fmt.Fprintf(w, "\nAdd this to the site at %s:\n", p.Origin)
	fmt.Fprintf(w, "  <script src=%q data-key=%q defer></script>\n", p.IngestURL+"/t.js", p.IngestKey)
}

func newPropertiesList() *cobra.Command {
	var asJSON bool
	cmd := command.New("list", "List properties for a project", "", func(ctx context.Context, args []string) error {
		c, err := client()
		if err != nil {
			return err
		}
		props, err := c.ListProperties(ctxOrTodo(ctx), args[0])
		if err != nil {
			return err
		}
		io := iostreams.FromContext(ctx)
		if asJSON {
			return render.JSON(io.Out, props)
		}
		if len(props) == 0 {
			fmt.Fprintln(io.Out, "No properties.")
			return nil
		}
		rows := make([][]string, 0, len(props))
		for _, p := range props {
			rows = append(rows, []string{
				p.ID, p.Name, p.Origin,
				fmt.Sprintf("%d", p.MonitorCount),
				fmt.Sprintf("%d", p.OpenErrorGroups),
			})
		}
		return render.Table(io.Out, []string{"ID", "NAME", "ORIGIN", "MONITORS", "OPEN ERRORS"}, rows)
	})
	cmd.Args = cobra.ExactArgs(1)
	cmd.Use = "list <project-id>"
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output JSON")
	return cmd
}

func newPropertiesAdd() *cobra.Command {
	var name, origin string
	cmd := command.New("add", "Add a property to a project", "", func(ctx context.Context, args []string) error {
		c, err := client()
		if err != nil {
			return err
		}
		p, err := c.CreateProperty(ctxOrTodo(ctx), args[0], name, origin)
		if err != nil {
			return err
		}
		io := iostreams.FromContext(ctx)
		fmt.Fprintf(io.Out, "Created property %s\n", p.Name)
		fmt.Fprintf(io.Out, "  id:     %s\n", p.ID)
		fmt.Fprintf(io.Out, "  origin: %s\n", p.Origin)
		printIngestSnippet(io.Out, p)
		return nil
	})
	cmd.Args = cobra.ExactArgs(1)
	cmd.Use = "add <project-id>"
	cmd.Flags().StringVar(&name, "name", "", "Property name")
	cmd.Flags().StringVar(&origin, "origin", "", "Site origin, e.g. https://example.com")
	_ = cmd.MarkFlagRequired("name")
	_ = cmd.MarkFlagRequired("origin")
	return cmd
}

func newPropertiesShow() *cobra.Command {
	var asJSON bool
	cmd := command.New("show", "Show property details", "", func(ctx context.Context, args []string) error {
		c, err := client()
		if err != nil {
			return err
		}
		data, err := c.GetProperty(ctxOrTodo(ctx), args[0])
		if err != nil {
			return err
		}
		io := iostreams.FromContext(ctx)
		if asJSON {
			_, err := io.Out.Write(data)
			return err
		}
		var p struct {
			kamustatus.Property
			Monitors         []kamustatus.Monitor `json:"monitors"`
			ErrorGroupCounts map[string]int       `json:"errorGroupCounts"`
		}
		if err := json.Unmarshal(data, &p); err != nil {
			return err
		}
		rows := [][]string{
			{"id", p.ID},
			{"name", p.Name},
			{"origin", p.Origin},
			{"monitors", fmt.Sprintf("%d", len(p.Monitors))},
			{"open errors", fmt.Sprintf("%d", p.ErrorGroupCounts["open"])},
			{"resolved errors", fmt.Sprintf("%d", p.ErrorGroupCounts["resolved"])},
			{"ingest key", p.IngestKey},
			{"ingest url", p.IngestURL},
		}
		if err := render.Table(io.Out, nil, rows); err != nil {
			return err
		}
		if len(p.Monitors) > 0 {
			fmt.Fprintln(io.Out)
			mrows := make([][]string, 0, len(p.Monitors))
			for _, m := range p.Monitors {
				state := "ON"
				if !m.Enabled {
					state = "OFF"
				}
				mrows = append(mrows, []string{m.ID, m.Name, m.Type, m.Target, state})
			}
			return render.Table(io.Out, []string{"MONITOR", "NAME", "TYPE", "TARGET", "STATE"}, mrows)
		}
		return nil
	})
	cmd.Args = cobra.ExactArgs(1)
	cmd.Use = "show <property-id>"
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output JSON")
	return cmd
}

func newPropertiesSet() *cobra.Command {
	var name, origin string
	// cmd is captured by the runner so it can ask which flags were actually
	// given: PATCH semantics mean "unset" and "set to empty" are different
	// requests, which a zero-value check cannot distinguish.
	var cmd *cobra.Command
	cmd = command.New("set", "Change a property's name or origin", "", func(ctx context.Context, args []string) error {
		updates := map[string]any{}
		if cmd.Flags().Changed("name") {
			updates["name"] = name
		}
		if cmd.Flags().Changed("origin") {
			updates["origin"] = origin
		}
		if len(updates) == 0 {
			return fmt.Errorf("nothing to change: pass --name and/or --origin")
		}
		c, err := client()
		if err != nil {
			return err
		}
		p, err := c.UpdateProperty(ctxOrTodo(ctx), args[0], updates)
		if err != nil {
			return err
		}
		io := iostreams.FromContext(ctx)
		fmt.Fprintf(io.Out, "Updated property %s\n", p.Name)
		fmt.Fprintf(io.Out, "  origin: %s\n", p.Origin)
		return nil
	})
	cmd.Args = cobra.ExactArgs(1)
	cmd.Use = "set <property-id>"
	cmd.Flags().StringVar(&name, "name", "", "New property name")
	cmd.Flags().StringVar(&origin, "origin", "", "New site origin, e.g. https://example.com")
	return cmd
}

func newPropertiesRotateKey() *cobra.Command {
	var yes bool
	cmd := command.New("rotate-key", "Issue a new ingest key, invalidating the old one", "", func(ctx context.Context, args []string) error {
		io := iostreams.FromContext(ctx)
		if !yes {
			return fmt.Errorf("rotating invalidates the current key immediately and every page using the old snippet stops reporting until it is updated. Re-run with --yes to confirm")
		}
		c, err := client()
		if err != nil {
			return err
		}
		p, err := c.RotateIngestKey(ctxOrTodo(ctx), args[0])
		if err != nil {
			return err
		}
		fmt.Fprintf(io.Out, "Rotated the ingest key for %s\n", p.Name)
		printIngestSnippet(io.Out, p)
		return nil
	})
	cmd.Args = cobra.ExactArgs(1)
	cmd.Use = "rotate-key <property-id>"
	cmd.Flags().BoolVar(&yes, "yes", false, "Confirm the rotation")
	return cmd
}

func newPropertiesDelete() *cobra.Command {
	cmd := command.New("delete", "Delete a property", "", func(ctx context.Context, args []string) error {
		c, err := client()
		if err != nil {
			return err
		}
		if err := c.DeleteProperty(ctxOrTodo(ctx), args[0]); err != nil {
			return err
		}
		fmt.Fprintln(iostreams.FromContext(ctx).Out, "Property deleted.")
		return nil
	})
	cmd.Args = cobra.ExactArgs(1)
	cmd.Use = "delete <property-id>"
	return cmd
}
