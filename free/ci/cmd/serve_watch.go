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
	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/providers/github/reconcile"
	"github.com/nself-org/plugins/free/ci/internal/readiness"
	"github.com/nself-org/plugins/free/ci/internal/readiness/sample"
	"github.com/nself-org/plugins/free/ci/internal/report"
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
