package version

import (
	"context"
	"fmt"
	"runtime"
	"runtime/debug"

	"github.com/spf13/cobra"

	"github.com/kotisivukamu/kamucli/internal/command"
	"github.com/kotisivukamu/kamucli/internal/iostreams"
)

func New(v, commit, date string) *cobra.Command {
	return command.New("version", "Print version info", "", func(ctx context.Context, _ []string) error {
		io := iostreams.FromContext(ctx)
		ver, c, d := resolve(v, commit, date)
		fmt.Fprintf(io.Out, "kamu %s\n", ver)
		if c != "" {
			fmt.Fprintf(io.Out, "  commit: %s\n", c)
		}
		if d != "" {
			fmt.Fprintf(io.Out, "  date:   %s\n", d)
		}
		fmt.Fprintf(io.Out, "  go:     %s %s/%s\n", runtime.Version(), runtime.GOOS, runtime.GOARCH)
		return nil
	})
}

func resolve(v, commit, date string) (string, string, string) {
	if v != "" && v != "dev" {
		return v, commit, date
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return v, commit, date
	}
	out := v
	if info.Main.Version != "" && info.Main.Version != "(devel)" {
		out = info.Main.Version
	}
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			if commit == "" {
				commit = s.Value
			}
		case "vcs.time":
			if date == "" {
				date = s.Value
			}
		}
	}
	return out, commit, date
}
