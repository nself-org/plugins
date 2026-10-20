//go:build integration

package reliability

// Purpose: runs one pipeline of the lab with the real run loop: a git repository with one commit, a plan with the
//          fixture job (remote only, one infra retry) and the dispatcher placing it on the agent nodes.
// Inputs:  the job shape (iterations of work, idempotent or not, release kind).
// Outputs: a run handle whose result, attempts and evidence the tests assert on.
// Constraints: the run opens its own handle on the lab's store file; nothing here sleeps without a deadline.

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/nself-org/plugins/free/ci/internal/transport/agent"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/plan"
	"github.com/nself-org/plugins/free/ci/internal/report"
	"github.com/nself-org/plugins/free/ci/internal/run"
	_ "github.com/nself-org/plugins/free/ci/internal/schedtel" // the binary registers the telemetry sink the run records outcomes through
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// jobSpec is the fixture job: it records its start, works for iter x 200 ms (a beat per iteration) and then
// appends one line to /tmp/side-effect. A job killed before the end never writes the side effect.
type jobSpec struct {
	iter       int
	kind       model.JobKind // "test" unless set
	idempotent bool
	infraMax   int
	failExit   int // non-zero: exit with this status instead of writing the side effect
	// retryIter, when set, is the iteration count on a node that already ran a job (its /tmp/starts exists): a
	// retry then ends quickly while the first attempts of the fleet outlive a long setup and a long outage.
	retryIter int
	name      string
}

// label is what the job writes to the side-effect file (its name), so a retry on another node is told apart.
func (j jobSpec) label() string {
	if j.name == "" {
		return "unit"
	}
	return j.name
}

func (j jobSpec) script() string {
	tail := `echo ` + j.label() + ` >> /tmp/side-effect`
	if j.failExit != 0 {
		tail = `exit ` + itoa(j.failExit)
	}
	n := itoa(j.iter)
	pick := ""
	if j.retryIter > 0 {
		pick = `n=` + n + `; [ -e /tmp/starts ] && n=` + itoa(j.retryIter) + `; `
		n = `$n`
	}
	return pick + `echo $(date +%s%N) >> /tmp/starts; i=0; while [ $i -lt ` + n + ` ]; do echo $(date +%s%N) >> /tmp/beats; echo tick-$i; sleep 0.2; i=$((i+1)); done; ` + tail
}

func (j jobSpec) job() model.Job {
	yes := true
	idem := j.idempotent
	kind := j.kind
	if kind == "" {
		kind = "test"
	}
	return model.Job{Kind: kind, Run: []string{"sh", "-c", j.script()}, Required: &yes, RemoteOnly: true, Idempotent: &idem, Timeout: "5m", Retry: &model.RetryConfig{InfraMax: j.infraMax}}
}

// runHandle is one run in flight.
type runHandle struct {
	l    *lab
	sha  string
	ch   chan run.Result
	out  *lockedBuffer
	stop context.CancelFunc
}

// initRepo creates the git repository every run of the lab uses; nodes are authorized for its path.
func (l *lab) initRepo() {
	dir := l.t.TempDir()
	l.root = dir
	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("x"), 0o600); err != nil {
		l.t.Fatal(err)
	}
	l.git("init", "-q")
	l.git("add", "f.txt")
}

func (l *lab) git(args ...string) string {
	cmd := exec.Command("git", args...)
	cmd.Dir = l.root
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.invalid", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.invalid")
	out, err := cmd.CombinedOutput()
	if err != nil {
		l.t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// commit makes a new commit (so a run has its own revision) and returns its id.
func (l *lab) commit() string {
	l.git("commit", "-q", "--allow-empty", "-m", "run "+itoa(int(l.runs.Add(1))))
	return l.git("rev-parse", "HEAD")
}

type noReport struct{}

func (noReport) Post(context.Context, report.Status) error { return nil }

// start begins a run of the fixture job and returns its handle; the run is cancelled by t.Cleanup.
func (l *lab) start(spec jobSpec) *runHandle { return l.startJobs(1, spec) }

// startJobs begins a run of count independent copies of the fixture job (unit when one, unit01.. otherwise).
func (l *lab) startJobs(count int, spec jobSpec) *runHandle {
	t := l.t
	t.Helper()
	root, sha := l.root, l.commit()
	jobs := map[string]model.Job{}
	for i := 1; i <= count; i++ {
		name := "unit"
		if count > 1 {
			name = fmt.Sprintf("unit%02d", i)
		}
		js := spec
		js.name = name
		jobs[name] = js.job()
	}
	pl, err := plan.Build(plan.Input{Config: model.PipelineConfig{Version: 1, Jobs: jobs},
		World: plan.World{Platform: hostPlatform(), CPU: 4, MemMB: 8192}})
	if err != nil {
		t.Fatal(err)
	}
	out := &lockedBuffer{}
	capture := func(_ context.Context, argv ...string) (string, int, error) {
		switch j := strings.Join(argv, " "); {
		case strings.Contains(j, "rev-parse HEAD"):
			return sha + "\n", 0, nil
		case strings.Contains(j, "remote get-url"):
			return "", 1, nil
		}
		return "", 0, nil
	}
	c := &run.Coordinator{Opts: run.Options{Root: root, Home: l.c.home, NoStatus: true}, Plan: pl, ConfigDigest: "cfg", Out: out, Capture: capture, Reporter: noReport{},
		Capacity: func() ciexec.Capacity {
			return ciexec.Capacity{CPUs: 8, MemoryMB: 16384, FreeMemoryMB: 12000, BatteryPercent: -1, PluggedIn: true, Interactive: true}
		}, Poll: 10 * time.Millisecond}
	ctx, cancel := context.WithCancel(context.Background())
	h := &runHandle{l: l, sha: sha, ch: make(chan run.Result, 1), out: out, stop: cancel}
	t.Cleanup(cancel)
	go func() {
		res, rerr := c.Run(ctx)
		t.Logf("fixture job outcomes: %+v", res.Summary.Jobs)
		if rerr != nil && ctx.Err() == nil {
			t.Errorf("Run: %v\n%s", rerr, out)
		}
		h.ch <- res
	}()
	return h
}

// result waits for the run to finish.
func (h *runHandle) result(within time.Duration) run.Result {
	h.l.t.Helper()
	select {
	case r := <-h.ch:
		return r
	case <-time.After(within):
		h.l.t.Fatalf("the run did not finish within %v\n%s\n%s", within, h.out, h.l.diag())
		return run.Result{}
	}
}

// attempts lists the run's attempts, job then n. The run may not have created its pipeline yet: nil then.
func (h *runHandle) attempts() []store.AttemptView {
	ctx := context.Background()
	runs, err := h.l.c.st.PipelinesByRevision(ctx, h.sha)
	if err != nil || len(runs) == 0 {
		return nil
	}
	atts, err := h.l.c.st.PipelineAttempts(ctx, runs[0].ID)
	if err != nil {
		h.l.t.Fatal(err)
	}
	return atts
}

// waitAttempt waits for attempt n to be in one of states and returns the attempts at that moment.
func (h *runHandle) waitAttempt(n int, within time.Duration, states ...string) []store.AttemptView {
	h.l.t.Helper()
	var got []store.AttemptView
	until(h.l.t, within, "attempt "+itoa(n)+" to be "+strings.Join(states, " or "), func() bool {
		got = h.attempts()
		for _, a := range got {
			if a.N == n {
				for _, s := range states {
					if a.State == s {
						return true
					}
				}
			}
		}
		return false
	}, func() string { return fmt.Sprintf("attempts: %+v\n", got) + h.out.String() + h.l.diag() })
	return got
}

// runnerOf is the node the attempt holds (or held) its lease on: the lease row while it exists.
func (h *runHandle) runnerOf(attemptID string) string {
	h.l.t.Helper()
	le, err := h.l.c.st.LeaseGet(context.Background(), "lease-"+attemptID)
	if err != nil {
		h.l.t.Fatalf("lease of %s: %v", attemptID, err)
	}
	return le.RunnerID
}

// previousOf is the runner recorded for retry anti-affinity: the node the attempt before this one was lost on.
func (h *runHandle) previousOf(attemptID string) string {
	h.l.t.Helper()
	r, err := h.l.c.st.LeasePreviousRunner(context.Background(), attemptID)
	if err != nil {
		h.l.t.Fatal(err)
	}
	return r
}

// diagText is the run output and the agent logs, for a failure message.
func (h *runHandle) diagText() string { return h.out.String() + h.l.diag() }

// waitRenewed waits until the agent's heartbeat has renewed the attempt's lease at least once. The grant gives
// the lease the dispatcher's own TTL; from the first heartbeat on, the lease lives coordinator_ttl past the last one.
func (h *runHandle) waitRenewed(attemptID string) {
	h.l.t.Helper()
	until(h.l.t, 30*time.Second, "the first lease renewal", func() bool {
		le, err := h.l.c.st.LeaseGet(context.Background(), "lease-"+attemptID)
		return err == nil && time.Until(time.Unix(0, le.ExpiresAt)) <= h.l.c.p.ttl+time.Second
	}, h.diagText)
}

// decision is the stored placement decision of the attempt as JSON (what `ci explain` prints).
func (h *runHandle) decision(attemptID string) string {
	h.l.t.Helper()
	d, ok, err := h.l.c.st.TelDecision(context.Background(), attemptID)
	if err != nil || !ok {
		h.l.t.Fatalf("no placement decision for %s (found %v): %v", attemptID, ok, err)
	}
	return string(d.Doc)
}

// leaseLog is the coordinator's durable log of the attempt's lease: the text of every line, in order.
func (h *runHandle) leaseLog(attemptID string) []string {
	h.l.t.Helper()
	raw, err := os.ReadFile(agent.LogFile(h.l.c.home+"/agent-logs", "lease-"+attemptID))
	if err != nil {
		h.l.t.Fatalf("lease log of %s: %v", attemptID, err)
	}
	var out []string
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		var rec struct {
			LineSeq uint64 `json:"line_seq"`
			Text    string `json:"text"`
		}
		if err = json.Unmarshal([]byte(line), &rec); err != nil {
			h.l.t.Fatalf("lease log of %s: %q: %v", attemptID, line, err)
		}
		out = append(out, strings.TrimRight(rec.Text, "\n"))
	}
	return out
}

// leaseLogQuiet is leaseLog for a failure message: an unreadable log is a short text, never a fatal.
func (h *runHandle) leaseLogQuiet(attemptID string) string {
	raw, err := os.ReadFile(agent.LogFile(h.l.c.home+"/agent-logs", "lease-"+attemptID))
	if err != nil {
		return "(" + err.Error() + ")"
	}
	return string(raw)
}

// assertTicks fails unless the log holds tick-0 .. tick-(n-1) exactly once each and in order: no duplicate
// and no missing line after a resend.
func assertTicks(t *testing.T, log []string, n int) {
	t.Helper()
	var ticks []string
	for _, l := range log {
		if strings.HasPrefix(l, "tick-") {
			ticks = append(ticks, l)
		}
	}
	if len(ticks) != n {
		t.Fatalf("the log has %d tick lines, want %d: %v", len(ticks), n, ticks)
	}
	for i, l := range ticks {
		if l != "tick-"+itoa(i) {
			t.Fatalf("tick line %d is %q: a line was duplicated or lost: %v", i, l, ticks)
		}
	}
}
