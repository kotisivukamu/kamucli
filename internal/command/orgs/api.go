package orgs

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/kotisivukamu/kamucli/internal/client/kamuid"
	"github.com/kotisivukamu/kamucli/internal/config"
	"github.com/kotisivukamu/kamucli/internal/iostreams"
)

// relogin is appended to auth failures whose fix is a fresh `kamu login`.
const relogin = "run `kamu login` again"

// rpAPIToken returns a bearer for kamuid's /v1/rp resource server, minting one
// via the refresh-token grant (resource = the RP API audience) when the cached
// token is missing or about to expire. The grant rotates the refresh token, so
// the config is re-saved with the replacement.
func rpAPIToken(ctx context.Context, cfg *config.Config) (string, error) {
	if cfg.RPAPIToken != "" && !cfg.RPAPITokenExpiresAt.IsZero() &&
		time.Until(cfg.RPAPITokenExpiresAt) > time.Minute {
		return cfg.RPAPIToken, nil
	}
	if cfg.RefreshToken == "" {
		return "", errors.New(`not logged in — run "kamu login"`)
	}

	client := kamuid.New(cfg.ResolveIssuer())
	// Request the FULL login scope set, not just the org scopes: kamuid rotates
	// the refresh token and the rotated one carries exactly the scopes requested
	// here, so narrowing would permanently narrow the stored grant. Requesting
	// them explicitly also detects stale logins — a refresh token consented
	// before the kamu.org.* scopes existed fails with invalid_scope.
	ts, err := client.Refresh(ctx, kamuid.RefreshRequest{
		ClientID:     cfg.ResolveClientID(),
		RefreshToken: cfg.RefreshToken,
		Scope:        config.DefaultScopes,
		Resource:     cfg.ResolveRPAPIAudience(),
	})
	if err != nil {
		switch {
		case kamuid.IsOAuthCode(err, "invalid_scope"):
			return "", fmt.Errorf("%w\n\nYour stored login predates the org-management scopes — %s", err, relogin)
		case kamuid.IsOAuthCode(err, "invalid_grant"):
			return "", fmt.Errorf("%w\n\nYour session has expired — %s", err, relogin)
		default:
			return "", fmt.Errorf("mint kamuid API token: %w", err)
		}
	}

	if ts.RefreshToken != "" {
		cfg.RefreshToken = ts.RefreshToken
	}
	if ts.IDToken != "" {
		cfg.IDToken = ts.IDToken
	}
	cfg.RPAPIToken = ts.AccessToken
	cfg.RPAPITokenExpiresAt = ts.AccessExpiresAt()
	if err := config.Save(cfg); err != nil {
		return "", fmt.Errorf("save config: %w", err)
	}
	return ts.AccessToken, nil
}

// api loads the config and returns an authenticated kamuid org client.
func api(ctx context.Context) (*kamuid.OrgAPI, *config.Config, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, nil, err
	}
	token, err := rpAPIToken(ctx, cfg)
	if err != nil {
		return nil, nil, err
	}
	return kamuid.NewOrgAPI(cfg.ResolveIssuer(), token), cfg, nil
}

// loadOrgs returns the caller's organizations from kamuid's live list
// endpoint, falling back to the id_token claim (with a note on stderr) when the
// API is unreachable — so list/switch still work offline or against a kamuid
// without the /v1/rp org endpoints.
func loadOrgs(ctx context.Context, io *iostreams.IOStreams, cfg *config.Config) ([]kamuid.Organization, error) {
	if cfg.RefreshToken == "" && cfg.IDToken == "" {
		return nil, errors.New(`not logged in — run "kamu login"`)
	}
	token, apiErr := rpAPIToken(ctx, cfg)
	if apiErr == nil {
		orgs, err := kamuid.NewOrgAPI(cfg.ResolveIssuer(), token).ListOrgs(ctx)
		if err == nil {
			return orgs, nil
		}
		apiErr = friendly(err)
	}
	if cfg.IDToken == "" {
		return nil, apiErr
	}
	claims, err := kamuid.ParseIDTokenClaims(cfg.IDToken)
	if err != nil {
		return nil, apiErr
	}
	fmt.Fprintf(io.ErrOut, "note: live org list unavailable (%v); showing organizations from the cached login token.\n", apiErr)
	return claims.Organizations, nil
}

// friendly rewraps kamuid API errors whose fix is a re-login with a clear hint.
func friendly(err error) error {
	var ae *kamuid.APIError
	if errors.As(err, &ae) {
		switch {
		case ae.Status == http.StatusForbidden && ae.Code == "insufficient_scope":
			return fmt.Errorf("%w\n\nYour login is missing the org-management scopes — %s", err, relogin)
		case ae.Status == http.StatusUnauthorized:
			return fmt.Errorf("%w\n\nToken rejected — %s", err, relogin)
		}
	}
	return err
}

// matchOrg finds arg among orgs by slug or id (slug match is case-insensitive).
func matchOrg(orgs []kamuid.Organization, arg string) *kamuid.Organization {
	want := strings.ToLower(strings.TrimSpace(arg))
	for i := range orgs {
		if orgs[i].ID == arg || strings.ToLower(orgs[i].Slug) == want {
			return &orgs[i]
		}
	}
	return nil
}

// slugs returns the sorted slug list, for "available: ..." error messages.
func slugs(orgs []kamuid.Organization) []string {
	out := make([]string, 0, len(orgs))
	for _, o := range orgs {
		out = append(out, o.Slug)
	}
	sort.Strings(out)
	return out
}

// resolveOrg maps a slug-or-id (or, when empty, the active org) to one of the
// caller's organizations via the live list endpoint.
func resolveOrg(ctx context.Context, c *kamuid.OrgAPI, cfg *config.Config, arg string) (*kamuid.Organization, error) {
	target := strings.TrimSpace(arg)
	if target == "" {
		target = cfg.ActiveOrg
	}
	if target == "" {
		return nil, errors.New("no organization given and no active org set — pass a slug or run `kamu orgs switch <slug>`")
	}
	orgs, err := c.ListOrgs(ctx)
	if err != nil {
		return nil, friendly(err)
	}
	match := matchOrg(orgs, target)
	if match == nil {
		if len(orgs) == 0 {
			return nil, errors.New("you are not a member of any organization")
		}
		return nil, fmt.Errorf("no organization matches %q; available: %s", target, strings.Join(slugs(orgs), ", "))
	}
	return match, nil
}
