package registry

// Purpose: keep provider authority out of node capability updates.
// Inputs: reported and stored capabilities. Outputs: a sanitized report.
// Constraints: ordinary user labels and non-hosted location reports remain usable.
import (
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

func nodeReportedCapability(in, old model.Capability) model.Capability {
	if in.Location.Kind.Value != nil && *in.Location.Kind.Value == "hosted" {
		in.Location = old.Location
	}
	labels := make([]string, 0, len(in.Identity.Labels))
	for _, label := range in.Identity.Labels {
		if !strings.HasPrefix(label, "nself.") {
			labels = append(labels, label)
		}
	}
	for _, label := range old.Identity.Labels {
		if strings.HasPrefix(label, "nself.") {
			labels = append(labels, label)
		}
	}
	in.Identity.Labels = labels
	return in
}
