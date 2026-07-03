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
)

const (
	// EnvAccessKey holds a kamuhub access key (a scoped, signed platform
	// context), same as `kamu sites`/`kamu dns`. This is the only credential
	// `kamu status` accepts: kamustatus is now reached through the kamuhub BFF,
	// which requires the signed platform context.
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

// client resolves the kamuhub access key (--key then env) and builds a
// kamustatus client pointed at the BFF, exactly like `kamu sites`/`kamu dns`.
// The raw KamuID access token from `kamu auth login` is deliberately NOT
// accepted: kamustatus (a resource server) now requires the BFF-signed
// X-Kamuhub-Authz context, which only the access-key path through the front
// door carries.
func client() (*kamustatus.Client, error) {
	key := keyFlag
	if key == "" {
		key = os.Getenv(EnvAccessKey)
	}
	if key == "" {
		return nil, errors.New("no access key. Create one in the dashboard (Manage -> Access keys) and pass it:\n\n    export " + EnvAccessKey + "=...\n\nor --key <token>")
	}
	return kamustatus.New(os.Getenv(EnvURL), key), nil
}

// ctxOrTodo guards against a nil context from the run path; cobra normally
// supplies one but the type allows nil.
func ctxOrTodo(ctx context.Context) context.Context {
	if ctx == nil {
		return context.TODO()
	}
	return ctx
}
