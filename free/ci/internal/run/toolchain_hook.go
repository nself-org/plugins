package run

// Purpose: register the pinned toolchain image as the image of `--container`
// (P7-CI-43, cap:ci.toolchain-image).
// Inputs:  NSELF_CI_IMAGE (--image ref@digest), the toolchain-image row of
// versions.yaml, docker through the exec funnel.
// Outputs: the image reference jobs run in; E645 while the row is pending.
// Constraints: one file per hook; the resolver lives in internal/toolchain; a
// pull of the multi-gigabyte image gets its own long timeout (report.Capture
// stops a process after 30 s), a local image inspect stays short.

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"time"

	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/toolchain"
)

const (
	pullTimeout    = 20 * time.Minute
	inspectTimeout = 30 * time.Second
)

var toolchainCaptureSeq atomic.Int64

type toolchainImage struct{ resolver *toolchain.Resolver }

// Image implements ContainerImageHook.
func (t toolchainImage) Image(ctx context.Context, _ AttemptInfo) (string, error) {
	return t.resolver.Resolve(ctx)
}

// dockerTimeout gives a pull (a multi-gigabyte download) the long budget and everything else the short one.
func dockerTimeout(argv []string) time.Duration {
	for _, a := range argv[min(1, len(argv)):] {
		if a == "pull" {
			return pullTimeout
		}
		if !strings.HasPrefix(a, "-") {
			break
		}
	}
	return inspectTimeout
}

// dockerCapture runs a docker command through the exec funnel: output merged,
// a non-zero exit is not an error, err only when the process could not run.
func dockerCapture(ctx context.Context, argv ...string) (string, int, error) {
	timeout := dockerTimeout(argv)
	home, err := os.MkdirTemp("", "nself-ci-img-")
	if err != nil {
		return "", -1, err
	}
	defer func() { _ = os.RemoveAll(home) }()
	var buf strings.Builder
	ex := ciexec.Executor{Isolation: ciexec.BaselineIsolator{}}
	res := ex.Run(ctx, ciexec.JobSpec{
		AttemptID: fmt.Sprintf("img-%d-%d", time.Now().UnixNano(), toolchainCaptureSeq.Add(1)), CoordinatorID: "capture",
		Job: model.Job{Kind: "static"}, Home: home, Timeout: timeout, Command: argv,
		Output: func(b []byte) { buf.Write(b) },
	})
	return buf.String(), res.ExitCode, res.Err
}

func init() {
	RegisterImage(toolchainImage{resolver: toolchain.NewResolver(dockerCapture)})
}
