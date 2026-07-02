# kamu

Unified CLI for the Kamu platform — drive **kamudb** (databases), **kamubee** (apps), **kamudns** (DNS), **kamusites** (websites), and **kamustatus** (uptime monitoring) from one binary with one login against **kamuid**.

```
kamu auth login
kamu db list
kamu bee apps
kamu dns zones
kamu sites list
kamu clone my-project
kamu status projects list
```

`kamu status` talks to **kamustatus**, a kamuhub resource server. It uses the
unified platform identity — the `kamu auth login` token, or a kamuhub access key
(`export KAMU_ACCESS_KEY=...`, same as `kamu sites`). No project-scoped keys.

`kamu clone <project>` clones a project's git repository over HTTPS from the
platform forge (`git.kamuhub.com`), which hosts repos for **every** project
type — git access is a platform capability, not a sites feature. Your kamuhub
access key is the git credential: the clone hands it to git in memory (never
in the URL, never on disk), and installs `kamu git-credential` as the
repo-local credential helper, so plain `git pull` / `git push` just work
afterwards — git asks the helper, the helper presents the key.

## Working with agents

The credential-helper flow means any coding agent (or CI job) that shells out
to ordinary `git` just works inside a cloned repo — no token pasting, no
remote-URL surgery. Give the agent's box a scoped kamuhub access key
(dashboard → Manage → Access keys):

```sh
export KAMU_ACCESS_KEY=...
kamu clone my-project
cd my-project && git push   # every git operation presents the access key
```

Access keys are scoped to a subset of your grants, TTL'd, and revocable from
the dashboard — whatever is on the machine is the most an agent can use, and
revoking the key cuts its git access instantly, platform-side.

## Install

### Homebrew

```sh
brew install kotisivukamu/tap/kamu
```

### From source

Requires Go 1.25+.

```sh
go install github.com/kotisivukamu/kamucli/cmd/kamu@latest
```

### Pre-built binaries

Download from [Releases](https://github.com/kotisivukamu/kamucli/releases). Archives for `darwin_amd64`, `darwin_arm64`, `linux_amd64`, `linux_arm64`.

## History

This repo started as `kamu-cli` (public, released through **v0.4.1**), was folded
into the private **kamuhub** monorepo as `cli/` in June 2026, and un-folded back
here as **kamucli** in July 2026 — kamuhub is private, so brew/`go install`
could not reach it (see kamuhub ADR 0005). The version line continues where the
public repo left off: the first post-un-fold release is **v0.5.0**.

The intended direction is for `kamu` to drive the platform **through the kamuhub
BFF** (`app.kamuhub.com`) — one base URL, one session, the same signed grant
context the dashboard uses — rather than talking to each service directly.

## Development

```sh
go build -o kamu ./cmd/kamu
./kamu --help
./kamu version
```

Layout follows [flyctl](https://github.com/superfly/flyctl): one package per noun under `internal/command/`, one file per verb.

## Release

Push a plain **`vX.Y.Z`** tag; GitHub Actions (`.github/workflows/release.yml`)
runs GoReleaser, publishes the GitHub release, and pushes the Homebrew formula to
[kotisivukamu/homebrew-tap](https://github.com/kotisivukamu/homebrew-tap)
(requires the `HOMEBREW_TAP_TOKEN` repo secret; without it the release still
ships binaries and skips the tap push).
