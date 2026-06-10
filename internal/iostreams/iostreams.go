package iostreams

import (
	"context"
	"io"
	"os"

	"golang.org/x/term"
)

type IOStreams struct {
	In     io.Reader
	Out    io.Writer
	ErrOut io.Writer

	isStdoutTTY bool
	isStderrTTY bool
	colorEnabled bool
}

func System() *IOStreams {
	stdoutTTY := term.IsTerminal(int(os.Stdout.Fd()))
	stderrTTY := term.IsTerminal(int(os.Stderr.Fd()))
	return &IOStreams{
		In:           os.Stdin,
		Out:          os.Stdout,
		ErrOut:       os.Stderr,
		isStdoutTTY:  stdoutTTY,
		isStderrTTY:  stderrTTY,
		colorEnabled: stdoutTTY && os.Getenv("NO_COLOR") == "" && os.Getenv("KAMU_NO_COLOR") == "",
	}
}

func (s *IOStreams) IsStdoutTTY() bool  { return s.isStdoutTTY }
func (s *IOStreams) IsStderrTTY() bool  { return s.isStderrTTY }
func (s *IOStreams) ColorEnabled() bool { return s.colorEnabled }

type ctxKey struct{}

func NewContext(ctx context.Context, io *IOStreams) context.Context {
	return context.WithValue(ctx, ctxKey{}, io)
}

func FromContext(ctx context.Context) *IOStreams {
	if io, ok := ctx.Value(ctxKey{}).(*IOStreams); ok {
		return io
	}
	return System()
}
