package darwin

// Purpose: running scripts on the node as root, and the option defaults, for the darwin family. The
// Linux families keep the same helpers private to internal/nodes/bootstrap; they are small enough to
// repeat here rather than widen that package.
// Inputs:  bootstrap.Options and a bound remote.Target.
// Outputs: an `elevated` runner that sends one `sh -c` script per step.
// Constraints: the ssh user must be root or have passwordless sudo (`sudo -n`), else E669 no_root; the
//          script text is built from constants and integers only; step errors carry a short,
//          control-free detail (bootstrap.Detail), never the node's raw output.

import (
	"context"
	"strings"

	"github.com/nself-org/cli/sdk/go/v2/remote"
	"github.com/nself-org/plugins/free/ci/internal/nodes/bootstrap"
)

// bind fills the defaults of o and validates it for an Install (needSlots) or an Uninstall.
func bind(o bootstrap.Options, t remote.Target, needSlots bool) (bootstrap.Options, error) {
	o.Target = t
	if needSlots && (o.Slots < 1 || o.Slots > MaxSlots) {
		return o, bootstrap.NewError(bootstrap.ReasonUsage, "--slots must be between 1 and %d on macOS", MaxSlots)
	}
	if o.Run == nil {
		if o.Target.Dest == "" {
			return o, bootstrap.NewError(bootstrap.ReasonUsage, "no node to bootstrap")
		}
		tt := o.Target
		o.Run = func(ctx context.Context, argv ...string) (string, error) { return remote.RunArgv(ctx, tt, argv...) }
	}
	if o.Copy == nil {
		tt := o.Target
		o.Copy = func(ctx context.Context, local, remotePath string) error {
			return remote.CopyTo(ctx, tt, local, remotePath)
		}
	}
	if o.Logf == nil {
		o.Logf = func(string, ...any) {}
	}
	return o, nil
}

// elevated runs a script as root: directly when the ssh user is root, through `sudo -n` otherwise.
type elevated struct {
	o    bootstrap.Options
	root bool
}

func newElevated(ctx context.Context, o bootstrap.Options) (*elevated, error) {
	out, err := o.Run(ctx, "id", "-u")
	if err != nil {
		return nil, bootstrap.NewError(bootstrap.ReasonStep, "cannot reach the node: %s", bootstrap.Detail(err))
	}
	if lastLine(out) == "0" {
		return &elevated{o: o, root: true}, nil
	}
	if _, err = o.Run(ctx, "sudo", "-n", "true"); err != nil {
		return nil, bootstrap.NewError(bootstrap.ReasonNoRoot, "the ssh user is not root and has no passwordless sudo; --system needs one of the two")
	}
	return &elevated{o: o}, nil
}

// sh runs script as root. A failure is an E669 step error with the given reason key.
func (e *elevated) sh(ctx context.Context, reason, step, script string) (string, error) {
	argv := []string{"sh", "-c", script}
	if !e.root {
		argv = append([]string{"sudo", "-n"}, argv...)
	}
	out, err := e.o.Run(ctx, argv...)
	if err != nil {
		return out, bootstrap.NewError(reason, "bootstrap step %s failed: %s", step, bootstrap.Detail(err))
	}
	return out, nil
}

func lastLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.LastIndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[i+1:])
	}
	return s
}
