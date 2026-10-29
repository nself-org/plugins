package registry

import (
	"github.com/nself-org/plugins/free/ci/internal/model"
	"strings"
)

// Placement facts come from the operator, regardless of a report's claimed source.
func discoveryPlacement(c model.Capability) model.Capability {
	c.Location = model.CapabilityLocation{Kind: assigned[string](nil), Region: assigned[string](nil)}
	c.Identity.Labels = reportedLabels(c.Identity.Labels, nil)
	return c
}

func preservePlacement(in, old model.Capability) model.Capability {
	in.Location = old.Location
	in.Identity.Labels = reportedLabels(in.Identity.Labels, old.Identity.Labels)
	return in
}

func reportedLabels(reported, assigned []string) []string {
	out := make([]string, 0, len(reported)+len(assigned))
	for _, label := range reported {
		if !strings.HasPrefix(label, "pool.") {
			out = append(out, label)
		}
	}
	for _, label := range assigned {
		if strings.HasPrefix(label, "pool.") {
			out = append(out, label)
		}
	}
	return out
}

func eligible(c model.Capability) bool {
	return c.Separation.UIDSeparation.Value != nil && *c.Separation.UIDSeparation.Value
}
