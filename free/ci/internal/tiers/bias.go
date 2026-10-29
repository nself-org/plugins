package tiers

// Purpose: the data side of the `auto` preset: the effective tier order, the
// rank of a runner inside its tier and World.Bias.
// Inputs:  a sched.World (Policy.Preset, Policy.Trust, Runners, Hosted).
// Outputs: Order, Rank, Bias and the world contributor.
// Constraints: pure and deterministic (no clock, no map order in any output);
// a rank is clamped below sched.TierStepMs so it never crosses into the next
// tier; the effective list keeps the order it was written in, and a lower scope
// can only drop entries (TightenSubset); a tier the order leaves out is tried
// after every listed tier (the order is a preference: keeping a tier out is the
// overrides' job, local_only, lan_only, private_only, providers.deny); not
// `auto` means no output at all.

import (
	"context"
	"slices"
	"strconv"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

// RankLabel is the label prefix `ci nodes sync` and `ci nodes label` write.
const RankLabel = "rank="

// Order returns the effective tier order of eff, best first: the merged
// sched.auto.order without unknown or repeated entries, or DefaultOrder when the
// key is absent.
func Order(eff trust.Effective) []string {
	v, _, ok := eff.Value(KeyOrder)
	if !ok {
		return DefaultOrder()
	}
	var raw []string
	switch xs := v.(type) {
	case []string:
		raw = xs
	case []any:
		for _, x := range xs {
			if s, ok := x.(string); ok {
				raw = append(raw, s)
			}
		}
	}
	out := []string{}
	for _, s := range raw {
		if slices.Contains(DefaultOrder(), s) && !slices.Contains(out, s) {
			out = append(out, s)
		}
	}
	return out
}

// Rank returns the runner's rank inside its tier: its `rank=<n>` label when it
// has one, else position, its index in the registry list it came from.
func Rank(r sched.Runner, position int) int64 {
	n := int64(position)
	for _, l := range r.Capability.Identity.Labels {
		if v, ok := strings.CutPrefix(l, RankLabel); ok {
			if i, err := strconv.ParseInt(v, 10, 64); err == nil && i >= 0 {
				n = i
				break
			}
		}
	}
	if n < 0 {
		n = 0
	}
	if n >= sched.TierStepMs {
		n = sched.TierStepMs - 1
	}
	return n
}

// ranked calls fn for every runner of w with its tier and rank.
func ranked(w sched.World, fn func(r sched.Runner, tier string, rank int64)) {
	for i, r := range w.Runners {
		fn(r, sched.TierOf(w, r), Rank(r, i))
	}
	for i, r := range w.Hosted {
		fn(r, sched.TierOf(w, r), Rank(r, i))
	}
}

// Bias returns tier index x sched.TierStepMs + rank per runner id; a runner
// whose tier is not in order takes the index len(order), after every listed
// tier. A runner in the best tier with rank 0 gets 0, which the scorer does not show.
func Bias(w sched.World, order []string) map[string]int64 {
	out := map[string]int64{}
	ranked(w, func(r sched.Runner, tier string, rank int64) {
		i := slices.Index(order, tier)
		if i < 0 {
			i = len(order)
		}
		b, _ := sched.Mul(int64(i), sched.TierStepMs)
		out[r.ID], _ = sched.Add(b, rank)
	})
	return out
}

func contribute(_ context.Context, w *sched.World) error {
	if w.Policy.Preset != sched.AutoPreset {
		return nil
	}
	if b := Bias(*w, Order(w.Policy.Trust)); len(b) > 0 {
		w.Bias = b
	}
	return nil
}
