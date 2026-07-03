// Package gitcredential implements the hidden `kamu git-credential` command —
// the git credential helper that `kamu clone` installs repo-locally. git
// invokes it as `kamu git-credential <get|store|erase>` with key=value
// attribute lines on stdin (terminated by a blank line or EOF).
//
// `get` answers with the kamuhub access key itself: the platform forge
// (git.kamuhub.com, kamuhub ADR 0005) accepts the access key as the git
// smart-HTTP password directly, so there is nothing to mint and nothing to
// persist — `store` and `erase` are no-ops. The helper only speaks for repos
// stamped with `git config kamu.project` at clone time, and only for the
// forge's own host, so the key never leaks to unrelated remotes.
package gitcredential

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"github.com/kotisivukamu/kamucli/internal/client/forge"
	"github.com/kotisivukamu/kamucli/internal/command"
	"github.com/kotisivukamu/kamucli/internal/config"
	"github.com/kotisivukamu/kamucli/internal/iostreams"
)

const envKey = "KAMU_ACCESS_KEY"

func New() *cobra.Command {
	cmd := command.New("git-credential <get|store|erase>", "Git credential helper for kamu project repos",
		"Git credential helper protocol backend, installed repo-locally by `kamu clone`. Not meant to be run by hand.",
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
		// The credential is the access key itself; there is nothing to store or
		// revoke here (revocation is instant platform-side).
		return nil
	}
	return fmt.Errorf("unknown credential action %q (want get, store, or erase)", args[0])
}

// get speaks the access key back in the credential-helper protocol. Failures
// go to stderr with a non-zero exit (git surfaces the message), so the fix —
// usually a missing KAMU_ACCESS_KEY on a fresh shell — is visible instead of
// a silent auth prompt.
func get(ctx context.Context, io *iostreams.IOStreams, attrs map[string]string) error {
	// The kamu.project stamp is the "this is a kamu repo" marker: without it,
	// stay out of the way entirely so the helper is inert in non-kamu repos
	// (git configs can leak in via includes; better to fail loudly here than
	// answer for a repo we never cloned).
	if _, err := repoProject(ctx); err != nil {
		return errors.New("not a kamu project repo (git config kamu.project is unset) — clone with `kamu clone`")
	}
	// Only answer for the platform forge's own host: a repo can have other
	// remotes, and git asks every configured helper about each of them.
	// Staying silent (exit 0, no output) lets git fall through to its other
	// helpers without ever leaking the access key to an unrelated host.
	if !hostMatches(attrs, forge.BaseURL()) {
		return nil
	}
	key := config.ResolveAccessKey("")
	if key == "" {
		return errors.New("no access key. Run `kamu login` (or export " + envKey + "=...)")
	}
	fmt.Fprintln(io.Out, "username=kamu") // cosmetic; the forge reads only the password
	fmt.Fprintf(io.Out, "password=%s\n", key)
	return nil
}

// repoProject reads the owner/name stamped by `kamu clone`. git runs
// credential helpers with the repo as cwd, so plain `git config` resolves the
// repo-local value.
func repoProject(ctx context.Context) (string, error) {
	out, err := exec.CommandContext(ctx, "git", "config", "--get", "kamu.project").Output()
	if err != nil {
		return "", err
	}
	p := strings.TrimSpace(string(out))
	if p == "" {
		return "", errors.New("kamu.project is empty")
	}
	return p, nil
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
// is for the forge base URL the key belongs to. git includes the port in the
// host attribute when non-default, matching url.URL's Host.
func hostMatches(attrs map[string]string, base string) bool {
	u, err := url.Parse(base)
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
