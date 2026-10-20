package burst

// Purpose: the two small helpers the teardown and the sweeper share: matching a
// machine against the deploy-host exclusion set and sorting map keys.
// Inputs:  a deployguard.Set and a Resource. Outputs: a bool, a sorted list.
// Constraints: a machine matches by its addresses and its name; the set is
// the NODE-14 export, read through Options.Exclusion at the moment of use.

import (
	"sort"

	"github.com/nself-org/plugins/free/ci/internal/nodes/deployguard"
)

func excluded(s deployguard.Set, r Resource) bool {
	return s.Match(deployguard.Facts{Addrs: r.Addrs, Names: []string{r.Name}}).Matched
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
