package plan

// Purpose: matrix expansion from the declared support policy.
// Inputs:  `support:` (default: the host platform) and a job's
// matrix.platform ("support" or a list of platforms).
// Outputs: the platform of every expanded entry, sorted; a job without a
// matrix has one entry with an empty platform (it runs on the host).
// Constraints: a matrix never adds a platform the support policy does not
// declare; listed platforms outside the policy are dropped.

import (
	"sort"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// supportPolicy is the declared platform list, sorted and de-duplicated.
func supportPolicy(cfg model.PipelineConfig, w World) []string {
	list := cfg.Support
	if len(list) == 0 && w.Platform != "" {
		list = []string{w.Platform}
	}
	return sortedUnique(list)
}

// expandMatrix returns the entries of one job. empty reports a matrix whose
// platforms are all outside the support policy: the job keeps one
// unplaceable entry so it is never silently dropped.
func expandMatrix(j model.Job, support []string) (entries []string, empty bool) {
	if j.Matrix == nil {
		return []string{""}, false
	}
	var want []string
	switch v := j.Matrix.Platform.(type) {
	case string:
		if v == "support" {
			want = support
		} else {
			want = []string{v}
		}
	case []string:
		want = v
	case []any:
		for _, x := range v {
			if s, ok := x.(string); ok {
				want = append(want, s)
			}
		}
	}
	allowed := toSet(support)
	for _, p := range sortedUnique(want) {
		if allowed[p] {
			entries = append(entries, p)
		}
	}
	if len(entries) == 0 {
		first := ""
		if u := sortedUnique(want); len(u) > 0 {
			first = u[0]
		}
		return []string{first}, true
	}
	return entries, false
}

func sortedUnique(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

func toSet(list []string) map[string]bool {
	m := make(map[string]bool, len(list))
	for _, s := range list {
		m[s] = true
	}
	return m
}
