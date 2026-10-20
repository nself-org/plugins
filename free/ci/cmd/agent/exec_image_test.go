package main

import (
	"context"
	"errors"
	"github.com/nself-org/plugins/free/ci/internal/agent/session"
	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
	"strings"
	"testing"
)

type missingImageHook struct {
	calls *int
	err   error
}

func (h missingImageHook) Image(context.Context, cirun.AttemptInfo) (string, error) {
	*h.calls++
	return "", h.err
}

func TestRaisedJobResolvesImageBeforeExecution(t *testing.T) {
	useState(t)
	missing := errors.New("test image unavailable")
	for _, level := range []model.Isolation{"container", "sandboxed-container"} {
		t.Run(string(level), func(t *testing.T) {
			calls := 0
			old := cirun.Registered.Image
			cirun.RegisterImage(missingImageHook{&calls, missing})
			t.Cleanup(func() { cirun.RegisterImage(old) })
			spec := execSpec{AttemptID: "raised", Job: model.Job{Isolation: level}}
			res, tail, err := execute(context.Background(), spec, jobInputs{})
			if err != nil {
				t.Fatal(err)
			}
			if calls != 1 || !errors.Is(res.Err, missing) || !strings.Contains(tail, "NSELF_CI_IMAGE") {
				t.Fatalf("image resolution: calls=%d err=%v tail=%q", calls, res.Err, tail)
			}
		})
	}
}

type deliveredSpecObserver struct {
	got  *ciexec.JobSpec
	stop error
}

func (o deliveredSpecObserver) Prepare(_ context.Context, spec *ciexec.JobSpec) error {
	*o.got = *spec
	return o.stop
}

type pinnedImageHook struct{}

func (pinnedImageHook) Image(context.Context, cirun.AttemptInfo) (string, error) {
	return "example.invalid/toolchain@sha256:" + strings.Repeat("a", 64), nil
}

func TestAgentReceivesImageAndLeaseNetwork(t *testing.T) {
	useState(t)
	for _, scope := range []model.NetworkScope{"none", "restricted", "internet"} {
		t.Run(string(scope), func(t *testing.T) {
			old := cirun.Registered
			t.Cleanup(func() { cirun.Registered = old })
			var got ciexec.JobSpec
			stop := errors.New("stop before runtime")
			cirun.RegisterIsolation(deliveredSpecObserver{&got, stop})
			cirun.RegisterImage(pinnedImageHook{})
			runner := leaseRunner{coordinator: "test"}
			_, err := runner.Run(context.Background(), session.JobRun{Lease: protocol.Lease{AttemptID: "network", NetworkScope: scope, Checkout: protocol.Checkout{Revision: rev40}, Job: model.Job{Isolation: "container", Run: []string{"true"}}}})
			if err != nil {
				t.Fatal(err)
			}
			if got.Network != scope || got.Job.Container == nil || got.Job.Container.Image == "" || got.Job.Isolation != "container" {
				t.Fatalf("delivered scope=%s image=%v isolation=%s", got.Network, got.Job.Container, got.Job.Isolation)
			}
		})
	}
}
