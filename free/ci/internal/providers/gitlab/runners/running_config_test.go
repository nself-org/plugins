package runners_test

// Purpose: running hooks tolerate absent configuration but preserve real configuration failures.
// Inputs: registrar and policy sources. Outputs: decline or the original error.
// Constraints: no sockets or runner mutations.

import (
	"context"
	"errors"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/providers/gitlab/runners"
	"github.com/nself-org/plugins/free/ci/internal/providers/runtime"
)

func TestHandleRunningConfiguration(t *testing.T) {
	for _, mode := range []string{"absent", "nil registrar", "registrar error", "policy error"} {
		t.Run(mode, func(t *testing.T) {
			runners.UseRegistrar(nil)
			runtime.UsePolicy(nil)
			t.Cleanup(func() {
				runners.UseRegistrar(nil)
				runtime.UsePolicy(nil)
			})
			sentinel := errors.New("configuration unavailable")
			switch mode {
			case "nil registrar":
				runners.UseRegistrar(func(context.Context) (*runners.Registrar, error) { return nil, nil })
			case "registrar error":
				runners.UseRegistrar(func(context.Context) (*runners.Registrar, error) { return nil, sentinel })
			case "policy error":
				runtime.UsePolicy(func(context.Context) (*policy.Effective, error) { return nil, sentinel })
			}
			paused, err := runners.HandleRunning(context.Background(), runners.RunningEvent{Project: "acme/app", JobID: 1, RunnerID: 42})
			if paused {
				t.Fatal("configuration check paused a runner")
			}
			switch mode {
			case "registrar error":
				if !errors.Is(err, sentinel) {
					t.Fatalf("lost registrar error: %v", err)
				}
			case "policy error":
				if code(err) != "E700" {
					t.Fatalf("lost policy error: %v", err)
				}
			default:
				if err != nil {
					t.Fatalf("unconfigured runner must decline: %v", err)
				}
			}
		})
	}
}
