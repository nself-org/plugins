package inventory

// Purpose: the assigned label set an entry stands for: its plain labels, one `pool.<id>` per pool and
//          `rank=<n>`; the tier rank P7-AUTO-05 reads.
// Inputs:  an Entry.
// Outputs: the labels sorted by key.
// Constraints: sync only sets these keys; it never removes a label the operator set another way.

import (
	"sort"
	"strconv"

	"github.com/nself-org/plugins/free/ci/internal/nodes/labels"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// RankKey is the label that carries the tier rank.
const RankKey = "rank"

// PoolPrefix is the label key prefix of pool membership.
const PoolPrefix = "pool."

// Want returns the labels the entry stands for.
func (e Entry) Want() ([]store.NodeLabel, error) {
	in := append([]string{}, e.Labels...)
	for _, p := range e.Pools {
		in = append(in, PoolPrefix+p)
	}
	if e.Rank != nil {
		in = append(in, RankKey+"="+strconv.Itoa(*e.Rank))
	}
	set, err := labels.Parse(in)
	if err != nil {
		return nil, err
	}
	sort.Slice(set, func(i, j int) bool { return set[i].Key < set[j].Key })
	return set, nil
}
