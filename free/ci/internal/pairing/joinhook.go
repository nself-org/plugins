package pairing

// Purpose: the join hook registry (review F6): P7-NODE-12 and later tickets hang work on a completed
//          `nself-ci-agent join` (for example, starting discovery advertising) without editing join.
// Inputs:  RegisterJoinHook calls from init.
// Outputs: RunJoinHooks, called by cmd/agent/join.go after the enrollment is saved.
// Constraints: a name registers once; hooks run in registration order; the first error stops the run
//          and is reported by join (the enrollment itself is already saved).

import (
	"context"
	"fmt"
	"sync"

	"github.com/nself-org/plugins/free/ci/internal/pairing/enroll"
)

// JoinHook runs after a node joined; state is the agent state dir.
type JoinHook func(ctx context.Context, r enroll.JoinResult, state string) error

type namedHook struct {
	name string
	fn   JoinHook
}

var (
	hookMu    sync.Mutex
	joinHooks []namedHook
)

// RegisterJoinHook adds a hook. An empty name, a nil function or a duplicate name is an error.
func RegisterJoinHook(name string, fn JoinHook) error {
	hookMu.Lock()
	defer hookMu.Unlock()
	if name == "" || fn == nil {
		return fmt.Errorf("join hook needs a name and a function")
	}
	for _, h := range joinHooks {
		if h.name == name {
			return fmt.Errorf("duplicate join hook %q", name)
		}
	}
	joinHooks = append(joinHooks, namedHook{name, fn})
	return nil
}

// RunJoinHooks runs every hook in order and stops at the first error.
func RunJoinHooks(ctx context.Context, r enroll.JoinResult, state string) error {
	hookMu.Lock()
	hooks := append([]namedHook(nil), joinHooks...)
	hookMu.Unlock()
	for _, h := range hooks {
		if err := h.fn(ctx, r, state); err != nil {
			return fmt.Errorf("join hook %s: %w", h.name, err)
		}
	}
	return nil
}
