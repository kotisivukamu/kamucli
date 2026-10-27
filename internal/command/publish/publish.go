// Package publish implements `kamu publish` — put a self-contained HTML file on
// the web from the terminal.
//
// One published file = one forge repo (the platform provisions it; see kamuhub
// bff/publish.go). That is what makes the page versioned for free: this command
// commits the file into the repo and pushes, the repo's Forgejo Actions workflow
// deploys it to the kamucdn files plane, and `git log` in the repo is the page's
// revision history. Republishing the same local file finds the same repo and
// pushes another commit, so the URL never moves.
//
// Auth is a kamuhub access key — the same credential `kamu clone` uses, and the
// same one git presents to the forge — so publishing needs no second login.
package publish

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	pubapi "github.com/kotisivukamu/kamucli/internal/client/publish"
	"github.com/kotisivukamu/kamucli/internal/command"
	"github.com/kotisivukamu/kamucli/internal/config"
	"github.com/kotisivukamu/kamucli/internal/iostreams"
	"github.com/kotisivukamu/kamucli/internal/render"
)

const envKey = "KAMU_ACCESS_KEY"

// inlineHelper is the one-shot git credential helper (identical to the one
// `kamu clone` installs): the access key rides in the environment of that single
// git process, never on disk and never in a remote URL.
const inlineHelper = `!f(){ echo "username=$KAMU_GIT_USER"; echo "password=$KAMU_GIT_TOKEN"; };f`

func New() *cobra.Command {
	var key, org, name, repo, message, password string
	var asJSON bool

	// Declared before the closure so the run function can ask cobra whether
	// --password was actually passed (see setPassword below).
	var cmd *cobra.Command
	cmd = command.New("publish <file.html>", "Publish a self-contained HTML file to the web",
		`Publish a self-contained HTML file and get a shareable URL.

The platform creates one git repository per published file. This command commits
the file into that repository and pushes; the push deploys the page. Publishing
the same local file again pushes another commit to the same repository, so the
URL stays put and every version is in the repo's history.

Run "kamu publish list" to see what your organization has published.`,
		func(ctx context.Context, args []string) error {
			if ctx == nil {
				ctx = context.TODO()
			}
			return runPublish(ctx, args[0], opts{
				key: key, org: org, name: name, repo: repo,
				message: message, password: password,
				// An explicitly-passed empty --password means "remove the
				// password", which is indistinguishable from the default
				// without asking cobra whether the flag was set at all.
				setPassword: cmd.Flags().Changed("password"),
				asJSON:      asJSON,
			})
		})
	cmd.Args = cobra.ExactArgs(1)
	cmd.Flags().StringVar(&key, "key", "", "kamuhub access key (or "+envKey+")")
	cmd.Flags().StringVar(&org, "org", "", "organization slug (required when the key covers several)")
	cmd.Flags().StringVar(&name, "name", "", "display name for a new publish (default: the file name)")
	cmd.Flags().StringVar(&repo, "repo", "", "republish to this existing publish repo (owner/name)")
	cmd.Flags().StringVarP(&message, "message", "m", "", "commit message (default: \"Publish <file>\")")
	cmd.Flags().StringVar(&password, "password", "", "password-protect the page (empty string clears an existing password)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "output the publish record as JSON")

	cmd.AddCommand(newList())
	return cmd
}

type opts struct {
	key, org, name, repo, message, password string
	setPassword                             bool
	asJSON                                  bool
}

func newList() *cobra.Command {
	var key, org string
	var asJSON bool
	cmd := command.New("list", "List the files your organization has published", "",
		func(ctx context.Context, args []string) error {
			if ctx == nil {
				ctx = context.TODO()
			}
			io := iostreams.FromContext(ctx)
			k, err := resolveKey(key)
			if err != nil {
				return err
			}
			o, err := resolveOrg(org, k)
			if err != nil {
				return err
			}
			list, err := client(k).List(ctx, o)
			if err != nil {
				return friendly(err)
			}
			if asJSON {
				return json.NewEncoder(io.Out).Encode(list)
			}
			if len(list) == 0 {
				fmt.Fprintln(io.Out, "No published files yet. Publish one with `kamu publish <file.html>`.")
				return nil
			}
			rows := make([][]string, 0, len(list))
			for _, p := range list {
				rows = append(rows, []string{p.Name, p.PublicURL, p.Repo})
			}
			render.Table(io.Out, []string{"NAME", "URL", "REPO"}, rows)
			return nil
		})
	cmd.Args = cobra.NoArgs
	cmd.Flags().StringVar(&key, "key", "", "kamuhub access key (or "+envKey+")")
	cmd.Flags().StringVar(&org, "org", "", "organization slug")
	cmd.Flags().BoolVar(&asJSON, "json", false, "output as JSON")
	return cmd
}

func runPublish(ctx context.Context, file string, o opts) error {
	io := iostreams.FromContext(ctx)

	abs, err := filepath.Abs(file)
	if err != nil {
		return err
	}
	content, err := os.ReadFile(abs)
	if err != nil {
		return fmt.Errorf("read %s: %w", file, err)
	}
	if len(content) == 0 {
		return fmt.Errorf("%s is empty", file)
	}
	// A published page is a single self-contained document. Refusing anything
	// else here is clearer than letting the CDN serve a .png as index.html.
	if ext := strings.ToLower(filepath.Ext(abs)); ext != ".html" && ext != ".htm" {
		return fmt.Errorf("kamu publish takes a self-contained HTML file (got %q)", ext)
	}

	k, err := resolveKey(o.key)
	if err != nil {
		return err
	}
	orgSlug, err := resolveOrg(o.org, k)
	if err != nil {
		return err
	}
	api := client(k)

	state, err := loadState()
	if err != nil {
		return fmt.Errorf("read publish state: %w", err)
	}

	// Which repo are we pushing to? An explicit --repo wins; otherwise the one
	// this exact file was published to before; otherwise provision a new one.
	rec, known := Record{}, false
	switch {
	case o.repo != "":
		rec, known = state.lookupByRepo(o.repo)
		if !known {
			// A repo the local state has never seen (another machine, or a repo
			// named by hand). Look it up in the org's publishes rather than
			// guessing its clone URL.
			list, err := api.List(ctx, orgSlug)
			if err != nil {
				return friendly(err)
			}
			for _, p := range list {
				if strings.EqualFold(p.Repo, o.repo) {
					rec, known = fromPublish(&p), true
					break
				}
			}
			if !known {
				return fmt.Errorf("no publish repo %q in this organization", o.repo)
			}
		}
	default:
		rec, known = state.Files[abs]
	}

	if !known {
		name := o.name
		if name == "" {
			name = strings.TrimSuffix(filepath.Base(abs), filepath.Ext(abs))
		}
		fmt.Fprintf(io.ErrOut, "Creating a publish repository for %q...\n", name)
		p, err := api.Create(ctx, orgSlug, name)
		if err != nil {
			return friendly(err)
		}
		rec = fromPublish(p)
	}

	dir, err := workdir(rec.Repo)
	if err != nil {
		return err
	}
	if err := syncClone(ctx, dir, rec, k); err != nil {
		return err
	}

	target := filepath.Join(dir, filepath.FromSlash(rec.Path))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(target, content, 0o644); err != nil {
		return err
	}

	msg := o.message
	if msg == "" {
		msg = "Publish " + filepath.Base(abs)
	}
	changed, err := commitAndPush(ctx, dir, rec, msg, k)
	if err != nil {
		return err
	}

	state.Files[abs] = rec
	if err := saveState(state); err != nil {
		// The push already happened and the page is live; a lost state entry only
		// means the next republish of this path needs --repo.
		fmt.Fprintf(io.ErrOut, "warning: could not record publish state: %v\n", err)
	}

	if o.setPassword {
		// Protection can only be set on a fileset that exists, and the artifact
		// only exists once the push's workflow has deployed it — so wait for the
		// page to go live rather than failing on a race the user can't see.
		if err := protectWhenLive(ctx, io, api, orgSlug, rec, o.password); err != nil {
			return err
		}
	}

	if o.asJSON {
		return json.NewEncoder(io.Out).Encode(rec)
	}
	if !changed {
		fmt.Fprintln(io.ErrOut, "No changes since the last publish.")
	} else {
		fmt.Fprintln(io.ErrOut, "Pushed. The page redeploys in a few seconds.")
	}
	fmt.Fprintln(io.Out, rec.PublicURL)
	return nil
}

// syncClone makes dir a clean, up-to-date checkout of the publish repo: a clone
// the first time, a fetch + hard reset afterwards. Hard reset on purpose — this
// working copy is the CLI's own scratch space, never a place a user edits, so
// there is nothing to preserve and a merge conflict here would only be confusing.
func syncClone(ctx context.Context, dir string, rec Record, key string) error {
	env := append(os.Environ(), "KAMU_GIT_USER=kamu", "KAMU_GIT_TOKEN="+key)
	cred := []string{"-c", "credential.helper=", "-c", "credential.helper=" + inlineHelper}

	if _, err := os.Stat(filepath.Join(dir, ".git")); errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(filepath.Dir(dir), 0o700); err != nil {
			return err
		}
		args := append(append([]string{}, cred...), "clone", "--branch", rec.Branch, rec.CloneURL, dir)
		return git(ctx, "", env, args...)
	}
	args := append(append([]string{}, cred...), "-C", dir, "fetch", "origin", rec.Branch)
	if err := git(ctx, "", env, args...); err != nil {
		return err
	}
	if err := git(ctx, dir, env, "checkout", "-B", rec.Branch); err != nil {
		return err
	}
	return git(ctx, dir, env, "reset", "--hard", "origin/"+rec.Branch)
}

// commitAndPush commits the staged page and pushes. Reports whether anything
// actually changed: republishing an unmodified file is a no-op, not an error.
func commitAndPush(ctx context.Context, dir string, rec Record, message, key string) (bool, error) {
	if err := git(ctx, dir, nil, "add", "--", rec.Path); err != nil {
		return false, err
	}
	// `diff --cached --quiet` exits 1 when there is something staged.
	check := exec.CommandContext(ctx, "git", "-C", dir, "diff", "--cached", "--quiet")
	if err := check.Run(); err == nil {
		return false, nil
	}
	// Identity is set per-repo rather than relying on the user's global git
	// config: this scratch clone must be committable on a machine that has none.
	if err := git(ctx, dir, nil, "config", "user.name", "kamu"); err != nil {
		return false, err
	}
	if err := git(ctx, dir, nil, "config", "user.email", "kamu@kamuhub.com"); err != nil {
		return false, err
	}
	if err := git(ctx, dir, nil, "commit", "-m", message); err != nil {
		return false, err
	}
	env := append(os.Environ(), "KAMU_GIT_USER=kamu", "KAMU_GIT_TOKEN="+key)
	args := []string{"-c", "credential.helper=", "-c", "credential.helper=" + inlineHelper,
		"-C", dir, "push", "origin", rec.Branch}
	return true, git(ctx, "", env, args...)
}

// protectWhenLive retries the protection call until the first deploy has landed.
// kamuhub reports a not-yet-deployed fileset as a gateway failure (the CDN 404s
// it), and a fresh publish takes a few seconds of runner time, so a single
// attempt would fail for everyone publishing a protected page for the first time.
func protectWhenLive(ctx context.Context, io *iostreams.IOStreams, api *pubapi.Client, org string, rec Record, password string) error {
	fmt.Fprintln(io.ErrOut, "Waiting for the first deploy before setting the password...")
	var last error
	for attempt := 0; attempt < 12; attempt++ {
		if err := api.Protect(ctx, org, rec.ID, password); err == nil {
			fmt.Fprintln(io.ErrOut, "Password set.")
			return nil
		} else {
			last = err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
		}
	}
	return fmt.Errorf("the page is published, but setting the password failed: %w", last)
}

// git runs one git invocation, streaming stderr so the user sees git's own
// diagnostics (auth failures especially) instead of a swallowed exit code.
func git(ctx context.Context, dir string, env []string, args ...string) error {
	if dir != "" {
		args = append([]string{"-C", dir}, args...)
	}
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Stderr = os.Stderr
	if env != nil {
		cmd.Env = env
	}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git %s: %w", args[len(args)-1], err)
	}
	return nil
}

func fromPublish(p *pubapi.Publish) Record {
	return Record{
		ID: p.ID, Org: p.Org, Repo: p.Repo, CloneURL: p.CloneURL,
		Branch: p.Branch, Path: p.Path, PublicURL: p.PublicURL,
	}
}

func client(key string) *pubapi.Client {
	c, _ := config.Load()
	if c == nil {
		c = &config.Config{}
	}
	return pubapi.New(c.ResolveKamuhubBase(), key)
}

func resolveKey(flag string) (string, error) {
	k := config.ResolveAccessKey(flag)
	if k == "" {
		return "", errors.New("no access key. Run `kamu login`, or export " + envKey + "=... or pass --key <token>")
	}
	return k, nil
}

// resolveOrg picks the org slug to send: --org, else the single org in the access
// key's payload. A key spanning several orgs errors here rather than letting the
// server 400 — same rule as `kamu assets`.
func resolveOrg(flag, key string) (string, error) {
	if flag != "" {
		return flag, nil
	}
	slugs := keyOrgSlugs(key)
	switch len(slugs) {
	case 0:
		return "", nil
	case 1:
		return slugs[0], nil
	default:
		return "", fmt.Errorf("the access key covers several orgs (%s); pass --org <slug>", strings.Join(slugs, ", "))
	}
}

// keyOrgSlugs reads the org slugs out of the access key payload without
// verifying it — the server verifies; this only decides where to route.
func keyOrgSlugs(key string) []string {
	parts := strings.Split(key, ".")
	if len(parts) != 3 {
		return nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil
	}
	var p struct {
		Orgs []struct {
			Slug string `json:"slug"`
		} `json:"orgs"`
	}
	if json.Unmarshal(raw, &p) != nil {
		return nil
	}
	var slugs []string
	for _, o := range p.Orgs {
		if o.Slug != "" {
			slugs = append(slugs, o.Slug)
		}
	}
	return slugs
}

// friendly turns the statuses a user can actually act on into guidance.
func friendly(err error) error {
	var apiErr *pubapi.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.StatusCode {
		case 401:
			return errors.New("access key rejected. Run `kamu login` to get a fresh one")
		case 403:
			return errors.New("this key is not allowed to publish for that organization (needs the files.manage grant)")
		case 503:
			return errors.New("publishing is not enabled on this platform yet")
		}
	}
	return err
}
