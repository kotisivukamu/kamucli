package sites

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/kotisivukamu/kamucli/internal/client/kamuid"
	"github.com/kotisivukamu/kamucli/internal/client/kamusites"
	"github.com/kotisivukamu/kamucli/internal/command"
	"github.com/kotisivukamu/kamucli/internal/config"
	"github.com/kotisivukamu/kamucli/internal/iostreams"
	"github.com/kotisivukamu/kamucli/internal/picker"
)

// inlineHelper is the one-shot credential helper used for the clone itself:
// the token rides in the environment of that single git process and is never
// written to disk or embedded in the remote URL. The empty `credential.helper=`
// preceding it (see runClone) clears any inherited helpers so nothing else
// sees — or offers to store — the ephemeral credential.
const inlineHelper = `!f(){ echo "username=$KAMU_GIT_USER"; echo "password=$KAMU_GIT_TOKEN"; };f`

func newClone() *cobra.Command {
	var key string
	cmd := command.New("clone [site] [dir]", "Clone a site's repository",
		`Clone a site's git repository over HTTPS.

Credentials are minted per operation (a ~2h repo-scoped token) and never
stored: the clone passes the token to git in memory, and the cloned repo is
wired to the "kamu git-credential" helper so later fetch/push mint a fresh
one automatically.

<site> is a site id, slug, or name; omit it to pick interactively.`,
		func(ctx context.Context, args []string) error {
			if ctx == nil {
				ctx = context.TODO()
			}
			return runClone(ctx, args, key)
		})
	cmd.Args = cobra.RangeArgs(0, 2)
	cmd.Flags().StringVar(&key, "key", "", "kamuhub access key (or "+envKey+")")
	return cmd
}

func runClone(ctx context.Context, args []string, key string) error {
	io := iostreams.FromContext(ctx)
	k, err := resolveKey(key)
	if err != nil {
		return err
	}
	client := kamusites.New(os.Getenv(envURL), k)

	sites, err := client.Sites(ctx)
	if err != nil {
		return err
	}
	want := ""
	if len(args) > 0 {
		want = args[0]
	}
	target, err := chooseSite(ctx, sites, want)
	if err != nil {
		return err
	}

	creds, err := client.GitCredentials(ctx, target.ID)
	if err != nil {
		return friendlyCredentialsError(err)
	}

	dir := creds.RepoName
	if len(args) > 1 {
		dir = args[1]
	}

	// Clone with a one-shot inline helper: the leading empty helper clears any
	// inherited (system/global) helpers for this single process, and the token
	// travels via env — never on the command line, never in the URL, never on
	// disk.
	gitArgs := []string{"-c", "credential.helper=", "-c", "credential.helper=" + inlineHelper, "clone"}
	if creds.DefaultBranch != "" {
		gitArgs = append(gitArgs, "--branch", creds.DefaultBranch)
	}
	gitArgs = append(gitArgs, creds.CloneURL, dir)
	clone := exec.CommandContext(ctx, "git", gitArgs...)
	clone.Stdout = io.Out
	clone.Stderr = io.ErrOut
	clone.Env = append(os.Environ(),
		"KAMU_GIT_USER="+creds.Username,
		"KAMU_GIT_TOKEN="+creds.Password,
	)
	if err := clone.Run(); err != nil {
		return fmt.Errorf("git clone: %w", err)
	}

	gitConfig := func(kv ...string) error {
		c := exec.CommandContext(ctx, "git", append([]string{"-C", dir, "config"}, kv...)...)
		c.Stderr = io.ErrOut
		if err := c.Run(); err != nil {
			return fmt.Errorf("git config %s: %w", kv[len(kv)-2], err)
		}
		return nil
	}
	// Repo-local wiring for every later fetch/push: reset the helper list (an
	// empty entry clears system/global helpers, so e.g. a keychain never gets
	// asked to STORE our ephemeral token on approval), then install ours, then
	// stamp the site id it mints for.
	if err := gitConfig("--add", "credential.helper", ""); err != nil {
		return err
	}
	if err := gitConfig("--add", "credential.helper", "!kamu git-credential"); err != nil {
		return err
	}
	if err := gitConfig("kamu.site-id", target.ID); err != nil {
		return err
	}
	// Best-effort git author from the logged-in KamuID identity (id_token
	// already on disk — no extra API call); skip silently when not logged in.
	if cfg, err := config.Load(); err == nil && cfg.IDToken != "" {
		if claims, err := kamuid.ParseIDTokenClaims(cfg.IDToken); err == nil {
			if claims.Name != "" {
				_ = gitConfig("user.name", claims.Name)
			}
			if claims.Email != "" {
				_ = gitConfig("user.email", claims.Email)
			}
		}
	}

	fmt.Fprintf(io.Out, "\nCloned %s into %s\n", target.Slug, dir)
	fmt.Fprintf(io.Out, "  origin: %s\n", creds.CloneURL)
	fmt.Fprintln(io.Out, "  auth:   git push just works — credentials are minted per operation and expire in ~2h; nothing is stored on disk.")
	return nil
}

// chooseSite resolves the <site> argument (or its absence) to exactly one site:
// exact id/slug/name matches via matchSites, the shared picker on ambiguity,
// and a clear error when there is no terminal to pick on.
func chooseSite(ctx context.Context, sites []kamusites.Site, want string) (*kamusites.Site, error) {
	matches := matchSites(sites, want)
	switch len(matches) {
	case 0:
		if want == "" {
			return nil, errors.New("no sites visible to this access key")
		}
		return nil, fmt.Errorf("no site matches %q (try `kamu sites list`)", want)
	case 1:
		return &matches[0], nil
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		if want == "" {
			return nil, errors.New("multiple sites; pass a site id, slug, or name (see `kamu sites list`)")
		}
		return nil, fmt.Errorf("%q matches %d sites; pass an id or slug (see `kamu sites list`)", want, len(matches))
	}
	opts := make([]picker.Option[string], 0, len(matches))
	for _, s := range matches {
		opts = append(opts, picker.Option[string]{
			Value:       s.ID,
			Label:       s.Name,
			Description: s.Slug,
		})
	}
	picked, err := picker.Pick(ctx, picker.Config[string]{
		Title:       "Which site?",
		Description: "Type to filter, enter to select, esc to cancel.",
		Options:     opts,
	})
	if err != nil {
		return nil, err
	}
	for i := range matches {
		if matches[i].ID == picked {
			return &matches[i], nil
		}
	}
	return nil, errors.New("no site selected")
}

// matchSites narrows the visible sites to the candidates for <site>: an id or
// slug hit is exact and wins alone; otherwise exact (case-insensitive) name
// matches — possibly several; an empty want keeps everything (interactive
// pick). Pure so it's testable without a terminal or an API.
func matchSites(all []kamusites.Site, want string) []kamusites.Site {
	if want == "" {
		return all
	}
	for _, s := range all {
		if s.ID == want || s.Slug == want {
			return []kamusites.Site{s}
		}
	}
	var named []kamusites.Site
	for _, s := range all {
		if strings.EqualFold(s.Name, want) {
			named = append(named, s)
		}
	}
	return named
}

// friendlyCredentialsError rewrites git-credentials API failures into guidance.
func friendlyCredentialsError(err error) error {
	var apiErr *kamusites.APIError
	if !errors.As(err, &apiErr) {
		return err
	}
	switch apiErr.StatusCode {
	case 403:
		return errors.New("this access key lacks the sites.update grant needed for git access — mint one with it in the dashboard (Manage -> Access keys)")
	case 404:
		return errors.New("site not found (or not in this access key's scope) — try `kamu sites list`")
	case 409:
		return errors.New("site has no repo yet — it gets one on first build/publish")
	}
	return err
}
