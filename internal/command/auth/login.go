package auth

import (
	"context"
	"fmt"

	"github.com/cli/browser"
	"github.com/spf13/cobra"

	"github.com/kotisivukamu/kamu-cli/internal/client/kamuid"
	"github.com/kotisivukamu/kamu-cli/internal/command"
	"github.com/kotisivukamu/kamu-cli/internal/config"
	"github.com/kotisivukamu/kamu-cli/internal/iostreams"
)

const defaultScope = "openid profile email offline_access"

type loginFlags struct {
	scope     string
	noBrowser bool
}

func newLogin() *cobra.Command {
	var f loginFlags
	cmd := command.New("login", "Log in to kamuid via the device authorization flow", "",
		func(ctx context.Context, _ []string) error { return runLogin(ctx, &f) })
	cmd.Flags().StringVar(&f.scope, "scope", defaultScope, "OAuth scopes to request")
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

	ts, err := client.PollDeviceToken(ctx, clientID, da)
	if err != nil {
		return fmt.Errorf("device token: %w", err)
	}

	cfg.ClientID = clientID
	cfg.Endpoints.Kamuid = issuer
	cfg.AccessToken = ts.AccessToken
	cfg.RefreshToken = ts.RefreshToken
	cfg.IDToken = ts.IDToken
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
	fmt.Fprintf(io.ErrOut, "Logged in as %s\n", identity)
	return nil
}
