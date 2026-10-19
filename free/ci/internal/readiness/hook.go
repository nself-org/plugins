package readiness

// Purpose: readiness.cmd, the operator's own "is this machine busy?" script.
// Inputs: the hook argv. Outputs: Hook: exit 0 = ready, exit 3 = not ready with a
// one-line reason on stdout, anything else (other exit code, start failure, timeout)
// = unknown, which is treated as not ready (E800): an operator-owned node never
// guesses "ready" from silence.
// Constraints: argv through the exec funnel, never a shell, minimal environment, 5 s
// timeout, stdout clipped to one redacted line of at most MaxWhyLen characters.

import (
	"context"
	"fmt"
	"strings"
	"unicode"

	"github.com/nself-org/plugins/free/ci/internal/readiness/sample"
	"github.com/nself-org/plugins/free/ci/internal/redact"
)

// HookResult is the mapped outcome of one readiness.cmd run.
type HookResult string

// The three outcomes.
const (
	HookReady    HookResult = "ready"
	HookNotReady HookResult = "not_ready"
	HookUnknown  HookResult = "unknown"
)

// ExitNotReady is the exit status that means "not ready, reason on stdout".
const ExitNotReady = 3

// MaxWhyLen caps the reason line a hook may report.
const MaxWhyLen = 160

// Hook is what Decide needs from a hook run. Ran is false when no hook is configured.
type Hook struct {
	Ran    bool
	Result HookResult
	// Why is the one-line reason (empty when the hook said ready).
	Why string
}

// RunHook runs argv through run and maps the outcome.
func RunHook(ctx context.Context, run sample.Runner, argv []string) Hook {
	if len(argv) == 0 {
		return Hook{}
	}
	res, err := run.Run(ctx, argv, HookTimeout)
	switch {
	case err != nil:
		return Hook{Ran: true, Result: HookUnknown, Why: unknownWhy(fmt.Sprintf("readiness.cmd did not finish: %s", ClipLine(err.Error())))}
	case res.ExitCode == 0:
		return Hook{Ran: true, Result: HookReady}
	case res.ExitCode == ExitNotReady:
		why := ClipLine(res.Stdout)
		if why == "" {
			why = "readiness.cmd reported not ready"
		}
		return Hook{Ran: true, Result: HookNotReady, Why: why}
	}
	return Hook{Ran: true, Result: HookUnknown, Why: unknownWhy(fmt.Sprintf("readiness.cmd exited %d (only 0 and 3 are answers)", res.ExitCode))}
}

func unknownWhy(msg string) string { return "E800 " + msg }

// ClipLine returns the first non-empty line of s with secrets masked, control
// characters dropped and length bounded.
func ClipLine(s string) string {
	line := ""
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			line = l
			break
		}
	}
	f := redact.New().NewFilter()
	line = string(f.Redact([]byte(line))) + string(f.Flush())
	line = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, line)
	if r := []rune(line); len(r) > MaxWhyLen {
		line = string(r[:MaxWhyLen-1]) + "…"
	}
	return line
}
