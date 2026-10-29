package main

// Purpose: `nself ci serve watch` (P7-AUTO-11, ADR 0032 decision 4), the PERSONAL-mode trigger source,
//          under serve because the subcommand cap leaves no first-level slot (AUTO D9). The loop is
//          internal/watch; this file wires its ports: the HOST-17 head scan, the status ledger and
//          reporter, readiness, the result context and the coordinator path of a bare run.
// Inputs:  argv; the operator policy (watch.*, providers.github.*, providers.report_owner, readiness.*);
//          NSELF_CI_REPO, NSELF_CI_HOME.
// Outputs: one JSON envelope (or one line) per pass; demand, heartbeat and log under $NSELF_CI_HOME;
//          exit 0 after SIGTERM or --once, 1 on a start failure, 2 on usage.
// Constraints: V15 only; outbound only (no listener, no port); the token is read by the provider runtime
//          from providers.github.token_env, never printed or put in argv; a commit runs in a detached
//          worktree with Options.Record and trigger api, NoStatus because the watcher reports; no isolated
//          runner is wired, so a collaborator head is always skipped (E821); the hosted tier and the
//          background tick stay off until their ports exist.

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/output"
	"github.com/nself-org/plugins/free/ci/internal/demand"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/providers/github/reconcile"
	"github.com/nself-org/plugins/free/ci/internal/readiness"
	"github.com/nself-org/plugins/free/ci/internal/readiness/sample"
	"github.com/nself-org/plugins/free/ci/internal/report"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/trust/source"
	"github.com/nself-org/plugins/free/ci/internal/watch"
)

const serveWatchUsage = "nself ci serve watch [--interval 60s] [--repos o/r,…] [--once] [--json]"

type serveWatchFlags struct {
	interval   time.Duration
	repos      listFlag
	once, json bool
}

func newServeWatchFlagSet(f *serveWatchFlags) *flag.FlagSet {
	fs := flag.NewFlagSet("serve watch", flag.ContinueOnError)
	fs.DurationVar(&f.interval, "interval", 0, "pass cadence (default: providers.github.poll_interval)")
	fs.Var(&f.repos, "repos", "comma-separated owner/repo list (default: this checkout's repository)")
	fs.BoolVar(&f.once, "once", false, "run one pass and exit")
	fs.BoolVar(&f.json, "json", false, "print one v1 JSON envelope per pass")
	return fs
}

func init() {
	registerV15("serve watch", subcommandEntry{V15Only: true,
		Flags:   func() *flag.FlagSet { return newServeWatchFlagSet(&serveWatchFlags{}) },
		Handler: serveWatchCmd})
}

func serveWatchCmd(args []string) int {
	var f serveWatchFlags
	if pos, err := parseMixed(newServeWatchFlagSet(&f), args); err != nil || len(pos) != 0 || f.interval < 0 {
		return usageErr(serveWatchUsage)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	w, closeFn, err := buildWatcher(ctx, f)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	defer closeFn()
	w.Run(ctx, f.once, func(rep watch.PassReport) {
		if f.json {
			_ = output.WriteData(os.Stdout, "ci serve watch", rep, nil)
			return
		}
		fmt.Fprintf(os.Stdout, "%s ready=%t ran=%d queued=%d held=%d errors=%d\n", rep.At.Format(time.RFC3339), rep.Ready, rep.Ran, rep.Queued, rep.Held, len(rep.Errors))
	})
	return 0
}

// buildWatcher resolves the settings and wires the ports.
func buildWatcher(ctx context.Context, f serveWatchFlags) (w *watch.Watcher, closeFn func(), err error) {
	root, err := evidenceRoot("")
	if err != nil {
		return nil, nil, err
	}
	home := sample.CIHome()
	if home == "" {
		return nil, nil, errors.New("no CI home: set NSELF_CI_HOME")
	}
	st, err := openHistory(true)
	if err != nil {
		return nil, nil, err
	}
	closeFn = func() { _ = st.Close() }
	defer func() {
		if err != nil {
			closeFn()
		}
	}()
	res := policy.Resolver{Store: st, Run: policy.Runner(report.Capture)}
	got, err := res.Resolve(ctx, policy.Repo{Root: root, Project: policy.ProjectIDGitCached(ctx, res.Run, root), Visibility: "unknown", Class: "owner"}, policy.Revision{Dir: root})
	if err != nil {
		return nil, nil, err
	}
	eff := &got.Effective
	c, err := watch.ApplyPolicy(watch.Defaults(), eff)
	gs, gerr := reconcile.SettingsFrom(eff)
	if err = errors.Join(err, gerr); err != nil {
		return nil, nil, err
	}
	c.Interval = gs.PollInterval
	if f.interval > 0 {
		c.Interval = f.interval
	}
	gh := report.GH{Capture: report.Capture}
	if login, lerr := gh.Login(ctx); lerr == nil {
		c.Trusted = []string{login}
	}
	host, _ := os.Hostname()
	c.Host = strings.NewReplacer(" ", "-", "\t", "-", "\n", "-").Replace(firstOf(host, "localhost"))
	c.Incarnation = strconv.FormatInt(time.Now().UnixNano(), 36)
	roots, err := watchRoots(ctx, root, home, f.repos.vals)
	if err != nil {
		return nil, nil, err
	}
	for repo := range roots {
		c.Repos = append(c.Repos, repo)
	}
	reconcile.UsePolicy(func(context.Context) (*policy.Effective, error) { return eff, nil })
	reconcile.Use(st)
	rc := &report.ResultContexts{Read: gh.RequiredPinned}
	ev := &readiness.Evaluator{Config: func(context.Context) (readiness.Config, error) { return readiness.ConfigFrom(eff) }}
	team := providers.ReportOwner(*eff) == report.OwnerTeam
	logw := &watch.RotatingLog{Dir: watch.LogDir(home)}
	opts := reconcile.ScanOptions{Branches: c.Branches, MaxBranches: c.MaxBranches}
	w, err = watch.New(c, watch.Ports{
		Scanner: watchScanner{e: reconcile.Default(), opts: opts, days: c.BranchDays}, Ledger: gh, Reporter: gh,
		Runner: watchRunner{roots: roots, home: home}, ResultContext: rc.ResolveResultContext,
		Ready: func(ctx context.Context) (bool, []string) {
			r, rerr := ev.Evaluate(ctx)
			if rerr != nil {
				return false, []string{rerr.Error()}
			}
			return r.Ready, r.Why
		},
		TeamOwned: func(context.Context, string) bool { return team },
		Alert: func(_ context.Context, h watch.Head, d string) {
			logw.Event("alert", map[string]any{"repo": h.Repo, "sha": h.SHA, "message": d})
		},
		Publish: func(doc demand.Document) error {
			return demand.Write(demand.WriterConfig{Dir: home, Host: c.Host}, doc)
		},
		Heartbeat: watch.Heartbeat(watch.LogDir(home)), Log: logw.Event,
	})
	return w, closeFn, err
}

// watchRoots maps each watched repository to a local checkout: this checkout for its own origin, else
// $NSELF_CI_HOME/watch/repos/<owner>/<repo>. A repository with no checkout refuses the start.
func watchRoots(ctx context.Context, root, home string, repos []string) (map[string]string, error) {
	out := map[string]string{}
	url, _, _ := report.Capture(ctx, "git", "-C", root, "remote", "get-url", "origin")
	owner, name, ok := report.ParseRemote(strings.TrimSpace(url))
	self := strings.ToLower(owner + "/" + name)
	if len(repos) == 0 {
		if !ok {
			return nil, errors.New("this checkout has no GitHub origin; pass --repos")
		}
		return map[string]string{owner + "/" + name: root}, nil
	}
	for _, r := range repos {
		r = strings.TrimSpace(r)
		if strings.Count(r, "/") != 1 {
			return nil, fmt.Errorf("--repos: %q is not owner/repo", r)
		}
		dir := filepath.Join(home, "watch", "repos", filepath.FromSlash(r))
		if ok && strings.ToLower(r) == self {
			dir = root
		}
		if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
			return nil, fmt.Errorf("%s has no local checkout (clone it to %s)", r, dir)
		}
		out[r] = dir
	}
	return out, nil
}

// watchScanner is the HOST-17 head scan behind watch.Scanner.
type watchScanner struct {
	e    *reconcile.Engine
	opts reconcile.ScanOptions
	days int
}

func (s watchScanner) Scan(ctx context.Context, repo string) (watch.Scan, error) {
	o := s.opts
	if s.days > 0 {
		o.Since = s.e.Now().Add(-time.Duration(s.days) * 24 * time.Hour)
	}
	hs, err := s.e.ScanHeads(ctx, repo, o)
	var q *reconcile.QuotaStop
	if errors.As(err, &q) {
		return watch.Scan{}, &watch.QuotaError{RetryAt: q.RetryAt}
	}
	if err != nil || hs.NotModified {
		return watch.Scan{NotModified: hs.NotModified}, err
	}
	out := watch.Scan{}
	for _, h := range hs.Heads {
		out.Heads = append(out.Heads, watchHead(repo, h))
	}
	return out, nil
}

// watchHead classifies a scanned head by the TRUST source rules and restamps its facts as a watch
// trigger. Only a writer can push a branch of the repository; the default and protected branches are
// the owner's; a pull request's author association names its permission; a fork is untrusted.
func watchHead(repo string, h reconcile.Head) watch.Head {
	f := h.Facts
	now := f.ReceivedAt
	perm := "write"
	switch {
	case h.Fork:
		perm = ""
	case h.PR > 0:
		switch strings.ToUpper(h.AuthorAssociation) {
		case "OWNER":
			perm = "admin"
		case "MEMBER", "COLLABORATOR":
		default:
			perm = ""
		}
	case h.Default || h.Protected:
		perm = "admin"
	}
	if perm != "" {
		f.ActorPermission = model.Fact[string]{Value: &perm, Source: "api", ObservedAt: now, Confidence: "known"}
	}
	if h.Default {
		yes := true
		f.SHAReachable = model.Fact[bool]{Value: &yes, Source: "api", ObservedAt: now, Confidence: "known"}
	}
	class := source.Classify(f).Class
	f.Kind, f.Source, f.Auth = "api", "watch", "per-repo"
	return watch.Head{Repo: repo, SHA: h.SHA, Branch: h.Branch, Default: h.Default, Protected: h.Protected, Extra: h.Extra,
		PR: h.PR, Fork: h.Fork, Trust: class, CommittedAt: h.CommittedAt, Facts: f}
}

// watchRunner runs a commit through the coordinator in a detached worktree of its local checkout.
type watchRunner struct {
	roots map[string]string
	home  string
}

func (r watchRunner) Run(ctx context.Context, j watch.Job) (watch.Outcome, error) {
	root, ok := r.roots[j.Head.Repo]
	if !ok || j.Isolated {
		return watch.Outcome{}, fmt.Errorf("no local runner for %s", j.Head.Repo)
	}
	git := func(c context.Context, args ...string) error {
		out, exit, err := report.Capture(c, append([]string{"git", "-C", root}, args...)...)
		if err == nil && exit != 0 {
			err = fmt.Errorf("git %s: %s", args[0], strings.TrimSpace(out))
		}
		return err
	}
	dir := filepath.Join(r.home, "watch", "work", j.Head.SHA[:12])
	if err := git(ctx, "fetch", "--quiet", "--no-tags", "origin", j.Head.SHA); err != nil {
		return watch.Outcome{}, err
	}
	if err := git(ctx, "worktree", "add", "--detach", "--force", dir, j.Head.SHA); err != nil {
		return watch.Outcome{}, err
	}
	defer func() { _ = git(context.WithoutCancel(ctx), "worktree", "remove", "--force", dir) }()
	jobs := j.Only
	if len(jobs) == 0 {
		jobs = []string{""}
	}
	worst := watch.Outcome{State: report.Success, Description: "passed"}
	for _, job := range jobs {
		o := RunOptions{Root: dir, Home: r.home, Job: job, Trigger: "api", Record: true, NoStatus: true}
		if err := o.Validate(cirun.Hooks{}.Resolved()); err != nil {
			return watch.Outcome{}, err
		}
		p, digest, subst, err := preparePlan(ctx, o)
		if err != nil {
			return watch.Outcome{}, err
		}
		c := &cirun.Coordinator{Opts: o, Plan: p, ConfigDigest: digest, ConfigOutside: configOutside(ctx, dir), Substantive: subst, Out: os.Stderr}
		res, err := c.Run(ctx)
		switch {
		case err != nil:
			return watch.Outcome{}, err
		case res.Exit == 1:
			worst = watch.Outcome{State: report.Failure, Description: "gate failed (run " + res.RunID + ")"}
		case res.Exit != 0 && worst.State == report.Success:
			worst = watch.Outcome{State: report.Error, Description: "infrastructure error (run " + res.RunID + ")"}
		}
	}
	return worst, nil
}
