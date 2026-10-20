package update

// Purpose: the output side of `nodes upgrade`: the line per node, the note for a node an interrupted run left drained, and the exit
//          class of a failure.
// Inputs:  Result, the registry, an error.
// Outputs: text lines, the v1 JSON error envelope, an exit code (1 refused or usage, 2 infrastructure, 3 host key or revoked).
// Constraints: an error message is printed as it is (its text holds ids and reason text only, never a key or command output).

import (
	"context"
	"fmt"
	"io"

	"github.com/nself-org/cli/sdk/go/v2/output"
	"github.com/nself-org/plugins/free/ci/internal/nodes/lifecycle"
)

// noopNote names a node that an interrupted upgrade left drained when the re-run has nothing to install: the run did not put it
// back, because nothing proved the node healthy.
func noopNote(ctx context.Context, d *lifecycle.Deps, r Result) string {
	if !r.Noop {
		return ""
	}
	if n, err := d.Registry.Get(ctx, r.Node); err == nil && n.Record.State == "draining" && n.Record.Reason == DrainReason {
		return fmt.Sprintf("node %s is still drained from an interrupted upgrade; run `nself ci nodes test %s`, then `nself ci nodes enable %s`", r.Node, r.Node, r.Node)
	}
	return ""
}

func describe(r Result) string {
	switch {
	case r.Noop && r.Note != "":
		return fmt.Sprintf("node %s already runs %s\n%s", r.Node, r.To, r.Note)
	case r.Noop:
		return fmt.Sprintf("node %s already runs %s", r.Node, r.To)
	case r.Direction == Downgrade:
		return fmt.Sprintf("node %s downgraded %s -> %s (audited)", r.Node, r.From, r.To)
	}
	return fmt.Sprintf("node %s upgraded %s -> %s", r.Node, r.From, r.To)
}

func usageErr(format string, a ...any) error { return fmt.Errorf("E651: "+format, a...) }

// failOut prints the error and returns its exit class: E666 and usage are the caller's (1), host key and revoked are auth (3),
// everything else is infrastructure (2).
func failOut(out, errOut io.Writer, asJSON bool, err error) int {
	code := lifecycle.CodeOf(err)
	exit := output.ExitInfra
	switch code {
	case CodeRefused, "E651", "E650":
		exit = output.ExitUser
	case "E655", "E657":
		exit = output.ExitAuth
	}
	if asJSON {
		if code == "" {
			code = "E607"
		}
		_ = output.WriteError(out, "ci nodes upgrade", output.ErrorDetail{Code: code, Message: err.Error(), ExitCode: exit}, nil)
		return exit
	}
	_, _ = io.WriteString(errOut, err.Error()+"\n")
	return exit
}
