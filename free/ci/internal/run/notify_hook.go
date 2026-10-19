package run

// Purpose: the NotifyHook slot filled (P7-AUTO-18): when a run ends, tell the operator's
// notifier sinks (internal/notify) about a failure, an infrastructure error or a recovery,
// and send the daily digest. This is the only file of the package that imports internal/notify.
// Inputs: a NotifyRequest (the finished run). The operator's notify.* policy keys, resolved
// from the default state database like PolicyHook does; the branch and origin of the run's root.
// Outputs: sink deliveries, and one error line (a note in the run summary) naming the sinks
// that did not take a message. Nothing else: the run's verdict, exit code, status and
// duration never depend on a sink.
// Constraints: only under compat.V15(); inert (no store write, no network, no process) while
// notify.sinks is empty; registered from init(), like the other hooks of this package.
// Delivery is bounded by notify.DefaultBudget, so a hung sink delays the end of a run by at
// most that long, and is cancelled and counted as failed. The event carries ids, counts and
// the result, never output.

import (
	"context"
	"errors"
	"strings"
	"time"

	compat "github.com/nself-org/cli/sdk/go/v2/compat"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/notify"
	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/report"
)

func init() { RegisterNotify(&notifyHook{}) }

// notifyHook implements NotifyHook. The zero value is production; tests set the seams.
type notifyHook struct {
	// Load returns the notify config, the state it keeps and a release func (nil = resolve the
	// policy against the default store).
	Load func(ctx context.Context, root string) (notify.Config, notify.State, func(), error)
	// Ref names the branch the run was on ("" = ask git).
	Ref     func(ctx context.Context) string
	Deps    notify.Deps
	Budget  time.Duration
	After   func(time.Duration) <-chan time.Time
	Now     func() time.Time
	Capture Capture
	// Enabled says whether v1.5 behaviour is on (nil = compat.V15).
	Enabled func() bool
}

// Notify implements NotifyHook.
func (h *notifyHook) Notify(ctx context.Context, r NotifyRequest) error {
	if enabled := h.Enabled; (enabled == nil && !compat.V15()) || (enabled != nil && !enabled()) {
		return nil
	}
	load := h.Load
	if load == nil {
		load = h.loadPolicy
	}
	cfg, state, release, err := load(ctx, r.Root)
	if release != nil {
		defer release()
	}
	if err != nil {
		return errors.New("notify settings not read, nothing sent: " + firstLineOf(err.Error()))
	}
	if len(cfg.Sinks) == 0 {
		return nil
	}
	deps := h.Deps
	deps.AllowPrivate = cfg.AllowPrivate
	sinks, errs := notify.Build(cfg.Sinks, deps)
	var notes []string
	for _, e := range errs {
		notes = append(notes, firstLineOf(e.Error()))
	}
	eng := &notify.Engine{Sinks: sinks, Quiet: cfg.Quiet, DailyDigest: cfg.Digest, State: state, Now: h.Now, Budget: h.Budget, After: h.After}
	if r.Run.Dirty || r.Run.Revision == "" { // a dev run on a dirty tree has no commit of record: its sha must not claim an alert key
		return nil
	}
	notes = append(notes, eng.OnRun(ctx, h.runOf(ctx, r))...)
	if len(notes) == 0 {
		return nil
	}
	return errors.New(strings.Join(notes, "; "))
}

// runOf maps a finished run to what the notify engine judges.
func (h *notifyHook) runOf(ctx context.Context, req NotifyRequest) notify.Run {
	info := req.Run
	ref, repo := "", info.Evidence.RepoID
	if h.Ref != nil {
		ref = h.Ref(ctx)
	} else {
		ref = h.branch(ctx, req.Root)
	}
	if id := h.remoteID(ctx, req.Root); id != "" {
		repo = id
	}
	var failed []string
	for _, j := range info.Jobs {
		switch model.JobState(j.State) {
		case "failed", "lost", "needs-operator":
			failed = append(failed, j.Name)
		}
	}
	return notify.Run{Repo: repo, Ref: ref, Revision: info.Revision, Context: report.Context, RunID: info.RunID,
		EvidenceDigest: info.EvidenceDigest, Result: string(info.Result), Counts: info.Evidence.Counts, FailedJobs: failed}
}

func (h *notifyHook) capture() Capture {
	if h.Capture != nil {
		return h.Capture
	}
	return report.Capture
}

// branch is the checked-out branch of the run's root ("" when detached or unknown: such a
// run alerts on failure but has no ref to recover on).
func (h *notifyHook) branch(ctx context.Context, root string) string {
	if root == "" {
		return ""
	}
	out, exit, err := h.capture()(ctx, "git", "-c", "core.fsmonitor=false", "-C", root, "symbolic-ref", "--short", "-q", "HEAD")
	if err != nil || exit != 0 {
		return ""
	}
	return strings.TrimSpace(out)
}

// remoteID names the repository by a digest of its origin URL, which is the same for every
// clone of it (a served run's root changes each run); "" when there is no origin. The URL
// itself never leaves the host.
func (h *notifyHook) remoteID(ctx context.Context, root string) string {
	if root == "" {
		return ""
	}
	out, exit, err := h.capture()(ctx, "git", "-c", "core.fsmonitor=false", "-C", root, "config", "--get", "remote.origin.url")
	if url := strings.TrimSpace(out); err == nil && exit == 0 && url != "" {
		return shortDigest(url)
	}
	return ""
}

// loadPolicy resolves the operator's notify.* keys against the default state database.
func (h *notifyHook) loadPolicy(ctx context.Context, root string) (notify.Config, notify.State, func(), error) {
	st, err := OpenStore("")
	if err != nil {
		return notify.Config{}, nil, nil, err
	}
	release := func() { _ = st.Close() }
	if root == "" {
		root = "."
	}
	run := policy.Runner(h.capture())
	res := policy.Resolver{Store: st, Run: run}
	got, err := res.Resolve(ctx, policy.Repo{Root: root, Project: policy.ProjectIDGitCached(ctx, run, root), Visibility: "unknown", Class: "owner"}, policy.Revision{})
	if err != nil {
		return notify.Config{}, nil, release, err
	}
	cfg, err := notify.ConfigFrom(&got.Effective)
	return cfg, notify.StoreState{S: st}, release, err
}
