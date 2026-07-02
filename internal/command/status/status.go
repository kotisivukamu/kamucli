// Package status implements `kamu status` — monitor projects, monitors, alerts,
// and public status pages on kamustatus (https://github.com/kontakto-fi/kamustatus).
package status

import (
	"context"
	"errors"
	"os"

	"github.com/spf13/cobra"

	"github.com/kotisivukamu/kamucli/internal/client/kamustatus"
	"github.com/kotisivukamu/kamucli/internal/command"
	"github.com/kotisivukamu/kamucli/internal/config"
)

const (
	// EnvAccessKey holds a kamuhub access key (a scoped, signed platform
	// context), same as `kamu sites`. Takes precedence over the login token.
	EnvAccessKey = "KAMU_ACCESS_KEY"
	EnvURL       = "KAMU_KAMUSTATUS_URL"
)

// keyFlag is the --key persistent flag value (a kamuhub access key), shared by
// every `kamu status` subcommand, mirroring `kamu sites`/`kamu dns`.
var keyFlag string

func New() *cobra.Command {
	cmd := command.New("status", "Manage kamustatus monitors and status pages", "", nil)
	cmd.PersistentFlags().StringVar(&keyFlag, "key", "", "kamuhub access key (or "+EnvAccessKey+")")
	cmd.AddCommand(
		newProjects(),
		newMonitors(),
		newAlerts(),
		newPage(),
	)
	return cmd
}

// client resolves config + auth and returns a ready kamustatus client.
func client() (*kamustatus.Client, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}

	baseURL := os.Getenv(EnvURL)
	if baseURL == "" {
		baseURL = cfg.Endpoints.Kamustatus
	}

	// Unified platform identity: --key / KAMU_ACCESS_KEY (a kamuhub access key,
	// same as `kamu sites`/`kamu dns`), else the KamuID access token from
	// `kamu auth login`. kamustatus is a resource server and accepts either.
	token := keyFlag
	if token == "" {
		token = os.Getenv(EnvAccessKey)
	}
	if token == "" {
		token = cfg.AccessToken
	}
	if token == "" {
		return nil, errors.New(`not authenticated.

Run the unified login:

    kamu auth login

or present a kamuhub access key (Manage -> Access keys in the dashboard):

    export ` + EnvAccessKey + `=...`)
	}
	return kamustatus.New(baseURL, token), nil
}

// ctxOrTodo guards against a nil context from the run path; cobra normally
// supplies one but the type allows nil.
func ctxOrTodo(ctx context.Context) context.Context {
	if ctx == nil {
		return context.TODO()
	}
	return ctx
}
