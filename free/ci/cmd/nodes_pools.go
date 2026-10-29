package main

// Purpose: `nself ci nodes pools list|show|explain` (P7-AUTO-03, Epic AUTO D4, D10): the pools of the
//          runners this person can reach (contract:ci.pool v1) and the effective set of a project. Also
//          the composition root of the pools exclusion provider, so `ci plan` carries the same
//          pool.excluded reasons.
// Inputs:  show takes a pool id; --project <dir> (default: the current repository, env NSELF_CI_REPO);
//          --json; env NSELF_CI_HOME (the store with the nodes, their assigned labels and policy rows).
// Outputs: a table or one v1 JSON envelope; exit 0, 1 (usage, unknown pool), 2 (infrastructure).
// Constraints: V15 only; side-effect class read. Membership is the assigned `pool.<id>` label only; a
//          node's self-declared labels are never read. The policy comes from the TRUST-27 loader (store
//          rows, protected default branch `.nself/ci.yaml`); outside a repository list and show fall back
//          to every term unset and say so, explain refuses.

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/autoworld"
	"github.com/nself-org/plugins/free/ci/internal/nodes/labels"
	"github.com/nself-org/plugins/free/ci/internal/nodes/lifecycle"
	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/pools"
	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/report"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

type nodesPoolsOpts struct {
	json    bool
	project string
}

func newNodesPoolsFlags() (*flag.FlagSet, *nodesPoolsOpts) {
	o := &nodesPoolsOpts{}
	fs := flag.NewFlagSet("nodes pools", flag.ContinueOnError)
	fs.BoolVar(&o.json, "json", false, "print one v1 JSON envelope")
	fs.StringVar(&o.project, "project", "", "project directory whose pools.allow applies (default: the current repository)")
	return fs, o
}

func nodesPoolsFlags() *flag.FlagSet { fs, _ := newNodesPoolsFlags(); return fs }

// poolsWorld is what the pools commands read: the reachable runners and the project's resolved policy.
type poolsWorld struct {
	Runners   []pools.Runner
	Project   string
	Policy    trust.Effective
	PolicyErr error
}

// poolsDeps loads a poolsWorld for a project directory ("" means the current repository).
type poolsDeps struct {
	Load func(ctx context.Context, dir string) (poolsWorld, error)
}

func init() {
	for _, sub := range []string{"list", "show", "explain"} {
		sub := sub
		registerV15("nodes pools "+sub, subcommandEntry{
			V15Only: true,
			Flags:   nodesPoolsFlags,
			Handler: func(args []string) int {
				return runNodesPools(sub, args, poolsDeps{Load: loadPoolsWorld}, os.Stdout, os.Stderr)
			},
		})
	}
	reader := &labels.LazyReader{Path: func() string { return filepath.Join(lifecycle.CIHome(), "state.db") },
		Live: func() *store.Store {
			if d := nodeAdminDeps.Load(); d != nil {
				return d.store
			}
			return nil
		}}
	// The world builder runs only on the v1.5 plan path (v1.4 dispatches to internal/legacy), so the
	// provider is dormant in v1.4 without a gate of its own.
	fn := pools.ExclusionProvider(pools.Config{Labels: reader, Team: func() bool { return nodeAdminDeps.Load() != nil }})
	if err := autoworld.RegisterExclusion(pools.ProviderName, fn); err != nil {
		panic("nself-ci: " + err.Error())
	}
}

// parseNodesPools splits argv into the options and the positional arguments.
func parseNodesPools(sub string, args []string) (*nodesPoolsOpts, []string, error) {
	fs, o := newNodesPoolsFlags()
	fs.SetOutput(io.Discard)
	usage := placementUsage("usage: nself ci nodes pools " + map[string]string{"list": "list", "show": "show <pool>", "explain": "explain"}[sub] + " [--project <dir>] [--json]")
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			return o, nil, usage
		}
		if args = fs.Args(); len(args) == 0 {
			break
		}
		pos, args = append(pos, args[0]), args[1:]
	}
	if want := map[string]int{"show": 1}[sub]; len(pos) != want {
		return o, nil, usage
	}
	if sub == "show" && !pools.ValidID(pos[0]) {
		return o, nil, placementUsage("the pool id is not valid (^[a-z][a-z0-9-]{0,30}$)")
	}
	return o, pos, nil
}

// runNodesPools is the handler with its environment injected.
func runNodesPools(sub string, args []string, d poolsDeps, out, errOut io.Writer) int {
	command := "ci nodes pools " + sub
	o, pos, err := parseNodesPools(sub, args)
	if err != nil {
		return placementFail(out, errOut, command, o.json || hasJSONArg(args), err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	w, err := d.Load(ctx, o.project)
	if err != nil {
		return placementFail(out, errOut, command, o.json, placementInfra("the pools cannot be read: "+oneLine(err)))
	}
	var notes []string
	in := pools.Input{Runners: w.Runners, Project: w.Project}
	if w.PolicyErr != nil {
		if sub == "explain" || o.project != "" {
			return placementFail(out, errOut, command, o.json, placementInfra("the project policy cannot be resolved: "+oneLine(w.PolicyErr)))
		}
		notes = append(notes, "no project policy ("+oneLine(w.PolicyErr)+"): every pool term is unset")
		in.Project = ""
	} else {
		var more []string
		in.Allow, in.DevPools, more = pools.FromPolicy(w.Policy)
		notes = append(notes, more...)
	}
	res := pools.Resolve(in)
	switch sub {
	case "list":
		return writePools(out, command, o.json, res.Pools, notes)
	case "show":
		for _, p := range res.Pools {
			if p.ID == pos[0] {
				return writePools(out, command, o.json, []pools.Pool{p}, notes)
			}
		}
		return placementFail(out, errOut, command, o.json, placementUsage("pool "+pos[0]+" has no member"))
	}
	data := pools.Explain(in, notes)
	if o.json {
		return writeEnvelope(out, command, data)
	}
	writeExplainText(out, data)
	return 0
}

// writeEnvelope prints one v1 data envelope; the shared writer maps a write failure to the infra exit.
func writeEnvelope(out io.Writer, command string, data any) int {
	return backupData(backupEnv{Out: out}, command, data)
}

func writePools(out io.Writer, command string, asJSON bool, ps []pools.Pool, notes []string) int {
	if asJSON {
		return writeEnvelope(out, command, map[string]any{"pools": ps, "notes": append([]string{}, notes...)})
	}
	for _, p := range ps {
		state := "allowed"
		if !p.Effective.Allowed {
			state = "excluded " + p.Effective.Reasons[0].Detail
		}
		ids := make([]string, 0, len(p.Members))
		for _, m := range p.Members {
			ids = append(ids, m.RunnerID)
		}
		_, _ = fmt.Fprintf(out, "%s\t%s\t%s\t%s\n", p.ID, p.Scope, state, strings.Join(ids, " "))
	}
	for _, n := range notes {
		_, _ = fmt.Fprintf(out, "note: %s\n", plainText(n))
	}
	return 0
}

func writeExplainText(out io.Writer, d pools.Explanation) {
	_, _ = fmt.Fprintf(out, "project %s: eligible %s\n", plainText(d.Project), strings.Join(d.Eligible, " "))
	for _, r := range d.Excluded {
		_, _ = fmt.Fprintf(out, "excluded %s: %s %s\n", r.RunnerID, r.Reasons[0].Code, r.Reasons[0].Detail)
	}
	for _, id := range d.Unauthorized {
		_, _ = fmt.Fprintf(out, "unauthorized %s: lifecycle.authorized_projects omits the project\n", id)
	}
	for _, r := range d.DevExcluded {
		_, _ = fmt.Fprintf(out, "dev run excludes %s: %s %s\n", r.RunnerID, r.Reasons[0].Code, r.Reasons[0].Detail)
	}
	for _, n := range d.Notes {
		_, _ = fmt.Fprintf(out, "note: %s\n", plainText(n))
	}
}

// loadPoolsWorld reads the runners of the person's store, this machine and the registered providers, and
// resolves the policy of the project at dir.
func loadPoolsWorld(ctx context.Context, dir string) (poolsWorld, error) {
	d, closeFn, err := lifecycle.ProductionEnv(nil).Open(ctx, "")
	if err != nil {
		return poolsWorld{}, err
	}
	defer closeFn()
	nodes, err := d.Registry.List(ctx)
	if err != nil {
		return poolsWorld{}, err
	}
	var w poolsWorld
	w.Project, w.Policy, w.PolicyErr = resolvePoolsPolicy(ctx, d.Store, dir)
	for _, n := range nodes {
		set, err := d.Store.NodeLabels(ctx, n.Record.ID)
		if err != nil {
			return poolsWorld{}, err
		}
		r := pools.Runner{ID: n.Record.ID, Source: pools.SourceRegistry, Assigned: labels.Strings(set), Projects: n.Capability.Lifecycle.AuthorizedProjects}
		if v := n.Capability.Location.Kind.Value; v != nil {
			r.Location = *v
		}
		w.Runners = append(w.Runners, r)
	}
	set, err := d.Store.NodeLabels(ctx, labels.LocalNode)
	if err != nil {
		return poolsWorld{}, err
	}
	// This machine and the providers are not gated by authorized_projects: they are always the project's own.
	w.Runners = append(w.Runners, pools.Runner{ID: labels.LocalNode, Source: pools.SourceLocal, Location: "local", Assigned: labels.Strings(set), Projects: []string{w.Project}})
	for _, p := range providers.All() {
		w.Runners = append(w.Runners, pools.Runner{ID: p.ID(), Source: pools.SourceProvider, Location: "hosted", Projects: []string{w.Project}})
	}
	return w, nil
}

// resolvePoolsPolicy resolves the policy of the repository at dir (default NSELF_CI_REPO, else the working directory).
func resolvePoolsPolicy(ctx context.Context, st *store.Store, dir string) (string, trust.Effective, error) {
	if dir == "" {
		dir = os.Getenv("NSELF_CI_REPO")
	}
	if dir == "" {
		dir = "."
	}
	root, err := filepath.Abs(dir)
	if err != nil {
		return "", trust.Effective{}, err
	}
	if fi, err := os.Stat(root); err != nil || !fi.IsDir() {
		return "", trust.Effective{}, errors.New(plainText(dir) + " is not a directory")
	}
	run := policy.Runner(report.Capture)
	project := policy.ProjectIDGit(ctx, run, root)
	res := policy.Resolver{Store: st, Run: run}
	got, err := res.Resolve(ctx, policy.Repo{Root: root, Project: project, Visibility: "unknown", Class: "owner"}, policy.Revision{Dir: root})
	if err != nil {
		return project, trust.Effective{}, err
	}
	return project, got.Effective, nil
}
