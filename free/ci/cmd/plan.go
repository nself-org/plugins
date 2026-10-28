package main

// Purpose: the `plan` subcommand of nself-ci (P7-CI-35): probe the local
// world and the git revision, call the pure planner and print the plan.
// Inputs:  --json, --pipeline NAME, --trigger T, --repo DIR, --config FILE,
// --base SHA (env NSELF_CI_REPO, NSELF_CI_BASE_SHA).
// Outputs: a table, or with --json one v1 envelope whose data is the plan
// (jobs, layers, skipped, selection); exit 0, 1 (invalid config or plan),
// 2 (infrastructure).
// Constraints: V15 only, registered from init(); every process (git) runs
// through the exec funnel; catalogue checks join the plan only when the
// config declares no jobs of its own; SCHED files add flags to planFlags and
// set placePlan from their init(); nil placePlan adds nothing to the output.

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/output"

	"github.com/nself-org/plugins/free/ci/internal/config"
	"github.com/nself-org/plugins/free/ci/internal/detect"
	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/gates"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/plan"
)

// planFlags is the flag set of `plan`; SCHED files register more flags on it.
var planFlags = flag.NewFlagSet("plan", flag.ContinueOnError)

// placePlan, when set, adds placement to the plan after it is built and
// validated: data goes under data.placement, human to the table.
var placePlan func(ctx context.Context, p planned) (data any, human []string, err error)

var planOpts struct {
	json                               bool
	pipeline, trigger, repo, cfg, base string
}

func init() {
	planFlags.BoolVar(&planOpts.json, "json", false, "print one v1 JSON envelope")
	planFlags.StringVar(&planOpts.pipeline, "pipeline", "", "named pipeline to plan (default: every non-drill job)")
	planFlags.StringVar(&planOpts.trigger, "trigger", "", "plan the pipelines listing this trigger (push, pull_request, tag, manual, schedule, api, local)")
	planFlags.StringVar(&planOpts.repo, "repo", "", "repository root (env NSELF_CI_REPO; default: current directory)")
	planFlags.StringVar(&planOpts.cfg, "config", "", "config file (default: <repo>/.nself/ci.yaml)")
	planFlags.StringVar(&planOpts.base, "base", "", "base commit for affected selection (env NSELF_CI_BASE_SHA; default: merge-base with origin's default branch)")
	registerV15("plan", subcommandEntry{
		V15Only: true,
		Flags:   func() *flag.FlagSet { return planFlags },
		Handler: runPlan,
	})
}

func planFail(asJSON bool, err error) int {
	code, exit, msg := "E619", 2, err.Error()
	var pe *plan.Error
	var ce *config.Error
	switch {
	case errors.As(err, &pe):
		code, exit = pe.Code, 1
	case errors.As(err, &ce):
		code, exit = ce.Code, 1
		ds := make([]plan.Diagnostic, len(ce.Problems))
		for i, p := range ce.Problems {
			ds[i] = plan.Diagnostic{File: p.File, Line: p.Line, Pointer: p.Pointer, Message: p.Message}
		}
		if de := plan.DAGError(ds); de != nil { // a cycle or undefined dep is E616, not E620
			code, msg = de.Code, de.Error()
		}
	}
	if asJSON {
		_ = output.WriteError(os.Stdout, "ci plan", output.ErrorDetail{Code: code, Message: msg, ExitCode: exit}, nil)
	} else {
		fmt.Fprintln(os.Stderr, msg)
	}
	return exit
}

func runPlan(args []string) int {
	planFlags.SetOutput(os.Stderr)
	if err := planFlags.Parse(args); err != nil {
		return 2
	}
	o := planOpts
	ctx := context.Background()
	pd, err := buildPlanned(ctx, planSpec{o.repo, o.cfg, o.base, o.pipeline, o.trigger})
	if err != nil {
		return planFail(o.json, err)
	}
	p := pd.pl
	var place plan.PlaceFunc
	if placePlan != nil {
		place = func(ctx context.Context, p *plan.Plan) (any, []string, error) {
			pd.pl = p
			return placePlan(ctx, pd)
		}
	}
	cells, err := p.ApplyPlacement(ctx, place)
	if err != nil {
		return planFail(o.json, err)
	}
	if o.json {
		if err := output.WriteData(os.Stdout, "ci plan", p, nil); err != nil {
			return planFail(false, err)
		}
		return 0
	}
	if _, err := fmt.Fprintln(os.Stdout, strings.Join(p.Human(cells), "\n")); err != nil {
		return planFail(false, err)
	}
	return 0
}

// planChecks detects the repo's ecosystems and plans the catalogue checks
// (only when the config declares no jobs; detect.disable drops ecosystems).
func planChecks(root string, cfg model.PipelineConfig, full ...bool) ([]detect.Fact, []plan.Check, error) {
	reg := gates.DefaultRegistry()
	facts, err := planDetectors(full...).Detect(os.DirFS(root), reg.Has)
	if err != nil {
		return nil, nil, err
	}
	if len(cfg.Jobs) > 0 {
		return facts, nil, nil
	}
	off := map[string]bool{}
	if cfg.Detect != nil {
		for _, d := range cfg.Detect.Disable {
			off[d] = true
		}
	}
	var out []plan.Check
	for _, c := range reg.Plan(facts, gates.Options{Config: cfg}) {
		if off[gates.Ecosystem(c.Def.ID)] {
			continue
		}
		out = append(out, plan.Check{ID: c.Def.ID, Kind: c.Def.Kind, Tool: c.PlacementTool(), Workdir: c.Workdir, Target: c.Target,
			Argv: c.Command(), Required: c.Required, Substantive: c.Def.Substantive, Path: c.Def.Path,
			Platform: c.Def.Platform, Requires: c.Def.RequiresTools})
	}
	return facts, out, nil
}

var planSHA = regexp.MustCompile(`^[0-9a-f]{40,64}$`)

// planRevision computes the revision context through the exec funnel: the
// PR base sha (--base or NSELF_CI_BASE_SHA), else the merge-base with
// origin's default branch; no base means BaseKnown=false (full plan). An
// unknown default branch or a detached HEAD counts as the default branch
// (plan.ClassifyRevision), so required deep jobs are never dropped by guess.
func planRevision(ctx context.Context, root, base string) plan.Revision {
	raw := func(args ...string) (string, bool) { return planGit(ctx, root, args...) }
	git := func(args ...string) (string, bool) {
		s, ok := raw(args...)
		return strings.TrimSpace(s), ok
	}
	rev := plan.Revision{}
	for _, kv := range os.Environ() {
		rev.Env = append(rev.Env, strings.SplitN(kv, "=", 2)[0])
	}
	// Six independent read-only queries: each costs a funnel spawn, so they run
	// side by side and the revision waits for the slowest one, not their sum.
	type answer struct {
		out string
		ok  bool
	}
	remotes := []string{"main", "master", "trunk"}
	queries := [][]string{{"rev-parse", "--abbrev-ref", "HEAD"}, {"symbolic-ref", "--short", "refs/remotes/origin/HEAD"},
		{"describe", "--exact-match", "--tags", "HEAD"}}
	for _, n := range remotes {
		queries = append(queries, []string{"rev-parse", "--verify", "--quiet", "refs/remotes/origin/" + n})
	}
	answers := make([]answer, len(queries))
	var wg sync.WaitGroup
	for i, q := range queries {
		wg.Add(1)
		go func() {
			defer wg.Done()
			answers[i].out, answers[i].ok = git(q...)
		}()
	}
	wg.Wait()
	facts := plan.BranchFacts{Detached: true}
	if b := answers[0]; b.ok && b.out != "HEAD" && b.out != "" {
		facts.Branch, facts.Detached = b.out, false
		rev.Branch = b.out
	}
	if s := answers[1]; s.ok {
		facts.OriginHead = strings.TrimPrefix(s.out, "origin/")
	}
	facts.ExactTag = answers[2].ok
	for i, n := range remotes {
		if answers[3+i].ok {
			facts.Remotes = append(facts.Remotes, n)
		}
	}
	rev.DefaultBranch, rev.Release = plan.ClassifyRevision(facts)
	base = firstOf(base, os.Getenv("NSELF_CI_BASE_SHA"))
	if def := plan.DefaultBranchName(facts); base == "" && def != "" {
		if mb, ok := git("merge-base", "HEAD", "origin/"+def); ok {
			base = mb
		}
	}
	if !planSHA.MatchString(base) {
		return rev
	}
	// NUL-separated, renames split into delete + add, so no path is quoted
	// or lost: a file moved out of a module still selects that module.
	changed, ok1 := raw("diff", "--name-only", "--no-renames", "-z", base)
	fresh, ok2 := raw("ls-files", "-z", "--others", "--exclude-standard")
	if !ok1 || !ok2 {
		return rev
	}
	seen := map[string]bool{}
	for _, l := range strings.Split(changed+"\x00"+fresh, "\x00") {
		if l = strings.Trim(l, "\r\n"); l != "" && !seen[l] {
			seen[l] = true
			rev.Changed = append(rev.Changed, l)
		}
	}
	sort.Strings(rev.Changed)
	rev.BaseKnown = true
	return rev
}

// planGit runs git in a funnel workspace and returns its raw output; ok is
// false on a non-zero exit or any failure.
func planGit(ctx context.Context, root string, args ...string) (string, bool) {
	return planRun(ctx, append([]string{"git", "-c", "core.fsmonitor=false", "-C", root}, args...)...)
}

// planSeq keeps attempt ids unique when planRevision runs its queries at once.
var planSeq atomic.Uint64

// planRun runs a command in a funnel workspace and returns its raw output;
// ok is false on a non-zero exit or any failure.
func planRun(ctx context.Context, argv ...string) (string, bool) {
	home, err := os.MkdirTemp("", "nself-ci-plan-")
	if err != nil {
		return "", false
	}
	defer func() { _ = os.RemoveAll(home) }()
	var buf strings.Builder
	var ex ciexec.Executor
	res := ex.Run(ctx, ciexec.JobSpec{
		AttemptID: fmt.Sprintf("plan-git-%d-%d", time.Now().UnixNano(), planSeq.Add(1)), CoordinatorID: "plan",
		Job: model.Job{Kind: "static"}, Home: home, Timeout: 30 * time.Second,
		Command: argv,
		Output:  func(b []byte) { buf.Write(b) },
	})
	return buf.String(), res.Err == nil && res.ExitCode == 0
}
