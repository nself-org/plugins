package reproduce

// Purpose: the default version probe: a short process through the exec funnel.
// Constraints: the funnel is report.Capture, so this package never imports os/exec.

import (
	"context"

	"github.com/nself-org/plugins/free/ci/internal/report"
)

func defaultProbe(ctx context.Context, argv ...string) (string, int, error) {
	return report.Capture(ctx, argv...)
}
