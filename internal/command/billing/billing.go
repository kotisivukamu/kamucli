// Package billing implements `kamu billing` — the org's own billing paperwork:
// what rail it is on, which invoices and charges exist, and the document for
// each one. Auth is a kamuhub access key (kamuhub owns billing, so this talks
// to the front door itself, not to a product API).
//
// It exists because there was no way to get a receipt out of the platform. A
// charge showed up on the card statement, Stripe mailed nothing (receipt_email
// is not set), and nothing in the dashboard linked a downloadable document, so
// an org reconciling its books had "no receipt" rows it could not resolve.
// `kamu billing invoice <id> -o file.pdf` is the answer to that.
package billing

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/kotisivukamu/kamucli/internal/client/billing"
	"github.com/kotisivukamu/kamucli/internal/command"
	"github.com/kotisivukamu/kamucli/internal/config"
)

const envKey = "KAMU_ACCESS_KEY"

func New() *cobra.Command {
	cmd := command.New("billing", "Show billing state, invoices, charges and their receipts",
		"Your organization's billing on kamuhub: the rail and card on file, the invoices\n"+
			"and charges it has accrued, and the Stripe document for each.\n\n"+
			"Billing is per organization. The org comes from your access key; pass --org\n"+
			"(slug or kamuid org id) when the key covers more than one.", nil)
	cmd.AddCommand(
		newSummary(),
		newInvoices(),
		newInvoice(),
		newCharges(),
		newCharge(),
	)
	return cmd
}

// orgFlags are the three flags every billing subcommand takes.
type orgFlags struct {
	key    string
	org    string
	asJSON bool
}

func (f *orgFlags) bind(cmd *cobra.Command) {
	fl := cmd.Flags()
	fl.StringVar(&f.key, "key", "", "kamuhub access key (or "+envKey+")")
	fl.StringVar(&f.org, "org", "", "org slug or kamuid org id (defaults to the key's org)")
	fl.BoolVar(&f.asJSON, "json", false, "Output JSON")
}

// resolve turns the flags into a client and the kamuid_org_id to scope by.
func (f *orgFlags) resolve(ctx context.Context) (*billing.Client, string, error) {
	key := config.ResolveAccessKey(f.key)
	if key == "" {
		return nil, "", errors.New("no access key. Run `kamu login`, or export " + envKey + "=... or pass --key <token>")
	}
	org, err := resolveOrg(f.org, key)
	if err != nil {
		return nil, "", err
	}
	base := config.FromContext(ctx).ResolveKamuhubBase()
	return billing.New(base, key), org, nil
}

// keyOrg is one org as the access key payload describes it.
type keyOrg struct {
	Slug        string `json:"slug"`
	KamuIDOrgID string `json:"kamuid_org_id"`
}

// resolveOrg produces the kamuid_org_id the billing API wants.
//
// Billing rows are keyed on kamuid_org_id, not on a slug, so unlike `kamu
// assets` (whose server resolves slugs) this cannot pass a slug through. The
// mapping comes from the access key payload, which carries both. A --org value
// that matches no slug is sent as-is, so a raw kamuid org id still works and
// an unreadable key is not a dead end.
func resolveOrg(flag, key string) (string, error) {
	orgs := keyOrgs(key)

	if flag != "" {
		for _, o := range orgs {
			if o.Slug == flag && o.KamuIDOrgID != "" {
				return o.KamuIDOrgID, nil
			}
		}
		return flag, nil
	}

	switch len(orgs) {
	case 0:
		return "", errors.New("cannot tell which org to bill-query from the access key; pass --org <slug|kamuid org id>")
	case 1:
		if orgs[0].KamuIDOrgID == "" {
			return "", errors.New("the access key names an org but no kamuid org id; pass --org <kamuid org id>")
		}
		return orgs[0].KamuIDOrgID, nil
	default:
		names := make([]string, 0, len(orgs))
		for _, o := range orgs {
			names = append(names, o.Slug)
		}
		return "", fmt.Errorf("the access key covers several orgs (%s); pass --org <slug>", strings.Join(names, ", "))
	}
}

// keyOrgs reads the orgs out of the access key payload. No signature check —
// the BFF verifies the key; this only needs to route the request.
func keyOrgs(key string) []keyOrg {
	parts := strings.Split(key, ".")
	if len(parts) != 3 {
		return nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil
	}
	var p struct {
		Orgs []keyOrg `json:"orgs"`
	}
	if json.Unmarshal(raw, &p) != nil {
		return nil
	}
	return p.Orgs
}

// euros renders cents the way an invoice line reads, currency included.
func euros(cents int64, currency string) string {
	sign := ""
	if cents < 0 {
		sign = "-"
		cents = -cents
	}
	return fmt.Sprintf("%s%d,%02d %s", sign, cents/100, cents%100, strings.ToUpper(currency))
}

// day trims an RFC3339 timestamp (or a bare date) to the date. Billing
// paperwork is read by day, never by second.
func day(ts string) string {
	if len(ts) >= 10 {
		return ts[:10]
	}
	return ts
}

// dash renders an empty field as "-" so table columns stay readable.
func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func ctxOrTodo(ctx context.Context) context.Context {
	if ctx == nil {
		return context.TODO()
	}
	return ctx
}
