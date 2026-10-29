package sim

// Purpose: P7-AUTO-13 acceptance on a simulated node. A laptop tree with uncommitted work runs
// on a node: the dirty tree becomes a snapshot commit, the real ssh driver bundles it, the
// node-side checkout receiver verifies and unpacks it, and the node's job sees exactly the
// user's tree. No Docker, no network, no real ssh.
// Inputs:  a real git repository with a modified tracked file, an untracked file, an ignored
//          .env and an untracked .env.local; two registered nodes (the foreground fixture).
// Outputs: what the node staged, the run result, and the repository state before and after.
// Constraints: NSELF_V15=1 (set by the fixture); the user's git status, index and refs are
//              byte-identical after the run, after a failing job and after a cancel; the
//              private offload ref is gone; secret file content never reaches the report.

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/agent/checkout"
	"github.com/nself-org/plugins/free/ci/internal/agent/session"
	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/offload"
	"github.com/nself-org/plugins/free/ci/internal/plan"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/transport/ssh"
)

const (
	secretEnv   = "ENV_VALUE_MUST_NOT_TRAVEL"
	secretLocal = "ENV_LOCAL_VALUE_MUST_NOT_TRAVEL"
)

// realGit is the run.Capture of a real process: the snapshot builder and GitRevision need real git.
func realGit(ctx context.Context, argv ...string) (string, int, error) {
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	var b bytes.Buffer
	cmd.Stdout, cmd.Stderr = &b, &b
	err := cmd.Run()
	if ee, ok := err.(*exec.ExitError); ok {
		return b.String(), ee.ExitCode(), nil
	}
	return b.String(), 0, err
}

// stageCarrier is the node side of the checkout: lease announcements and the bundle's chunk
// frames go to the receiver, as `serve --stdio` does, and never reach the session.
type stageCarrier struct {
	session.Carrier
	r *checkout.Receiver
}

func (c *stageCarrier) Recv() (protocol.Message, error) {
	for {
		m, err := c.Carrier.Recv()
		if err != nil {
			return m, err
		}
		switch b := m.Body.(type) {
		case *protocol.Lease:
			_ = c.r.Expect(*b)
		case *protocol.Chunk:
			if b.Stream != checkout.Stream {
				return m, nil
			}
			epoch := uint64(0)
			if m.Epoch != nil {
				epoch = *m.Epoch
			}
			_ = c.r.Feed(m.LeaseID, epoch, *b)
			continue
		}
		return m, nil
	}
}

// seen is what a node's job found in the tree it was given.
type seen struct {
	err   error
	head  string
	files map[string]string // name -> content, for the names asked about; absent names are missing
}

// offloadNodes gives every node a receiver and returns a runner factory that stages the lease's
// bundle, records what the job sees, then calls body.
type offloadNodes struct {
	mu    sync.Mutex
	recv  map[string]*checkout.Receiver
	seen  chan seen
	probe []string
}

func newOffloadNodes(f *fixture, probe ...string) *offloadNodes {
	o := &offloadNodes{recv: map[string]*checkout.Receiver{}, seen: make(chan seen, 4), probe: probe}
	for _, n := range f.nodes {
		n.wrap = func(c session.Carrier) session.Carrier {
			r := checkout.NewReceiver(filepath.Join(n.state, "checkout"), nil)
			o.mu.Lock()
			o.recv[n.id] = r
			o.mu.Unlock()
			return &stageCarrier{Carrier: c, r: r}
		}
	}
	return o
}

func (o *offloadNodes) runner(body func(context.Context, session.JobRun) (session.JobResult, error)) func(context.Context, *node, session.JobRun) (session.JobResult, error) {
	return func(ctx context.Context, n *node, j session.JobRun) (session.JobResult, error) {
		o.mu.Lock()
		r := o.recv[n.id]
		o.mu.Unlock()
		s := seen{files: map[string]string{}}
		src, err := r.Stage(ctx, j.Lease, j.Root.Dir)
		if err != nil {
			s.err = err
			o.seen <- s
			return session.JobResult{}, err
		}
		if out, e := exec.Command("git", "-C", src, "rev-parse", "HEAD").Output(); e == nil {
			s.head = strings.TrimSpace(string(out))
		}
		for _, name := range o.probe {
			if b, e := os.ReadFile(filepath.Join(src, name)); e == nil {
				s.files[name] = string(b)
			}
		}
		o.seen <- s
		return body(ctx, j)
	}
}

func (o *offloadNodes) wait(t *testing.T) seen {
	t.Helper()
	select {
	case s := <-o.seen:
		return s
	case <-time.After(60 * time.Second):
		t.Fatal("no node staged the checkout")
	}
	panic("unreachable")
}

// dirtyFixture is the foreground fixture plus a user's repository with uncommitted work.
func dirtyFixture(t *testing.T, body func(context.Context, session.JobRun) (session.JobResult, error)) (*fixture, *offloadNodes) {
	t.Helper()
	probe := []string{"f.txt", "new.txt", ".env", ".env.local"}
	f := newFixture(t, nil)
	nodes := newOffloadNodes(f, probe...)
	for _, n := range f.nodes {
		n.runner = nodes.runner(body)
	}
	write := func(name, content string) {
		if err := os.WriteFile(filepath.Join(f.repo, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(".gitignore", ".env\nignored.txt\n")
	gitIn(t, f.repo, "add", ".gitignore")
	gitIn(t, f.repo, "commit", "-q", "-m", "ignore")
	f.sha = gitIn(t, f.repo, "rev-parse", "HEAD")
	write("f.txt", "edited on the laptop\n")
	write("new.txt", "untracked and not ignored\n")
	write(".env", secretEnv+"\n")
	write(".env.local", secretLocal+"\n")
	write("ignored.txt", "ignored\n")
	return f, nodes
}

// repoState is everything about the user's repository an offload must leave alone.
func repoState(t *testing.T, repo string) string {
	t.Helper()
	parts := []string{
		gitIn(t, repo, "status", "--porcelain=v2", "--ignored"),
		gitIn(t, repo, "for-each-ref"),
		gitIn(t, repo, "ls-files", "-s"),
		gitIn(t, repo, "rev-parse", "HEAD"),
	}
	for _, name := range []string{"f.txt", "new.txt", ".env", ".env.local", "ignored.txt", ".gitignore"} {
		b, err := os.ReadFile(filepath.Join(repo, name))
		if err != nil {
			t.Fatal(err)
		}
		parts = append(parts, name+"="+string(b))
	}
	return strings.Join(parts, "\n--\n")
}

// startDirty begins a dev run of one remote-only job with the snapshot offload hook installed.
// Status is not disabled by option: a snapshot run is unbound, so it is never a status of record.
func (f *fixture) startDirty(ctx context.Context, job model.Job) (<-chan run.Result, *bytes.Buffer, *[]string) {
	f.t.Helper()
	pl, err := plan.Build(plan.Input{Config: model.PipelineConfig{Version: 1, Jobs: map[string]model.Job{"unit": job}},
		World: plan.World{Platform: runtime.GOOS + "/" + runtime.GOARCH, CPU: 8, MemMB: 16384}})
	if err != nil {
		f.t.Fatal(err)
	}
	var excluded []string
	hook := run.SnapshotOffload{
		Build: func(ctx context.Context, root string) (*run.OffloadSnapshot, error) {
			s, err := offload.Take(ctx, realGit, root, f.t.TempDir(), "sim1")
			if err != nil {
				return nil, err
			}
			return &run.OffloadSnapshot{SHA: s.SHA, Excluded: s.Excluded, Release: func(ctx context.Context) error { return offload.Release(ctx, realGit, root, s) }}, nil
		},
		Excluded: func(names []string) { excluded = append(excluded, names...) },
	}
	out := &bytes.Buffer{}
	c := &run.Coordinator{Opts: run.Options{Root: f.repo, Home: f.home}, Plan: pl, ConfigDigest: "cfg", Out: out, Capture: realGit, Reporter: noReport{},
		Hooks: run.Hooks{Auto: run.AutoHooks{Offload: hook}},
		Capacity: func() ciexec.Capacity {
			return ciexec.Capacity{CPUs: 8, MemoryMB: 16384, FreeMemoryMB: 12000, BatteryPercent: -1, PluggedIn: true, Interactive: true}
		}, Poll: 10 * time.Millisecond}
	ch := make(chan run.Result, 1)
	go func() {
		res, err := c.Run(ctx)
		if err != nil {
			f.t.Errorf("Run: %v\n%s", err, out)
		}
		ch <- res
	}()
	return ch, out, &excluded
}

func passBody(_ context.Context, j session.JobRun) (session.JobResult, error) {
	j.Output([]byte("tree checked\n"))
	return session.JobResult{Outcome: session.Outcome{State: "passed"}, Evidence: evidence(j.Lease, "pass", "", nil)}, nil
}

// requireSnapshotTree checks what the node saw against the user's tree.
func requireSnapshotTree(t *testing.T, f *fixture, s seen) {
	t.Helper()
	if s.err != nil {
		t.Fatalf("the node could not stage the snapshot: %v", s.err)
	}
	if s.head == "" || s.head == f.sha {
		t.Fatalf("node HEAD %q: want the snapshot commit, not the committed HEAD %s", s.head, f.sha)
	}
	if parent := gitIn(t, f.repo, "rev-parse", "--verify", "-q", s.head+"^"); parent != f.sha {
		t.Fatalf("snapshot parent %s, want HEAD %s", parent, f.sha)
	}
	if s.files["f.txt"] != "edited on the laptop\n" {
		t.Errorf("the node's f.txt = %q: the tracked change did not arrive", s.files["f.txt"])
	}
	if s.files["new.txt"] != "untracked and not ignored\n" {
		t.Errorf("the node's new.txt = %q: the untracked file did not arrive", s.files["new.txt"])
	}
	for _, name := range []string{".env", ".env.local"} {
		if c, ok := s.files[name]; ok {
			t.Errorf("%s reached the node (%d bytes)", name, len(c))
		}
	}
}

func requirePrivateRefGone(t *testing.T, repo string) {
	t.Helper()
	if refs := gitIn(t, repo, "for-each-ref", "refs/nself-ci/"); refs != "" {
		t.Errorf("private refs left behind:\n%s", refs)
	}
}

func TestOffloadDirtyTreeToNode(t *testing.T) {
	f, nodes := dirtyFixture(t, passBody)
	before := repoState(t, f.repo)
	ch, out, excluded := f.startDirty(context.Background(), remoteJob(1))
	staged := nodes.wait(t)
	res := wait(t, ch, "the run")
	if res.Exit != 0 || res.Summary.Result != "pass" {
		t.Fatalf("exit %d result %s\n%s", res.Exit, res.Summary.Result, out)
	}
	requireSnapshotTree(t, f, staged)
	if len(*excluded) != 1 || (*excluded)[0] != ".env.local" {
		t.Errorf("excluded = %v, want only the untracked .env.local (the ignored .env is never listed)", *excluded)
	}
	for _, s := range []string{secretEnv, secretLocal} {
		if strings.Contains(out.String(), s) || strings.Contains(res.Summary.Status, s) {
			t.Errorf("secret content %q reached the report", s)
		}
	}
	// a snapshot run is a dev run: unbound evidence, no status of record
	if d := res.Summary.Doc; d.WorktreeState != "dirty" || d.Binding.State != "unbound" {
		t.Errorf("evidence worktree %q binding %q, want dirty and unbound", d.WorktreeState, d.Binding.State)
	}
	if after := repoState(t, f.repo); after != before {
		t.Errorf("the user's repository changed:\n--- before\n%s\n--- after\n%s", before, after)
	}
	requirePrivateRefGone(t, f.repo)
}

func TestOffloadCancelCleansRef(t *testing.T) {
	staged := make(chan struct{})
	body := func(ctx context.Context, j session.JobRun) (session.JobResult, error) {
		close(staged)
		j.Output([]byte("working\n"))
		<-ctx.Done() // the agent kills the job when the lease is cancelled
		return session.JobResult{Outcome: session.Outcome{State: "cancelled"}, Evidence: evidence(j.Lease, "cancelled", "", nil)}, nil
	}
	f, nodes := dirtyFixture(t, body)
	ssh.Default.Heartbeat = 50 * time.Second // the agent beats rarely, so only the carrier cancel can stop the job
	t.Cleanup(run.SetRemoteCancelWait(10 * time.Minute))
	before := repoState(t, f.repo)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, out, _ := f.startDirty(ctx, remoteJob(1))
	requireSnapshotTree(t, f, nodes.wait(t))
	select {
	case <-staged:
	case <-time.After(30 * time.Second):
		t.Fatal("the job never started on a node")
	}
	if refs := gitIn(t, f.repo, "for-each-ref", "refs/nself-ci/offload/"); refs == "" {
		t.Error("the snapshot ref does not exist while the job runs")
	}
	cancel() // Ctrl-C
	var res run.Result
	select {
	case res = <-ch:
	case <-time.After(30 * time.Second):
		t.Fatalf("the cancelled run did not end in 30 s\n%s", out)
	}
	if res.Summary.Result != "cancelled" {
		t.Fatalf("result %q, want cancelled\n%s", res.Summary.Result, out)
	}
	requirePrivateRefGone(t, f.repo)
	if after := repoState(t, f.repo); after != before {
		t.Errorf("the user's repository changed after a cancel:\n--- before\n%s\n--- after\n%s", before, after)
	}
}

func TestOffloadFailingJobReturnsTail(t *testing.T) {
	body := func(_ context.Context, j session.JobRun) (session.JobResult, error) {
		three := 3
		j.Output([]byte("FAIL: TestSomething\nboom on the node\n"))
		return session.JobResult{Outcome: session.Outcome{State: "failed", ExitCode: 3}, Evidence: evidence(j.Lease, "fail", "code", &three)}, nil
	}
	f, nodes := dirtyFixture(t, body)
	before := repoState(t, f.repo)
	ch, out, _ := f.startDirty(context.Background(), remoteJob(0))
	requireSnapshotTree(t, f, nodes.wait(t))
	res := wait(t, ch, "the run")
	jo := res.Summary.Jobs[0]
	if res.Exit == 0 || jo.State != "failed" || jo.ExitCode != 3 {
		t.Fatalf("a failing dev job: exit %d outcome %+v\n%s", res.Exit, jo, out)
	}
	if !strings.Contains(jo.Tail, "boom on the node") {
		t.Errorf("the log tail did not come back: %q", jo.Tail)
	}
	if res.Summary.Doc.Binding.State != "unbound" {
		t.Errorf("binding %q: a failing snapshot run is a dev run", res.Summary.Doc.Binding.State)
	}
	requirePrivateRefGone(t, f.repo)
	if after := repoState(t, f.repo); after != before {
		t.Errorf("the user's repository changed after a failed job:\n--- before\n%s\n--- after\n%s", before, after)
	}
}
