package registry

import (
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// PolicyActorPrefix marks a coordinator-side policy actor (the burst provider is "policy:burst"). Only such an
// actor may set or clear a node's hosted location or its nself.* labels; a node report may not (D-1402), because
// a hosted runner skips the AuthorizedProjects check in sched eligibility.
const PolicyActorPrefix = "policy:"

func isHosted(f model.Fact[string]) bool { return f.Value != nil && *f.Value == "hosted" }

// keepHostedKind returns the reported location kind, except that a report cannot make a node hosted or stop it
// being hosted: then the stored kind stays.
func keepHostedKind(old, reported model.Fact[string]) model.Fact[string] {
	if isHosted(old) != isHosted(reported) {
		return old
	}
	return reported
}

// keepProviderLabels returns the reported labels with the nself.* ones replaced by the stored nself.* ones.
func keepProviderLabels(old, reported []string) []string {
	const prefix = "nself."
	var out []string
	for _, l := range reported {
		if !strings.HasPrefix(l, prefix) {
			out = append(out, l)
		}
	}
	for _, l := range old {
		if strings.HasPrefix(l, prefix) {
			out = append(out, l)
		}
	}
	if reported == nil && out == nil {
		return nil
	}
	return out
}

func isolationRank(i model.Isolation) int {
	for n, v := range model.EnumValues("Isolation") {
		if v == string(i) {
			return n
		}
	}
	return -1
}
