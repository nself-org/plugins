package main

// Purpose: the `--where local|<node>|<pool>|hosted|auto|all` flag of bare
// `nself ci` and of `ci runs rerun` (P7-AUTO-06). One flag selects the target
// through the run scope of P7-AUTO-29; internal/where maps the value, this file
// reads what the operator registered and merges the result into the run's
// policy values. It is never a second placement path: sched.Place and
// dispatch.Admit decide, with these values in the revision scope.
// Inputs: the flag text; the registered nodes, pools and providers (the store).
// Outputs: RunOptions.PolicyRun gains the mapped keys; an unknown value or
// `all` before a race hook exists is E797 and exit 2.
// Constraints: V15 only; an explicit --prefer or --runner wins over the same
// key from --where; with no flag the run is `auto` when a non-local node or a
// provider is registered and today's local run otherwise, never a hidden
// constant (auto writes no value: the operator's sched.preset stands).

import (
	"context"
	"encoding/json"
	"flag"

	"github.com/nself-org/plugins/free/ci/internal/pools"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/where"
)

// whereValue is the --where text of the current invocation; tests reset it.
var whereValue string

// whereKnown reads what the operator registered; a seam for tests.
var whereKnown = productionWhereKnown

// whereChosen is the target the last invocation resolved, for the placement report.
var whereChosen where.Target

const whereHelp = "run target: local, a node id, a pool id, hosted, auto or all (default: auto when a node or provider is registered, else local)"

func init() {
	bareFlags.Func("where", whereHelp, func(v string) error {
		if v == "" {
			return &where.Error{Message: "--where needs a value"}
		}
		whereValue = v
		return nil
	})
	RegisterOptionsHook(whereOptions)
}

func whereOptions(o *RunOptions) error {
	v := whereValue
	whereChosen = where.Target{}
	k := whereKnown(context.Background(), !o.Record)
	if v == "" {
		if !k.HasRemote() {
			return nil
		}
		v = "auto"
	}
	return applyWhere(o, v, k)
}

// applyWhere maps v and merges the values under any value the flags already set.
func applyWhere(o *RunOptions, v string, k where.Known) error {
	t, err := where.Values(v, k, where.Opts{Dev: !o.Record, RaceReady: cirun.RaceRegistered()})
	if err != nil {
		return err
	}
	whereChosen = t
	if len(t.Values) == 0 {
		return nil
	}
	merged := map[string]json.RawMessage{}
	for key, raw := range t.Values {
		merged[key] = raw
	}
	for key, raw := range o.PolicyRun {
		merged[key] = raw
	}
	o.PolicyRun = merged
	return nil
}

// productionWhereKnown lists the registered nodes, pools and hosted providers.
// A store that cannot be read gives nothing registered.
func productionWhereKnown(ctx context.Context, dev bool) where.Known {
	w, err := loadPoolsWorld(ctx, "")
	if err != nil {
		return where.Known{}
	}
	return knownFromWorld(w, dev)
}

// knownFromWorld derives where.Known from the pools world: the effective set
// is the pools.allow term (and pools.dev for a dev run) the resolver applies.
func knownFromWorld(w poolsWorld, dev bool) where.Known {
	allow, devPools, _ := pools.FromPolicy(w.Policy)
	res := pools.Resolve(pools.Input{Runners: w.Runners, Allow: allow, Dev: dev, DevPools: devPools})
	var k where.Known
	seen := map[string]bool{}
	for _, r := range w.Runners {
		switch r.Source {
		case pools.SourceRegistry, pools.SourceCoordinator:
			k.Nodes = append(k.Nodes, r.ID)
			if d := res.Excluded[r.ID]; len(d) > 0 {
				if k.Excluded == nil {
					k.Excluded = map[string]string{}
				}
				k.Excluded[r.ID] = d[0].Detail
			}
		case pools.SourceProvider:
			k.Providers = append(k.Providers, r.ID)
		}
		for _, id := range pools.FromAssigned(r.Assigned) {
			if !seen[id] {
				seen[id] = true
				k.Pools = append(k.Pools, id)
			}
		}
	}
	if allow.Set {
		k.Effective = allow.IDs
	}
	return k
}

// whereFlagSet adds --where to a flag set that does not share bareFlags.
func whereFlagSet(fs *flag.FlagSet, dst *string) {
	fs.StringVar(dst, "where", "", whereHelp)
}

// jsonPlacement is data.placement of the bare run's --json envelope
// (contract:ci.placement-plan); nil when none could be computed.
var jsonPlacement any

// attachPlacement computes the placement the run's target gives, best effort:
// a plan or store that cannot be read leaves data.placement out, and the run
// itself is never failed by it. It reads the same plan, world and policy the
// run uses; it creates no lease and no record.
func attachPlacement(ctx context.Context, o RunOptions) {
	jsonPlacement = nil
	// Only when --where is given: ci.evidence/v1 allows no extra member, so a plain --json stays one evidence document.
	if whereValue == "" || !o.JSON || o.Stream || len(o.Adhoc) > 0 {
		return
	}
	pd, err := buildPlanned(ctx, planSpec{bare.repo, bare.cfg, bare.base, o.Pipeline, o.Trigger})
	if err != nil {
		return
	}
	// placementDoc reads the run's policy values from the parsed flags
	// (schedRevisionValues), the same values schedOptions put in o.PolicyRun.
	doc, err := placementDoc(ctx, pd)
	if err == nil {
		jsonPlacement = doc
	}
}

// withPlacement returns doc with a "placement" member added; doc unchanged
// when there is no placement.
func withPlacement(doc, placement any) any {
	if placement == nil {
		return doc
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		return doc
	}
	m := map[string]json.RawMessage{}
	if json.Unmarshal(raw, &m) != nil {
		return doc
	}
	pl, err := json.Marshal(placement)
	if err != nil {
		return doc
	}
	m["placement"] = pl
	return m
}
