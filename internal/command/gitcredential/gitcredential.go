// Package gitcredential implements the hidden `kamu git-credential` command —
// the git credential helper that `kamu sites clone` installs repo-locally.
// git invokes it as `kamu git-credential <get|store|erase>` with key=value
// attribute lines on stdin (terminated by a blank line or EOF).
//
// `get` mints a fresh short-lived credential for the repo's site (identified
// by the repo-local `git config kamu.site-id` stamped at clone time) through
// the kamusites git-credentials endpoint, on the same access-key rail as the
// other `kamu sites` commands. Nothing is ever persisted, so `store` and
// `erase` are no-ops.
package gitcredential

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"github.com/kotisivukamu/kamucli/internal/client/kamusites"
	"github.com/kotisivukamu/kamucli/internal/command"
	"github.com/kotisivukamu/kamucli/internal/iostreams"
)

const (
	envKey = "KAMU_ACCESS_KEY"
	envURL = "KAMU_KAMUSITES_URL"
)

func New() *cobra.Command {
	cmd := command.New("git-credential <get|store|erase>", "Git credential helper for kamu site repos",
		"Git credential helper protocol backend, installed repo-locally by `kamu sites clone`. Not meant to be run by hand.",
		run)
	cmd.Hidden = true
	cmd.Args = cobra.ExactArgs(1)
	return cmd
}

func run(ctx context.Context, args []string) error {
	if ctx == nil {
		ctx = context.TODO()
	}
	io := iostreams.FromContext(ctx)
	attrs, err := parseAttrs(io.In)
	if err != nil {
		return err
	}
	switch args[0] {
	case "get":
		return get(ctx, io, attrs)
	case "store", "erase":
		// Tokens are ephemeral by design; there is nothing to store or revoke.
		return nil
	}
	return fmt.Errorf("unknown credential action %q (want get, store, or erase)", args[0])
}

// get mints a credential for the enclosing repo's site and speaks it back in
// the credential-helper protocol. Failures go to stderr with a non-zero exit
// (git surfaces the message), so the fix — usually a missing KAMU_ACCESS_KEY
// on a fresh shell — is visible instead of a silent auth prompt.
func get(ctx context.Context, io *iostreams.IOStreams, attrs map[string]string) error {
	siteID, err := repoSiteID(ctx)
	if err != nil {
		return errors.New("not a kamu site repo (git config kamu.site-id is unset) — clone with `kamu sites clone`")
	}
	key := os.Getenv(envKey)
	if key == "" {
		return errors.New("no access key. Create one in the dashboard (Manage -> Access keys) and export " + envKey + "=...")
	}
	creds, err := kamusites.New(os.Getenv(envURL), key).GitCredentials(ctx, siteID)
	if err != nil {
		return err
	}
	// Only answer for the host our credential is actually for: a repo can have
	// other remotes, and git asks every configured helper about each of them.
	// Staying silent (exit 0, no output) lets git fall through to its other
	// helpers without ever leaking the token to an unrelated host.
	if !hostMatches(attrs, creds.CloneURL) {
		return nil
	}
	fmt.Fprintf(io.Out, "username=%s\n", creds.Username)
	fmt.Fprintf(io.Out, "password=%s\n", creds.Password)
	return nil
}

// repoSiteID reads the site id stamped by `kamu sites clone`. git runs
// credential helpers with the repo as cwd, so plain `git config` resolves the
// repo-local value.
func repoSiteID(ctx context.Context) (string, error) {
	out, err := exec.CommandContext(ctx, "git", "config", "--get", "kamu.site-id").Output()
	if err != nil {
		return "", err
	}
	id := strings.TrimSpace(string(out))
	if id == "" {
		return "", errors.New("kamu.site-id is empty")
	}
	return id, nil
}

// parseAttrs reads the credential-helper protocol's key=value input: one
// attribute per line, terminated by a blank line or EOF.
func parseAttrs(r io.Reader) (map[string]string, error) {
	attrs := map[string]string{}
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			break
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("malformed credential attribute %q", line)
		}
		attrs[k] = v
	}
	return attrs, sc.Err()
}

// hostMatches reports whether git's credential request (protocol/host attrs)
// is for the same place the minted clone_url points at. git includes the port
// in the host attribute when non-default, matching url.URL's Host.
func hostMatches(attrs map[string]string, cloneURL string) bool {
	u, err := url.Parse(cloneURL)
	if err != nil {
		return false
	}
	if p := attrs["protocol"]; p != "" && p != u.Scheme {
		return false
	}
	if h := attrs["host"]; h != "" && h != u.Host {
		return false
	}
	return true
}
