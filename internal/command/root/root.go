package root

import (
	"github.com/spf13/cobra"

	"github.com/kotisivukamu/kamucli/internal/command/auth"
	"github.com/kotisivukamu/kamucli/internal/command/bee"
	"github.com/kotisivukamu/kamucli/internal/command/clone"
	"github.com/kotisivukamu/kamucli/internal/command/db"
	"github.com/kotisivukamu/kamucli/internal/command/dns"
	"github.com/kotisivukamu/kamucli/internal/command/gitcredential"
	"github.com/kotisivukamu/kamucli/internal/command/orgs"
	"github.com/kotisivukamu/kamucli/internal/command/sites"
	"github.com/kotisivukamu/kamucli/internal/command/status"
	"github.com/kotisivukamu/kamucli/internal/command/version"
)

type BuildInfo struct {
	Version string
	Commit  string
	Date    string
}

func New(bi BuildInfo) *cobra.Command {
	root := &cobra.Command{
		Use:           "kamu",
		Short:         "Drive the Kamu platform from one CLI",
		Long:          "kamu is the unified CLI for the Kamu platform — manage databases (kamudb), apps (kamubee), and DNS (kamudns) with a single login against kamuid.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.AddGroup(
		&cobra.Group{ID: "platform", Title: "Platform services:"},
		&cobra.Group{ID: "account", Title: "Account & auth:"},
		&cobra.Group{ID: "meta", Title: "More:"},
	)

	add := func(cmd *cobra.Command, group string) *cobra.Command {
		cmd.GroupID = group
		root.AddCommand(cmd)
		return cmd
	}

	add(clone.New(), "platform")
	add(sites.New(), "platform")
	add(db.New(), "platform")
	add(bee.New(), "platform")
	add(dns.New(), "platform")
	add(status.New(), "platform")
	add(auth.New(), "account")
	add(auth.NewLogin(), "account") // top-level `kamu login` alias for `kamu auth login`
	add(orgs.New(), "account")
	add(version.New(bi.Version, bi.Commit, bi.Date), "meta")

	// Hidden plumbing: the git credential helper `kamu clone` installs
	// repo-locally. No group — it never shows in help.
	root.AddCommand(gitcredential.New())

	return root
}
