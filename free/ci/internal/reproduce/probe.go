package reproduce

// Purpose: the default version probe: a short process through the exec funnel.
// Constraints: the funnel is report.CaptureIn, so this package never imports os/exec.

import (
	"context"

	"github.com/nself-org/plugins/free/ci/internal/report"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
)

// defaultProbe runs the probe against the repository root, the directory the
// gate runner probed in when it recorded the version.
func defaultProbe(root string) cirun.Capture {
	return func(ctx context.Context, argv ...string) (string, int, error) {
		return report.CaptureIn(ctx, root, argv...)
	}
}
