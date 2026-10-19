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

// TestForegroundQueueCap: the 10 minute foreground cap (a per-run value) lowers
// the dispatcher's max queue; a run without per-run values keeps no limit (0).
func TestForegroundQueueCap(t *testing.T) {
	root, st, _ := runScopeRepo(t)
	flags := schedpolicy.Flags{Foreground: true, Runner: "ms5"}
	vals, err := flags.Values()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		run    RunValues
		queue  int64
		runner string
	}{
		{"interactive", vals, 600000, "ms5"},
		{"scripted", nil, 0, ""},
	} {
		rd := &runDispatch{sess: &Session{Store: st, PolicyRun: tc.run}, row: store.PipelineRow{Project: root}}
		p, err := rd.policy(context.Background(), "")
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if p.Overrides.MaxQueueMs != tc.queue || p.Overrides.Runner != tc.runner {
			t.Errorf("%s: max queue %d runner %q, want %d %q", tc.name, p.Overrides.MaxQueueMs, p.Overrides.Runner, tc.queue, tc.runner)
		}
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
	got := ignoredRunNotes([]string{"sched.overrides.max_queue: E671 run scope widening", "sched.overrides.runner: E671 run scope widening"}, []string{"sched.overrides.max_queue"})
	if len(got) != 1 || got[0] != "per-run policy value ignored: sched.overrides.runner: E671 run scope widening" {
		t.Errorf("notes = %q", got)
	}
}
