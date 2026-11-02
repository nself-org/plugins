package run

// Purpose: preserve invocation snapshots during cancellation-independent cleanup.
// Inputs: the run context, or nil for standalone finish callers.
// Outputs: a bounded cleanup context and its release function.
// Constraints: cancellation never skips evidence, status or hygiene finalization.

import (
	"context"
	"strings"
)

func firstLineOf(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return s
}

func finishBackground(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	return background(context.WithoutCancel(ctx))
}
