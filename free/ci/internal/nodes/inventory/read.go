package inventory

// Purpose: read an inventory from a file or from the output of a discovery command (P7-AUTO-04).
// Inputs:  a path, or an argv and a Runner (the exec-funnel caller; the CLI passes report.Capture).
// Outputs: the strictly decoded Document.
// Constraints: a file is read with a size cap, never past it; a command runs without a shell through the
//          funnel's allowlisted environment, is cut off after Limits.Timeout (10 s) and refused when its
//          output passes Limits.MaxBytes (1 MiB); a cut-off or oversized command applies nothing; the
//          output is decoded and dropped, never persisted. The funnel merges stderr into its output, so a
//          tool that warns on stderr fails the strict decode: quiet it in the command.

import (
	"context"
	"errors"
	"io"
	"os"
	"time"
)

// Limits bound one read.
type Limits struct {
	Timeout  time.Duration
	MaxBytes int
}

// DefaultLimits are the contract limits: 10 s and 1 MiB.
var DefaultLimits = Limits{Timeout: 10 * time.Second, MaxBytes: 1 << 20}

// Runner runs argv through the exec funnel and returns its output and exit code. err is set only when
// the process could not be run.
type Runner func(ctx context.Context, argv []string) (out string, exit int, err error)

func (l Limits) norm() Limits {
	if l.Timeout <= 0 {
		l.Timeout = DefaultLimits.Timeout
	}
	if l.MaxBytes <= 0 {
		l.MaxBytes = DefaultLimits.MaxBytes
	}
	return l
}

// ReadFile reads and decodes the inventory file at path.
func ReadFile(path string, l Limits) (Document, error) {
	l = l.norm()
	f, err := os.Open(path)
	if err != nil {
		return Document{}, newErr(ReasonReadFailed, "the inventory file cannot be opened")
	}
	defer func() { _ = f.Close() }()
	raw, err := io.ReadAll(io.LimitReader(f, int64(l.MaxBytes)+1))
	if err != nil {
		return Document{}, newErr(ReasonReadFailed, "the inventory file cannot be read")
	}
	if len(raw) > l.MaxBytes {
		return Document{}, newErr(ReasonTooLarge, "the inventory file is over %d bytes", l.MaxBytes)
	}
	return Decode(raw)
}

// ReadCmd runs argv and decodes what it prints.
func ReadCmd(ctx context.Context, run Runner, argv []string, l Limits) (Document, error) {
	l = l.norm()
	if run == nil || len(argv) == 0 || argv[0] == "" {
		return Document{}, newErr(ReasonNoRunner, "--cmd needs a command")
	}
	ctx, cancel := context.WithTimeout(ctx, l.Timeout)
	defer cancel()
	out, exit, err := run(ctx, argv)
	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		return Document{}, newErr(ReasonTimeout, "the command did not finish within %s", l.Timeout)
	case err != nil:
		return Document{}, newErr(ReasonCmdFailed, "the command could not be run")
	case len(out) > l.MaxBytes:
		return Document{}, newErr(ReasonTooLarge, "the command printed more than %d bytes", l.MaxBytes)
	case exit != 0:
		return Document{}, newErr(ReasonCmdFailed, "the command exited with status %d", exit)
	}
	return Decode([]byte(out))
}
