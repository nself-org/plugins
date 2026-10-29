package pools

// Purpose: pool id syntax and the reading of pool membership from assigned labels.
// Inputs:  pool ids, and a runner's assigned labels in the placement form (`key` or `key=value`).
// Outputs: ValidID, Reserved, FromAssigned.
// Constraints: ids match ^[a-z][a-z0-9-]{0,30}$; `local` and `hosted` are implicit and never come from a
//          label; `pool.` is the only label namespace read. Callers pass assigned labels only: a node's
//          self-declared labels are never membership.

import (
	"regexp"
	"slices"
	"strings"
)

// LabelPrefix is the assigned-label key prefix that makes a runner a pool member.
const LabelPrefix = "pool."

// The implicit pool ids.
const (
	// Local is the pool of this machine's runner.
	Local = "local"
	// Hosted is the pool of every provider runner.
	Hosted = "hosted"
)

var idRE = regexp.MustCompile(`^[a-z][a-z0-9-]{0,30}$`)

// ValidID reports whether id is a well-formed pool id.
func ValidID(id string) bool { return idRE.MatchString(id) }

// Reserved reports whether id is an implicit pool id.
func Reserved(id string) bool { return id == Local || id == Hosted }

// FromAssigned returns the pool ids named by assigned labels, sorted and without duplicates. A label
// outside the `pool.` namespace, with a malformed id or naming an implicit pool is not membership.
func FromAssigned(labels []string) []string {
	var out []string
	for _, l := range labels {
		key, _, _ := strings.Cut(l, "=")
		id, ok := strings.CutPrefix(key, LabelPrefix)
		if !ok || !ValidID(id) || Reserved(id) || slices.Contains(out, id) {
			continue
		}
		out = append(out, id)
	}
	slices.Sort(out)
	return out
}
