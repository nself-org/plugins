package main

// Purpose: `nself ci doctor --selftest [--alert]` (P7-AUTO-20): registers the selftest doctor
// section (checks CI-DS001…) and the alerter, and wires the real host to internal/selftest.
// Inputs:  the doctor Env (repository root, process runner, environment) and the run store.
// Outputs: doctor checks; with --alert one notifier message per failing set per day.
// Constraints: read-only; the store is opened only when state.db exists and never created or
// recovered; an unreadable source is a warning on its own check, not a failure of the report.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/doctor"
	"github.com/nself-org/plugins/free/ci/internal/evidence"
	"github.com/nself-org/plugins/free/ci/internal/history"
	"github.com/nself-org/plugins/free/ci/internal/notify"
	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/readiness/sample"
	"github.com/nself-org/plugins/free/ci/internal/report"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/selftest"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func init() {
	doctor.RegisterSection(doctor.Section{Name: "selftest", Selector: doctor.SelectSelftest, Run: selftestRun})
	doctor.SetAlerter(selftestAlert)
}

// openStateReadOnly opens the run store when it exists; nil when there is none.
func openStateReadOnly() (*store.Store, error) {
	if !evidence.StoreExists(evidence.StorePath()) {
		return nil, nil
	}
	return cirun.OpenStore(runsStorePath())
}

// selftestLocal wires the real host. st may be nil (no store on this machine).
func selftestLocal(env doctor.Env, st *store.Store) selftest.Local {
	l := selftest.Local{Root: env.Root, Home: sample.CIHome(), Path: env.Getenv("PATH"), Capture: env.Capture}
	if st == nil {
		return l
	}
	l.Sinks = func(ctx context.Context) ([]selftest.Sink, error) {
		stats, err := st.NotifySinkStats(ctx)
		out := make([]selftest.Sink, 0, len(stats))
		for _, s := range stats {
			sink := selftest.Sink{Name: s.Sink, Delivered: s.Delivered, Failed: s.Failed}
			if s.LastFailedAt > 0 {
				sink.LastFailedAt = time.Unix(0, s.LastFailedAt).UTC()
			}
			out = append(out, sink)
		}
		return out, err
	}
	l.Infra = func(ctx context.Context, since time.Time) (count, total int, err error) {
		entries, err := history.List(ctx, history.Env{Store: st, Root: env.Root, Home: os.Getenv("NSELF_CI_HOME")}, history.MaxLimit)
		for _, e := range entries {
			at, perr := time.Parse(time.RFC3339Nano, e.StartedAt)
			if perr != nil || at.Before(since) {
				continue
			}
			total++
			if e.Result == "error" {
				count++
			}
		}
		return count, total, err
	}
	return l
}

func selftestRun(ctx context.Context, env doctor.Env) []doctor.Check {
	st, err := openStateReadOnly()
	if st != nil {
		defer func() { _ = st.Close() }()
	}
	l := selftestLocal(env, st)
	if err != nil { // the store exists but cannot be opened: its three facts are unreadable, the rest still count
		bad := func(context.Context) error { return err }
		l.Sinks = func(ctx context.Context) ([]selftest.Sink, error) { return nil, bad(ctx) }
		l.Infra = func(ctx context.Context, _ time.Time) (int, int, error) { return 0, 0, bad(ctx) }
	}
	return selftest.Evaluate(l.Facts(ctx), selftest.DefaultLimits())
}

// policyEffective resolves the operator policy for root against the default store.
func policyEffective(ctx context.Context, root string) (policy.Effective, func(), error) {
	st, err := cirun.OpenStore("")
	if err != nil {
		return policy.Effective{}, func() {}, err
	}
	release := func() { _ = st.Close() }
	run := policy.Runner(report.Capture)
	res := policy.Resolver{Store: st, Run: run}
	got, err := res.Resolve(ctx, policy.Repo{Root: root, Project: policy.ProjectIDGitCached(ctx, run, root), Visibility: "unknown", Class: "owner"}, policy.Revision{})
	return got.Effective, release, err
}

func selftestAlert(ctx context.Context, env doctor.Env, checks []doctor.Check) (string, error) {
	eff, release, err := policyEffective(ctx, env.Root)
	defer release()
	if err != nil {
		return "", err
	}
	cfg, err := notify.ConfigFrom(&eff)
	if err != nil {
		return "", err
	}
	sinks, errs := notify.Build(cfg.Sinks, notify.Deps{AllowPrivate: cfg.AllowPrivate})
	host, _ := os.Hostname()
	a := selftest.Alerter{Sinks: sinks, Host: host, StatePath: filepath.Join(sample.CIHome(), "selftest-alert.json")}
	res, aerr := a.Alert(ctx, checks)
	for _, e := range errs {
		if aerr == nil {
			aerr = e
		}
	}
	switch {
	case res.Sent:
		return "alert sent for " + res.Set, aerr
	case res.Skipped != "" && res.Set != "":
		return "alert not sent for " + res.Set + ": " + res.Skipped, aerr
	}
	return strings.TrimSpace(res.Skipped), aerr
}
