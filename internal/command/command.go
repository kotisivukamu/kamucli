package command

import (
	"context"

	"github.com/spf13/cobra"
)

type Runner func(ctx context.Context, args []string) error

type Preparer func(ctx context.Context) (context.Context, error)

func New(use, short, long string, runFn Runner, preparers ...Preparer) *cobra.Command {
	cmd := &cobra.Command{
		Use:           use,
		Short:         short,
		Long:          long,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	if runFn != nil {
		cmd.RunE = func(c *cobra.Command, args []string) error {
			ctx := c.Context()
			for _, p := range preparers {
				next, err := p(ctx)
				if err != nil {
					return err
				}
				ctx = next
			}
			return runFn(ctx, args)
		}
	}
	return cmd
}
