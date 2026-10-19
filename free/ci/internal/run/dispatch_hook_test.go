package run

import (
	"context"
	"sync/atomic"
	"testing"

	compat "github.com/nself-org/cli/sdk/go/v2/compat"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

func dispatcherCount() (n int) {
	runDispatchers.Range(func(_, _ any) bool { n++; return true })
	return n
}

// TestRunPanicClosesTheDispatcher: a panic between the jobs and the conclusion (here
// the git capture of the closing revision) skips the AfterRun hooks; the run's
// dispatcher must still be closed and forgotten (D-1044).
func TestRunPanicClosesTheDispatcher(t *testing.T) {
	t.Setenv(compat.EnvVar, "1")
	prev := Dispatch
	t.Cleanup(func() { Dispatch = prev })
	var armed atomic.Bool
	Dispatch = func(ctx context.Context, s AttemptSpec) (Admitted, error) {
		a, err := dispatchHook(ctx, s)
		armed.Store(true)
		return a, err
	}
	before := dispatcherCount()
	r := newRig(t, map[string]model.Job{"a": sh("true")}, gitFake{})
	base := r.C.Capture
	r.C.Capture = func(ctx context.Context, argv ...string) (string, int, error) {
		if armed.Load() {
			panic("git exploded")
		}
		return base(ctx, argv...)
	}
	func() {
		defer func() {
			if recover() == nil {
				t.Error("Run did not panic: the test no longer exercises the pre-AfterRun path")
			}
		}()
		_, _ = r.C.Run(context.Background())
	}()
	if got := dispatcherCount(); got != before {
		t.Fatalf("%d dispatcher(s) left in runDispatchers after a run that never reached AfterRun", got-before)
	}
}
