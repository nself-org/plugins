package run

// Purpose: register the pinned toolchain image as the image of `--container`
// (P7-CI-43, cap:ci.toolchain-image).
// Inputs:  NSELF_CI_IMAGE (--image ref@digest), the toolchain-image row of
// versions.yaml, a docker runner through the report funnel.
// Outputs: the image reference jobs run in; E645 while the row is pending.
// Constraints: one file per hook; the resolver lives in internal/toolchain.

import (
	"context"

	"github.com/nself-org/plugins/free/ci/internal/report"
	"github.com/nself-org/plugins/free/ci/internal/toolchain"
)

type toolchainImage struct{ resolver *toolchain.Resolver }

// Image implements ContainerImageHook.
func (t toolchainImage) Image(ctx context.Context, _ AttemptInfo) (string, error) {
	return t.resolver.Resolve(ctx)
}

func init() {
	RegisterImage(toolchainImage{resolver: toolchain.NewResolver(report.Capture)})
}
