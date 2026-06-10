package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/kotisivukamu/kamu-cli/internal/command/root"
	"github.com/kotisivukamu/kamu-cli/internal/iostreams"
)

var (
	version = "dev"
	commit  = ""
	date    = ""
)

func main() {
	os.Exit(run())
}

func run() int {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	io := iostreams.System()
	ctx = iostreams.NewContext(ctx, io)

	cmd := root.New(root.BuildInfo{Version: version, Commit: commit, Date: date})
	cmd.SetIn(io.In)
	cmd.SetOut(io.Out)
	cmd.SetErr(io.ErrOut)

	if err := cmd.ExecuteContext(ctx); err != nil {
		if errors.Is(err, context.Canceled) {
			return 130
		}
		fmt.Fprintln(io.ErrOut, "Error:", err)
		return 1
	}
	return 0
}
