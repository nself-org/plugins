package main

// Purpose: show pools.record beside the effective development pool set.
// Inputs: the same protected policy and assigned labels as nodes pools.
// Outputs: record column and every exclusion reason in text or JSON.
// Constraints: V15 composition only; no project authorization command is added.

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	compat "github.com/nself-org/cli/sdk/go/v2/compat"
	"github.com/nself-org/plugins/free/ci/internal/autoworld"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/nodes/labels"
	"github.com/nself-org/plugins/free/ci/internal/nodes/lifecycle"
	"github.com/nself-org/plugins/free/ci/internal/pools"
	"github.com/nself-org/plugins/free/ci/internal/pools/record"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/sched"
)

func init() {
	entry := v15Subcommands["nodes pools explain"]
	entry.Handler = func(args []string) int {
		return runPoolsRecordExplain(args, poolsDeps{Load: loadPoolsWorld}, os.Stdout, os.Stderr)
	}
	v15Subcommands["nodes pools explain"] = entry
	reader := &labels.LazyReader{Path: func() string { return filepath.Join(lifecycle.CIHome(), "state.db") }}
	if err := autoworld.RegisterExclusion("pools.record.plan", func(ctx context.Context, w sched.World) (map[string][]model.PlacementReasonDetail, error) {
		if !compat.V15() || cirun.SessionFrom(ctx) != nil {
			return nil, nil
		}
		return record.ExclusionProvider(pools.Config{Labels: reader, Team: func() bool { return nodeAdminDeps.Load() != nil }}, func(context.Context) bool { return true })(ctx, w)
	}); err != nil {
		panic(err)
	}
}

type recordExplanation struct {
	pools.Explanation
	Record         []string            `json:"record"`
	RecordExcluded []pools.ExcludedRow `json:"record_excluded"`
}

func runPoolsRecordExplain(args []string, d poolsDeps, out, errOut io.Writer) int {
	const command = "ci nodes pools explain"
	o, _, err := parseNodesPools("explain", args)
	if err != nil {
		return placementFail(out, errOut, command, o.json || hasJSONArg(args), err)
	}
	w, err := d.Load(context.Background(), o.project)
	if err != nil {
		return placementFail(out, errOut, command, o.json, placementInfra("the pools cannot be read: "+oneLine(err)))
	}
	if w.PolicyErr != nil {
		return placementFail(out, errOut, command, o.json, placementInfra("the project policy cannot be resolved: "+oneLine(w.PolicyErr)))
	}
	allow, dev, notes := pools.FromPolicy(w.Policy)
	for _, ig := range w.Policy.Ignored() {
		if ig.Key == record.Key {
			notes = append(notes, fmt.Sprintf("%s from %s ignored: %s", ig.Key, ig.Scope, ig.Reason))
		}
	}
	in := pools.Input{Runners: w.Runners, Project: w.Project, Allow: allow, DevPools: dev}
	data := recordExplanation{Explanation: pools.Explain(in, notes), Record: []string{}, RecordExcluded: []pools.ExcludedRow{}}
	excluded := record.Excluded(record.FromPolicy(w.Policy), w.Runners)
	for _, r := range w.Runners {
		if why := excluded[r.ID]; len(why) > 0 {
			data.RecordExcluded = append(data.RecordExcluded, pools.ExcludedRow{RunnerID: r.ID, Pools: pools.Memberships(r), Reasons: why})
		} else if slices.Contains(data.Eligible, r.ID) {
			data.Record = append(data.Record, r.ID)
		}
	}
	slices.Sort(data.Record)
	if o.json {
		return writeEnvelope(out, command, data)
	}
	writeExplainText(out, data.Explanation)
	fmt.Fprintln(out, "runner\trecord")
	for _, r := range w.Runners {
		state := "allowed"
		if !slices.Contains(data.Record, r.ID) {
			state = "excluded"
		}
		fmt.Fprintf(out, "%s\t%s\n", r.ID, state)
	}
	fmt.Fprintf(out, "record eligible %s\n", strings.Join(data.Record, " "))
	for _, r := range data.RecordExcluded {
		fmt.Fprintf(out, "record excludes %s: %s %s\n", r.RunnerID, r.Reasons[0].Code, r.Reasons[0].Detail)
	}
	return 0
}
