package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/cli/browser"
	"github.com/spf13/cobra"

	"github.com/kotisivukamu/kamucli/internal/client/kamuhub"
	"github.com/kotisivukamu/kamucli/internal/client/kamuid"
	"github.com/kotisivukamu/kamucli/internal/command"
	"github.com/kotisivukamu/kamucli/internal/config"
	"github.com/kotisivukamu/kamucli/internal/iostreams"
)

// kamuid prod discovery doesn't list "organizations" or the kamu.org.* scopes
// in scopes_supported, but the server still honors them. `organizations` feeds
// the id_token claim (`kamu orgs`, whoami's org count); kamu.org.profile.read +
// kamu.org.manage gate the /v1/rp org endpoints (`kamu orgs create/show/...`)
// and must be consented HERE — a refresh grant can only re-issue scopes the
// original grant carried, so logins predating them need a re-login.
const defaultScope = config.DefaultScopes

type loginFlags struct {
	scope     string
	org       string
	noBrowser bool
}

// NewLogin exposes the login command so root can also mount it top-level as
// `kamu login` alongside `kamu auth login`.
func NewLogin() *cobra.Command { return newLogin() }

func newLogin() *cobra.Command {
	var f loginFlags
	cmd := command.New("login", "Log in: device flow against kamuid, then mint a kamuhub access key", "",
		func(ctx context.Context, _ []string) error { return runLogin(ctx, &f) })
	cmd.Flags().StringVar(&f.scope, "scope", defaultScope, "OAuth scopes to request")
	cmd.Flags().StringVar(&f.org, "org", "", "Organization to scope the access key to (default: your first org)")
	cmd.Flags().BoolVar(&f.noBrowser, "no-browser", false, "Don't open the verification URL in a browser")
	return cmd
}

func runLogin(ctx context.Context, f *loginFlags) error {
	io := iostreams.FromContext(ctx)

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	issuer := cfg.ResolveIssuer()
	clientID := cfg.ResolveClientID()

	client := kamuid.New(issuer)

	da, err := client.StartDeviceAuth(ctx, clientID, f.scope)
	if err != nil {
		return fmt.Errorf("start device auth: %w", err)
	}

	fmt.Fprintf(io.ErrOut, "First copy this one-time code:\n\n    %s\n\n", da.UserCode)
	fmt.Fprintf(io.ErrOut, "Then open:\n    %s\n\n", da.VerificationURIComplete)

	if !f.noBrowser {
		if err := browser.OpenURL(da.VerificationURIComplete); err != nil {
			fmt.Fprintf(io.ErrOut, "(couldn't open browser automatically: %v)\n", err)
		}
	}
	fmt.Fprintln(io.ErrOut, "Waiting for approval...")

	// Request the kamuhub audience so the access token comes back as an
	// audience-bound JWT the BFF can verify locally when we mint the access key.
	ts, err := client.PollDeviceToken(ctx, clientID, config.KamuhubAudience, da)
	if err != nil {
		return fmt.Errorf("device token: %w", err)
	}

	cfg.ClientID = clientID
	cfg.Endpoints.Kamuid = issuer
	cfg.AccessToken = ts.AccessToken
	cfg.RefreshToken = ts.RefreshToken
	cfg.IDToken = ts.IDToken
	// Any cached RP-API token belongs to the previous session; drop it so
	// `kamu orgs` re-mints from the fresh refresh token.
	cfg.RPAPIToken = ""
	cfg.RPAPITokenExpiresAt = time.Time{}
	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	identity := "you"
	if claims, err := kamuid.ParseIDTokenClaims(ts.IDToken); err == nil {
		switch {
		case claims.Email != "":
			identity = claims.Email
		case claims.Name != "":
			identity = claims.Name
		case claims.Subject != "":
			identity = claims.Subject
		}
	}
	fmt.Fprintf(io.ErrOut, "Logged in to kamuid as %s\n", identity)

	// Exchange the KamuID token for a kamuhub access key — the credential products
	// and git actually accept (ADR 0006). The raw KamuID token never leaves here.
	kh := kamuhub.New(cfg.ResolveKamuhubBase())
	res, err := kh.Login(ctx, ts.AccessToken, kamuhub.LoginRequest{Org: f.org, Label: "kamu CLI login"})
	if err != nil {
		return fmt.Errorf("mint access key: %w\n\nkamuid login succeeded but the kamuhub key mint failed; retry `kamu login`", err)
	}

	cfg.AccessKey = res.Token
	cfg.AccessKeyExpiresAt = res.ExpiresAt()
	if cfg.ActiveOrg == "" {
		cfg.ActiveOrg = res.Org.Slug
	}
	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("save access key: %w", err)
	}

	expiry := "never"
	if !res.ExpiresAt().IsZero() {
		expiry = res.ExpiresAt().Local().Format("2006-01-02 15:04")
	}
	fmt.Fprintf(io.ErrOut, "Access key stored for org %q (%d capabilities; expires %s).\n",
		res.Org.Slug, len(res.Org.Grants), expiry)
	fmt.Fprintln(io.ErrOut, "You can now `kamu clone`, and product commands work without KAMU_ACCESS_KEY.")
	return nil
}
