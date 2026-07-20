You are working in the **kamucli** repository — the public `kamu` CLI, one binary that
drives the whole Kamu platform (kamudb, kamubee, kamudns, kamusites, kamustatus, org and
git operations) from one login against **kamuid**. Standalone public repo, un-folded from
the kamuhub monorepo (2026-07-02). See `README.md` for the user-facing story; this file is
the working agreement.

## What lives here (and what does not)

The CLI is a thin, well-behaved client. Business logic, authorization, and tenancy live in
the services — the CLI presents credentials and renders responses, it does not make policy.
If a command needs a new capability, the server owns the rule; the CLI owns the UX. Identity
→ KamuID; authz + billing + access keys → kamuhub; per-product logic → each service.

## Stack

| Layer | Technology |
|---|---|
| **Language** | Go (see `go.mod` for the pinned version) |
| **Commands** | `spf13/cobra` |
| **Prompts** | `charmbracelet/huh` + the in-repo `internal/picker` (one component, every menu inherits it) |
| **Config** | `yaml.v3` → `~/.kamu/config.yml` (override `$KAMU_CONFIG`), written 0600 in a 0700 dir, atomic temp+rename |
| **Release** | GoReleaser off `vX.Y.Z` tags — cross-compiles darwin/linux amd64+arm64, publishes the GitHub release, pushes the Homebrew formula |

## Layout

- `cmd/kamu/main.go` — entrypoint; builds the root command and executes.
- `internal/command/<name>/` — one package per top-level command; `command.go` holds the thin
  cobra wrapper (`Runner`/`Preparer` pattern). Register new commands in `internal/command/root/root.go`.
- `internal/client/<service>/` — one API client per service (kamuid, kamuhub, kamusites, …).
- Shared: `internal/config`, `internal/iostreams`, `internal/render`, `internal/picker`.

## Auth model (load-bearing — don't conflate the two credentials)

- **Login** is the OIDC device-authorization grant (RFC 8628) against **kamuid** (issuer
  `accounts.kamuhub.com`, public client `kamu-cli`, no secret). The KamuID token is then
  exchanged at kamuhub `POST /api/cli/login` for a **kamuhub access key** (ADR 0006).
- **Product commands** (sites, dns, status, clone, git) present the **kamuhub access key** as
  the bearer — a deliberately narrowed, org-scoped, TTL'd, revocable credential. Tenant scope
  is baked into the key (minted per-org at `kamu login --org <slug>`), not sent per request.
- **Org-management commands** (`kamu orgs create/show/delete/invite/remove`) act **as the user**,
  not the access key: a refresh-grant token audienced to kamuid's RP API (`{issuer}/api/v1/rp`),
  cached separately in config. Org lifecycle is a user concern, so an agent/CI holding only an
  access key cannot manage orgs — that is intentional, don't "fix" it by routing orgs through
  the access key.

## Conventions

- **English** for all command surfaces and help text. **No emojis** in code, output, or commits.
- Run `gofmt` before committing; `go build ./...`, `go vet ./...`, and `go test ./...` must be green.
- Prefer the shared `iostreams`/`render` helpers over ad-hoc `fmt.Println`; respect TTY detection
  (colour/prompts off when not a TTY, and refuse destructive prompts non-interactively unless `--yes`).
- **Fail fast, no defensive fallbacks** for cases that can't happen. Validate at the boundary
  (flags, user input, API responses); trust internal code. When a stored credential is too old
  for a new scope, detect it and tell the user to re-login — don't silently degrade.
- Don't add abstractions, flags, or backwards-compat shims beyond what the task needs.

## Git / releases

- Feature work goes on a branch → PR → merge to `main` (this repo uses PRs; not every sibling does).
- Commit subject style: `area: imperative summary` (e.g. `orgs: …`, `auth: …`, `sites: …`).
  The body documents *why* — reasoning, alternatives, non-obvious constraints — not process noise.
- Cutting a release = tag `main` with `vX.Y.Z` and push the tag; CI (GoReleaser) does the rest.
  The tag is the release. Bump minor for new commands, patch for fixes.
