// Package clone implements `kamu clone` — clone any project's git repository
// from the platform forge (git.kamuhub.com). Git access is a platform
// capability (kamuhub ADR 0005): the forge hosts repos for ALL project types,
// so clone is a top-level verb, not a per-product one. Auth is a kamuhub
// access key (a scoped, signed platform context); export KAMU_ACCESS_KEY or
// pass --key — the same key is what git presents on every later fetch/push.
package clone

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/kotisivukamu/kamucli/internal/client/forge"
	"github.com/kotisivukamu/kamucli/internal/client/kamuid"
	"github.com/kotisivukamu/kamucli/internal/command"
	"github.com/kotisivukamu/kamucli/internal/config"
	"github.com/kotisivukamu/kamucli/internal/iostreams"
	"github.com/kotisivukamu/kamucli/internal/picker"
)

const envKey = "KAMU_ACCESS_KEY"

// inlineHelper is the one-shot credential helper used for the clone itself:
// the access key rides in the environment of that single git process and is
// never written to disk or embedded in the remote URL. The empty
// `credential.helper=` preceding it (see runClone) clears any inherited
// helpers so nothing else sees — or offers to store — the credential.
const inlineHelper = `!f(){ echo "username=$KAMU_GIT_USER"; echo "password=$KAMU_GIT_TOKEN"; };f`

func New() *cobra.Command {
	var key string
	cmd := command.New("clone [project] [dir]", "Clone a project's repository from the platform forge",
		`Clone a project's git repository over HTTPS from the platform forge
(git.kamuhub.com), which hosts repos for every project type.

Your access key IS the git credential: the clone passes it to git in memory
(never in the URL, never on disk), and the cloned repo is wired to the
"kamu git-credential" helper so later fetch/push present it automatically.
Revoke the key in the dashboard and git access stops with it.

<project> is a repo name or owner/name; a substring of the name or
description also works. Omit it to pick interactively.`,
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
	client := forge.New(os.Getenv(forge.EnvURL), k)

	repos, err := client.Repos(ctx)
	if err != nil {
		return friendlyListError(err)
	}
	want := ""
	if len(args) > 0 {
		want = args[0]
	}
	target, err := chooseRepo(ctx, repos, want)
	if err != nil {
		return err
	}

	dir := target.Name
	if len(args) > 1 {
		dir = args[1]
	}

	// Clone with a one-shot inline helper: the leading empty helper clears any
	// inherited (system/global) helpers for this single process, and the key
	// travels via env — never on the command line, never in the URL, never on
	// disk.
	gitArgs := []string{"-c", "credential.helper=", "-c", "credential.helper=" + inlineHelper, "clone"}
	if target.DefaultBranch != "" {
		gitArgs = append(gitArgs, "--branch", target.DefaultBranch)
	}
	gitArgs = append(gitArgs, target.CloneURL, dir)
	clone := exec.CommandContext(ctx, "git", gitArgs...)
	clone.Stdout = io.Out
	clone.Stderr = io.ErrOut
	clone.Env = append(os.Environ(),
		"KAMU_GIT_USER=kamu", // cosmetic; the forge reads only the password
		"KAMU_GIT_TOKEN="+k,
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
	// asked to STORE the access key on approval), then install ours, then stamp
	// the project marker the helper requires before it ever answers.
	if err := gitConfig("--add", "credential.helper", ""); err != nil {
		return err
	}
	if err := gitConfig("--add", "credential.helper", "!kamu git-credential"); err != nil {
		return err
	}
	if err := gitConfig("kamu.project", target.FullName()); err != nil {
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

	fmt.Fprintf(io.Out, "\nCloned %s into %s\n", target.FullName(), dir)
	fmt.Fprintf(io.Out, "  origin: %s\n", target.CloneURL)
	fmt.Fprintln(io.Out, "  auth:   git push just works — every git operation presents your access key ("+envKey+"); nothing is stored on disk, and revoking the key cuts access instantly.")
	return nil
}

// resolveKey pulls the access key from --key or the env, with the same guidance
// the sites commands give.
func resolveKey(key string) (string, error) {
	if key == "" {
		key = os.Getenv(envKey)
	}
	if key == "" {
		return "", errors.New("no access key. Create one in the dashboard (Manage -> Access keys) and pass it:\n\n    export " + envKey + "=...\n\nor --key <token>")
	}
	return key, nil
}

// chooseRepo resolves the <project> argument (or its absence) to exactly one
// repo: exact matches via matchRepos, the shared picker on ambiguity, and a
// clear error when there is no terminal to pick on.
func chooseRepo(ctx context.Context, repos []forge.Repo, want string) (*forge.Repo, error) {
	matches := matchRepos(repos, want)
	switch len(matches) {
	case 0:
		if want == "" {
			return nil, errors.New("no repos visible to this access key")
		}
		return nil, fmt.Errorf("no project matches %q", want)
	case 1:
		return &matches[0], nil
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		if want == "" {
			return nil, errors.New("multiple projects; pass a project name (or owner/name)")
		}
		return nil, fmt.Errorf("%q matches %d projects; pass an exact name or owner/name", want, len(matches))
	}
	opts := make([]picker.Option[string], 0, len(matches))
	for _, r := range matches {
		opts = append(opts, picker.Option[string]{
			Value:       r.FullName(),
			Label:       r.Name,
			Description: r.Description,
		})
	}
	picked, err := picker.Pick(ctx, picker.Config[string]{
		Title:       "Which project?",
		Description: "Type to filter, enter to select, esc to cancel.",
		Options:     opts,
	})
	if err != nil {
		return nil, err
	}
	for i := range matches {
		if matches[i].FullName() == picked {
			return &matches[i], nil
		}
	}
	return nil, errors.New("no project selected")
}

// matchRepos narrows the visible repos to the candidates for <project>: an
// exact owner/name or (case-insensitive) name match wins first — possibly
// several names across owners; otherwise a substring match against name and
// description; an empty want keeps everything (interactive pick). Pure so it's
// testable without a terminal or an API.
func matchRepos(all []forge.Repo, want string) []forge.Repo {
	if want == "" {
		return all
	}
	var exact []forge.Repo
	for _, r := range all {
		if r.FullName() == want || strings.EqualFold(r.Name, want) {
			exact = append(exact, r)
		}
	}
	if len(exact) > 0 {
		return exact
	}
	needle := strings.ToLower(want)
	var loose []forge.Repo
	for _, r := range all {
		if strings.Contains(strings.ToLower(r.Name), needle) ||
			strings.Contains(strings.ToLower(r.Description), needle) {
			loose = append(loose, r)
		}
	}
	return loose
}

// friendlyListError rewrites forge API failures into guidance.
func friendlyListError(err error) error {
	var apiErr *forge.APIError
	if !errors.As(err, &apiErr) {
		return err
	}
	if apiErr.StatusCode == 401 {
		return errors.New("access key invalid or revoked — mint a new one in the dashboard (Manage -> Access keys) and export " + envKey + "=...")
	}
	return err
}
