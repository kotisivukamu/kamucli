// Package dns implements `kamu dns` — manage kamudns zones and records from the
// CLI. Auth is a kamuhub access key (a scoped, signed platform context); export
// KAMU_ACCESS_KEY or pass --key. Domains can be named or referenced by id; a
// name is resolved against the caller's managed zones.
package dns

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/kotisivukamu/kamucli/internal/client/kamudns"
	"github.com/kotisivukamu/kamucli/internal/command"
	"github.com/kotisivukamu/kamucli/internal/iostreams"
)

const (
	envKey = "KAMU_ACCESS_KEY"
	envURL = "KAMU_KAMUDNS_URL"
)

func New() *cobra.Command {
	cmd := command.New("dns", "Manage kamudns zones and records", "", nil)
	cmd.AddCommand(
		newZones(),
		newGet(),
		newSearch(),
		newRecords(),
	)
	return cmd
}

// client resolves the access key (flag then env) and builds a kamudns client.
func client(key string) (*kamudns.Client, error) {
	if key == "" {
		key = os.Getenv(envKey)
	}
	if key == "" {
		return nil, errors.New("no access key. Create one in the dashboard (Manage -> Access keys) and pass it:\n\n    export " + envKey + "=...\n\nor --key <token>")
	}
	return kamudns.New(os.Getenv(envURL), key), nil
}

// resolveDomain maps a domain name OR id to a managed zone's id. A bare id
// (matching a zone id) is returned as-is; otherwise the arg is matched against
// zone domains so users can pass "example.com" instead of a UUID.
func resolveDomain(ctx context.Context, c *kamudns.Client, arg string) (string, error) {
	zones, err := c.Zones(ctx)
	if err != nil {
		return "", err
	}
	want := strings.ToLower(strings.TrimSpace(arg))
	for _, z := range zones {
		if z.ID == arg || strings.ToLower(z.Domain) == want {
			return z.ID, nil
		}
	}
	return "", fmt.Errorf("no managed domain matches %q (see `kamu dns zones`)", arg)
}

func newZones() *cobra.Command {
	var key string
	cmd := command.New("zones", "List managed domains (zones)", "", func(ctx context.Context, _ []string) error {
		io := iostreams.FromContext(ctx)
		c, err := client(key)
		if err != nil {
			return err
		}
		zones, err := c.Zones(ctx)
		if err != nil {
			return err
		}
		if len(zones) == 0 {
			fmt.Fprintln(io.Out, "No managed domains.")
			return nil
		}
		w := tabwriter.NewWriter(io.Out, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "DOMAIN\tSTATUS\tEXPIRES\tAUTORENEW\tID")
		for _, z := range zones {
			fmt.Fprintf(w, "%s\t%s\t%s\t%t\t%s\n", z.Domain, z.Status, shortDate(z.ExpiresAt), z.AutoRenew, z.ID)
		}
		return w.Flush()
	})
	cmd.Flags().StringVar(&key, "key", "", "kamuhub access key (or "+envKey+")")
	return cmd
}

func newGet() *cobra.Command {
	var key string
	cmd := command.New("get <domain>", "Show a domain's details", "", func(ctx context.Context, args []string) error {
		if len(args) != 1 {
			return errors.New("usage: kamu dns get <domain>")
		}
		io := iostreams.FromContext(ctx)
		c, err := client(key)
		if err != nil {
			return err
		}
		id, err := resolveDomain(ctx, c, args[0])
		if err != nil {
			return err
		}
		raw, err := c.Domain(ctx, id)
		if err != nil {
			return err
		}
		var pretty any
		if json.Unmarshal(raw, &pretty) != nil {
			fmt.Fprintln(io.Out, string(raw))
			return nil
		}
		b, _ := json.MarshalIndent(pretty, "", "  ")
		fmt.Fprintln(io.Out, string(b))
		return nil
	})
	cmd.Flags().StringVar(&key, "key", "", "kamuhub access key (or "+envKey+")")
	return cmd
}

func newSearch() *cobra.Command {
	var (
		key   string
		limit int
	)
	cmd := command.New("search <query>", "Check domain availability across TLDs", "", func(ctx context.Context, args []string) error {
		if len(args) != 1 {
			return errors.New("usage: kamu dns search <query>")
		}
		io := iostreams.FromContext(ctx)
		c, err := client(key)
		if err != nil {
			return err
		}
		results, err := c.Search(ctx, args[0], limit)
		if err != nil {
			return err
		}
		if len(results) == 0 {
			fmt.Fprintln(io.Out, "No results.")
			return nil
		}
		w := tabwriter.NewWriter(io.Out, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "DOMAIN\tAVAILABLE\tREGISTER\tPREMIUM")
		for _, r := range results {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", r.Domain, yesno(r.Available), price(r), prem(r.IsPremium))
		}
		return w.Flush()
	})
	f := cmd.Flags()
	f.StringVar(&key, "key", "", "kamuhub access key (or "+envKey+")")
	f.IntVar(&limit, "limit", 0, "max results (server default 16)")
	return cmd
}

func newRecords() *cobra.Command {
	cmd := command.New("records", "List, add, and delete DNS records", "", nil)
	cmd.AddCommand(newRecordsList(), newRecordsAdd(), newRecordsDelete())
	return cmd
}

func newRecordsList() *cobra.Command {
	var key string
	cmd := command.New("list <domain>", "List a domain's DNS records", "", func(ctx context.Context, args []string) error {
		if len(args) != 1 {
			return errors.New("usage: kamu dns records list <domain>")
		}
		io := iostreams.FromContext(ctx)
		c, err := client(key)
		if err != nil {
			return err
		}
		id, err := resolveDomain(ctx, c, args[0])
		if err != nil {
			return err
		}
		records, err := c.Records(ctx, id)
		if err != nil {
			return err
		}
		if len(records) == 0 {
			fmt.Fprintln(io.Out, "No DNS records.")
			return nil
		}
		w := tabwriter.NewWriter(io.Out, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "TYPE\tNAME\tCONTENT\tTTL\tID")
		for _, r := range records {
			fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%s\n", r.Type, r.Name, r.Content, r.TTL, r.ID)
		}
		return w.Flush()
	})
	cmd.Flags().StringVar(&key, "key", "", "kamuhub access key (or "+envKey+")")
	return cmd
}

func newRecordsAdd() *cobra.Command {
	var (
		key, recType, name, content string
		ttl                         int
	)
	cmd := command.New("add <domain>", "Add a DNS record", "Add a DNS record. To change a record, delete it and add the new one (the API has no in-place update).", func(ctx context.Context, args []string) error {
		if len(args) != 1 {
			return errors.New("usage: kamu dns records add <domain> --type A --content 1.2.3.4 [--name @] [--ttl 3600]")
		}
		if strings.TrimSpace(recType) == "" || strings.TrimSpace(content) == "" {
			return errors.New("--type and --content are required")
		}
		io := iostreams.FromContext(ctx)
		c, err := client(key)
		if err != nil {
			return err
		}
		id, err := resolveDomain(ctx, c, args[0])
		if err != nil {
			return err
		}
		rec, err := c.AddRecord(ctx, id, kamudns.RecordInput{
			Type:    strings.ToUpper(strings.TrimSpace(recType)),
			Name:    strings.TrimSpace(name),
			Content: strings.TrimSpace(content),
			TTL:     ttl,
		})
		if err != nil {
			return err
		}
		fmt.Fprintf(io.Out, "Added %s %s -> %s (id %s)\n", rec.Type, rec.Name, rec.Content, rec.ID)
		return nil
	})
	f := cmd.Flags()
	f.StringVar(&key, "key", "", "kamuhub access key (or "+envKey+")")
	f.StringVar(&recType, "type", "", "record type (A, AAAA, CNAME, MX, TXT, SRV, CAA, ALIAS)")
	f.StringVar(&name, "name", "", "record name (defaults to @)")
	f.StringVar(&content, "content", "", "record content/value")
	f.IntVar(&ttl, "ttl", 0, "TTL seconds (default 3600)")
	return cmd
}

func newRecordsDelete() *cobra.Command {
	var key string
	cmd := command.New("delete <domain> <record-id>", "Delete a DNS record", "", func(ctx context.Context, args []string) error {
		if len(args) != 2 {
			return errors.New("usage: kamu dns records delete <domain> <record-id>")
		}
		io := iostreams.FromContext(ctx)
		c, err := client(key)
		if err != nil {
			return err
		}
		id, err := resolveDomain(ctx, c, args[0])
		if err != nil {
			return err
		}
		if err := c.DeleteRecord(ctx, id, args[1]); err != nil {
			return err
		}
		fmt.Fprintf(io.Out, "Deleted record %s\n", args[1])
		return nil
	})
	cmd.Flags().StringVar(&key, "key", "", "kamuhub access key (or "+envKey+")")
	return cmd
}

func shortDate(s string) string {
	if len(s) >= 10 {
		return s[:10]
	}
	if s == "" {
		return "-"
	}
	return s
}

func yesno(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func prem(b bool) string {
	if b {
		return "yes"
	}
	return ""
}

func price(r kamudns.SearchItem) string {
	if r.Prices == nil || r.Prices.Register == nil {
		return "-"
	}
	return fmt.Sprintf("%.2f", *r.Prices.Register)
}
