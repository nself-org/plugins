package run

// Purpose: the per-run policy values reach both policy resolutions of a run
// (P7-AUTO-29, D-0984): the pipeline's policy digest (PolicyHook) and the
// placement policy of the dispatcher (runDispatch.policy), and what the policy
// refuses is reported, not dropped.

import (
	"context"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/report"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/schedpolicy"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func runScopeRepo(t *testing.T) (root string, st *store.Store, db string) {
	t.Helper()
	root = t.TempDir()
	if out, err := exec.Command("git", "init", "-q", "-b", "main", root).CombinedOutput(); err != nil {
		t.Fatalf("git init: %s %v", out, err)
	}
	db = filepath.Join(t.TempDir(), "state.db")
	st, err := store.Open(db, store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return root, st, db
}

// TestForegroundQueueCap: the 10 minute foreground queue cap is the dispatcher's
// default for an interactive run whose policy sets no max_queue; it never
// shortens a longer limit the project set, and a scripted run asks the
// dispatcher to withhold it. --max-queue lowers the policy value.
func TestForegroundQueueCap(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name        string
		project     string // the project's max_queue ("" = none)
		interactive bool
		flags       schedpolicy.Flags
		queue       int64
		runner      string
	}{
		{"interactive, nothing set: the dispatcher default applies", "", true, schedpolicy.Flags{}, 0, ""},
		{"scripted, nothing set", "", false, schedpolicy.Flags{}, 0, ""},
		{"interactive keeps a longer project limit", `"1h"`, true, schedpolicy.Flags{}, 3600000, ""},
		{"--max-queue 5m and --runner ms5", "", true, schedpolicy.Flags{MaxQueue: "5m", Runner: "ms5"}, 300000, "ms5"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, st, _ := runScopeRepo(t)
			if tc.project != "" {
				project := policy.ProjectIDGitCached(ctx, policy.Runner(report.Capture), root)
				err := st.ApplyWrite(ctx, store.PolicyWrite{Op: store.PolicySet, Scope: "project", Subject: project, Key: schedpolicy.KeyMaxQueue, Value: []byte(tc.project), Actor: "test"}, nil)
				if err != nil {
					t.Fatal(err)
				}
			}
			vals, err := tc.flags.Values()
			if err != nil {
				t.Fatal(err)
			}
			sess := &Session{Store: st, PolicyRun: vals, Interactive: tc.interactive}
			rd := &runDispatch{sess: sess, row: store.PipelineRow{Project: root}}
			p, err := rd.policy(ctx, "")
			if err != nil {
				t.Fatal(err)
			}
			if p.Overrides.MaxQueueMs != tc.queue || p.Overrides.Runner != tc.runner {
				t.Errorf("max queue %d runner %q, want %d %q", p.Overrides.MaxQueueMs, p.Overrides.Runner, tc.queue, tc.runner)
			}
			if j := queueJob(sess, AttemptSpec{Job: &model.Job{}, Foreground: true}, sched.Job{}, "a", 1, 1); j.NoDefaultQueueCap == tc.interactive {
				t.Errorf("interactive %v but NoDefaultQueueCap %v", tc.interactive, j.NoDefaultQueueCap)
			}
		})
	}
}

// TestPolicyHookReportsIgnoredRunValues: a project-pinned runner beats `--runner`
// and the hook lists the refusal; the digest carries the values that applied.
func TestPolicyHookReportsIgnoredRunValues(t *testing.T) {
	ctx := context.Background()
	root, st, db := runScopeRepo(t)
	project := policy.ProjectIDGitCached(ctx, policy.Runner(report.Capture), root)
	err := st.ApplyWrite(ctx, store.PolicyWrite{Op: store.PolicySet, Scope: "project", Subject: project, Key: schedpolicy.KeyRunner, Value: []byte(`"ms6"`), Actor: "test"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	hook := PolicyHook{OpenStore: func() (*store.Store, error) { return store.Open(db, store.Options{}) }, Capture: report.Capture}
	none, err := hook.Resolve(ctx, PolicyRequest{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(none.IgnoredRun) != 0 {
		t.Fatalf("a run without values ignored %v", none.IgnoredRun)
	}
	same, err := hook.Resolve(ctx, PolicyRequest{Root: root, Run: RunValues{schedpolicy.KeyRunner: json.RawMessage(`"ms5"`)}})
	if err != nil {
		t.Fatal(err)
	}
	if same.Digest != none.Digest || len(same.IgnoredRun) != 1 {
		t.Errorf("an ignored value changed the digest or went unreported: %v", same.IgnoredRun)
	}
	lower, err := hook.Resolve(ctx, PolicyRequest{Root: root, Run: RunValues{schedpolicy.KeyMaxQueue: json.RawMessage(`"5m"`)}})
	if err != nil || len(lower.IgnoredRun) != 0 {
		t.Errorf("a lower max queue must apply: %v %v", lower.IgnoredRun, err)
	}
	// placement values do not enter the digest: `ci evidence verify` re-resolves without them
	if lower.Digest != none.Digest {
		t.Error("a per-run value changed the policy digest of the run (its evidence would not verify)")
	}
}

// TestQueueJobDefaultCap: only an interactive run keeps the implicit foreground
// queue cap; the cron or scripted run's attempt asks the dispatcher to withhold it.
func TestQueueJobDefaultCap(t *testing.T) {
	job := model.Job{}
	for _, interactive := range []bool{true, false} {
		sess := &Session{Interactive: interactive, Revision: "abc"}
		got := queueJob(sess, AttemptSpec{Job: &job, Foreground: true}, sched.Job{}, "a1", 2, 512)
		if got.NoDefaultQueueCap == interactive || !got.Foreground || got.Revision != "abc" || got.LocalCPU != 2 || got.LocalMemMB != 512 {
			t.Errorf("interactive %v: %+v", interactive, got)
		}
	}
}

func TestIgnoredRunNotes(t *testing.T) {
	got := ignoredRunNotes([]string{"sched.overrides.runner: E819 run scope widening"})
	if len(got) != 1 || got[0] != "per-run policy value ignored: sched.overrides.runner: E819 run scope widening" {
		t.Errorf("notes = %q", got)
	}
}

type ignoredPolicy struct{ lines []string }

func (p ignoredPolicy) Resolve(ctx context.Context, r PolicyRequest) (PolicyResult, error) {
	got, err := defaultResolver{}.Resolve(ctx, r)
	got.IgnoredRun = p.lines
	return got, err
}

// TestIgnoredRunValuesReachSummary: what the policy refused of the run's flags is
// a note of the finished run (never a silent drop).
func TestIgnoredRunValuesReachSummary(t *testing.T) {
	r := passRig(t)
	r.C.Hooks.Policy = ignoredPolicy{[]string{"sched.overrides.runner: E819 run scope widening"}}
	res := r.run(t)
	var got []string
	for _, n := range res.Summary.Notes {
		if strings.Contains(n, "per-run policy value ignored") {
			got = append(got, n)
		}
	}
	if len(got) != 1 || !strings.Contains(got[0], "sched.overrides.runner") {
		t.Fatalf("notes = %q", res.Summary.Notes)
	}
}

// TestPolicyHookReadsTheRequestStore: the hook reads the operator rows from the
// store the request names (the coordinator's home), not the default one.
func TestPolicyHookReadsTheRequestStore(t *testing.T) {
	ctx := context.Background()
	t.Setenv("NSELF_CI_HOME", t.TempDir()) // the default store: empty
	root, st, db := runScopeRepo(t)
	project := policy.ProjectIDGitCached(ctx, policy.Runner(report.Capture), root)
	err := st.ApplyWrite(ctx, store.PolicyWrite{Op: store.PolicySet, Scope: "project", Subject: project, Key: schedpolicy.KeyRunner, Value: []byte(`"ms6"`), Actor: "test"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	run := RunValues{schedpolicy.KeyRunner: json.RawMessage(`"ms5"`)}
	got, err := PolicyHook{}.Resolve(ctx, PolicyRequest{Root: root, Store: db, Run: run})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.IgnoredRun) != 1 { // only the pin in the request's store makes --runner ms5 refused
		t.Fatalf("the project pin of the request store was not read: ignored %v", got.IgnoredRun)
	}
}
