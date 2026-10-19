package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/sdk/go/v2/output"

	"github.com/nself-org/plugins/free/ci/internal/evidence"
	"github.com/nself-org/plugins/free/ci/internal/model"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
)

const ciYAML = "version: 1\njobs:\n  unit:\n    kind: test\n    run: [\"sh\", \"-c\", \"echo job-output-must-not-travel\"]\n"

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.email=t@example.test", "-c", "user.name=t", "-c", "commit.gpgsign=false", "-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func put(t *testing.T, dir, name, body string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// evRepo is a committed repository with a one-job pipeline, V15 on, a private
// run store and a private TMPDIR (leftover temporary worktrees show up there).
type evRepo struct {
	t              *testing.T
	dir, home, tmp string
}

func newEvRepo(t *testing.T) *evRepo {
	t.Helper()
	r := &evRepo{t: t, dir: t.TempDir(), home: t.TempDir(), tmp: t.TempDir()}
	git(t, r.dir, "init", "-q", "-b", "main")
	put(t, r.dir, "go.mod", "module x\n\ngo 1.22\n")
	put(t, r.dir, "go.sum", "h1:one\n")
	put(t, r.dir, ".nself/ci.yaml", ciYAML)
	git(t, r.dir, "add", "-A")
	git(t, r.dir, "commit", "-q", "-m", "init")
	t.Setenv("NSELF_V15", "1")
	t.Setenv("NSELF_CI_REPO", r.dir)
	t.Setenv("NSELF_CI_HOME", r.home)
	t.Setenv("TMPDIR", r.tmp)
	t.Setenv("NSELF_CI_BASE_SHA", "")
	return r
}

func (r *evRepo) head() string { return git(r.t, r.dir, "rev-parse", "HEAD") }

func (r *evRepo) commit(name, body string) string {
	put(r.t, r.dir, name, body)
	git(r.t, r.dir, "add", "-A")
	git(r.t, r.dir, "commit", "-q", "-m", "change "+name)
	return r.head()
}

// ci runs `nself ci <args>` in process and returns stdout, stderr and the exit code.
func (r *evRepo) ci(args ...string) (string, string, int) {
	r.t.Helper()
	capture := func(f **os.File) (restore func() string) {
		rd, wr, err := os.Pipe()
		if err != nil {
			r.t.Fatal(err)
		}
		old := *f
		*f = wr
		done := make(chan string)
		go func() { b, _ := io.ReadAll(rd); done <- string(b) }()
		return func() string { _ = wr.Close(); *f = old; return <-done }
	}
	endOut, endErr := capture(&os.Stdout), capture(&os.Stderr)
	code := runV15(args)
	stdout := endOut()
	return stdout, endErr(), code
}

// ciJSON runs a command that prints one envelope and returns its data.
func (r *evRepo) ciJSON(args ...string) (map[string]any, int) {
	r.t.Helper()
	out, errOut, code := r.ci(args...)
	if err := output.CheckEnvelope([]byte(out)); err != nil {
		r.t.Fatalf("%v: not a v1 envelope: %v\nstdout: %s\nstderr: %s", args, err, out, errOut)
	}
	var env struct {
		Data  map[string]any `json:"data"`
		Error map[string]any `json:"error"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		r.t.Fatal(err)
	}
	if env.Data == nil {
		env.Data = map[string]any{"error": env.Error}
	}
	return env.Data, code
}

func (r *evRepo) runCount() int {
	r.t.Helper()
	st, err := cirun.OpenStore(filepath.Join(r.home, "state.db"))
	if err != nil {
		r.t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	page, err := st.History(context.Background(), 0, 100)
	if err != nil {
		r.t.Fatal(err)
	}
	return len(page.Pipelines)
}

func (r *evRepo) leftovers() []string {
	left, _ := filepath.Glob(filepath.Join(r.tmp, "nself-ci-verify-*"))
	return left
}

func schemaOK(t *testing.T, doc any) {
	t.Helper()
	schemas, err := model.Schemas()
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(doc)
	if err = model.ValidateJSON(schemas["ci-evidence.v1.schema.json"], b); err != nil {
		t.Fatalf("not ci.evidence/v1: %v\n%s", err, b)
	}
}

func str(m map[string]any, keys ...string) string {
	var cur any = m
	for _, k := range keys {
		next, _ := cur.(map[string]any)
		cur = next[k]
	}
	s, _ := cur.(string)
	return s
}

func TestBareJSONIsOneEvidenceEnvelope(t *testing.T) {
	r := newEvRepo(t)
	out, _, code := r.ci("--no-status", "--json")
	if code != 0 || strings.Count(strings.TrimSpace(out), "\n") < 1 || !strings.HasPrefix(out, "{") {
		t.Fatalf("exit %d\n%s", code, out)
	}
	if err := output.CheckEnvelope([]byte(out)); err != nil {
		t.Fatalf("not one envelope: %v\n%s", err, out)
	}
	var env struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("more than one JSON value: %v", err)
	}
	schemaOK(t, env.Data)
	if str(env.Data, "revision") != r.head() || str(env.Data, "binding", "state") != "bound" || str(env.Data, "result") != "pass" {
		t.Fatalf("data: %v", env.Data)
	}
	if human, _, c := r.ci("--no-status"); c != 0 || !strings.Contains(human, "nself-ci: pass") || strings.Contains(human, "schema_version") {
		t.Fatalf("human output changed: %d %q", c, human)
	}
}

func TestDirtyTree(t *testing.T) {
	r := newEvRepo(t)
	put(t, r.dir, "untracked.txt", "not committed\n")
	put(t, r.dir, "go.sum", "h1:edited\n")
	data, code := r.ciJSON("--no-status", "--json")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	schemaOK(t, data)
	if str(data, "binding", "state") != "unbound" || str(data, "binding", "reason") != "dirty" || str(data, "worktree_state") != "dirty" {
		t.Fatalf("a dirty tree must be unbound: %v", data)
	}
	if data["tracked_changes_digest"] == nil || str(data, "freshness", "state") == "current" {
		t.Fatalf("tracked digest / freshness: %v %v", data["tracked_changes_digest"], data["freshness"])
	}
	dirtyRun := str(data, "run_id")
	rep, code := r.ciJSON("evidence", "verify", r.head(), "--json")
	if code != 0 || str(rep, "evidence", "run_id") == dirtyRun || str(rep, "binding", "state") != "bound" || str(rep, "freshness", "state") != "current" {
		t.Fatalf("verify accepted the dirty evidence or failed: exit %d %v", code, rep)
	}
	if str(rep, "evidence", "input_digests", "lockfiles") != "" {
		t.Fatal("lockfiles is a list")
	}
	locks := rep["evidence"].(map[string]any)["input_digests"].(map[string]any)["lockfiles"].([]any)
	if len(locks) != 1 || locks[0].(map[string]any)["sha256"] == nil {
		t.Fatalf("lockfiles: %v", locks)
	}
	if r.runCount() != 2 || len(r.leftovers()) != 0 {
		t.Fatalf("runs %d, leftovers %v", r.runCount(), r.leftovers())
	}
	if got := git(t, r.dir, "status", "--porcelain"); !strings.Contains(got, "untracked.txt") || !strings.Contains(got, "go.sum") {
		t.Fatalf("verify touched the working tree: %q", got)
	}
}

func TestVerifyWorktree(t *testing.T) {
	r := newEvRepo(t)
	first, code := r.ciJSON("--no-status", "--json")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	prev := str(first, "run_id")
	rep, code := r.ciJSON("evidence", "verify", r.head(), "--policy", "required", "--json")
	if code != 0 || str(rep, "evidence", "run_id") != prev || r.runCount() != 1 {
		t.Fatalf("held evidence must be returned without running: exit %d runs %d %v", code, r.runCount(), str(rep, "evidence", "run_id"))
	}
	if str(rep, "class") != "default" || rep["quarantined"] != false || str(rep, "result") != "pass" {
		t.Fatalf("report: %v", rep)
	}
	sha2 := r.commit("go.sum", "h1:two\n")
	shown, code := r.ciJSON("evidence", prev, "--json")
	schemaOK(t, shown)
	if code != 0 || str(shown, "freshness", "state") != "stale" || !strings.Contains(str(shown, "freshness", "reason"), "go.sum") {
		t.Fatalf("after a lockfile change the old run must read stale: %v", shown["freshness"])
	}
	put(t, r.dir, "a.txt", "uncommitted\n")
	rep, code = r.ciJSON("evidence", "verify", sha2, "--json")
	if code != 0 || r.runCount() != 2 || str(rep, "revision") != sha2 || str(rep, "evidence", "revision") != sha2 {
		t.Fatalf("a lockfile change must run a new pipeline: exit %d runs %d %v", code, r.runCount(), rep)
	}
	if len(r.leftovers()) != 0 || len(strings.Split(git(t, r.dir, "worktree", "list"), "\n")) != 1 {
		t.Fatalf("the temporary worktree survived: %v", r.leftovers())
	}
	st, _ := cirun.OpenStore(filepath.Join(r.home, "state.db"))
	defer func() { _ = st.Close() }()
	row, found, err := st.PipelineByID(context.Background(), str(rep, "evidence", "run_id"))
	if err != nil || !found || row.Project == r.dir || strings.HasPrefix(row.Project, r.dir) {
		t.Fatalf("the run was not in a temporary worktree: %q %v", row.Project, err)
	}
	for _, bad := range []string{"HEAD", "main", sha2[:10]} {
		if _, _, c := r.ci("evidence", "verify", bad); c != 1 {
			t.Errorf("verify %s: exit %d, want 1", bad, c)
		}
	}
	if out, _, c := r.ci("evidence", "verify", "--json"); c != 1 || out != "" {
		t.Errorf("no revision: exit %d %q", c, out)
	}
}

func TestVerifyFile(t *testing.T) {
	r := newEvRepo(t)
	first, _ := r.ciJSON("--no-status", "--json")
	write := func(name string, mut func(map[string]any)) string {
		doc := map[string]any{}
		b, _ := json.Marshal(first)
		_ = json.Unmarshal(b, &doc)
		mut(doc)
		p := filepath.Join(t.TempDir(), name)
		b, _ = json.Marshal(doc)
		if err := os.WriteFile(p, b, 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	file := write("ok.json", func(map[string]any) {})
	rep, code := r.ciJSON("evidence", "verify", "--file", file, "--json")
	if code != 0 {
		t.Fatalf("exit %d %v", code, rep)
	}
	for _, k := range []string{"result", "class", "binding", "freshness", "quarantined", "evidence", "revision"} {
		if _, ok := rep[k]; !ok {
			t.Errorf("report lacks %s", k)
		}
	}
	cases := []struct {
		name string
		mut  func(map[string]any)
		want int
	}{
		{"revision edited", func(d map[string]any) { d["revision"] = strings.Repeat("a", 40) }, 11},
		{"digest edited", func(d map[string]any) {
			d["input_digests"].(map[string]any)["lockfiles"].([]any)[0].(map[string]any)["sha256"] = strings.Repeat("0", 64)
		}, 11},
		{"schema invalid", func(d map[string]any) { delete(d, "checks") }, 11},
		{"unbound", func(d map[string]any) {
			d["run_id"] = "r-from-elsewhere" // a run this machine did not record: no cross-check
			d["worktree_state"] = "dirty"
			d["binding"] = map[string]any{"state": "unbound", "reason": "dirty"}
			d["freshness"] = map[string]any{"state": "unknown", "reason": "dirty"}
		}, 12},
		{"result fails", func(d map[string]any) {
			d["run_id"] = "r-from-elsewhere"
			d["result"] = "fail"
			d["checks"].([]any)[0].(map[string]any)["result"] = "fail"
			d["counts"] = map[string]any{"pass": 0, "fail": 1, "skip": 0, "error": 0, "total": 1}
		}, 1},
	}
	for _, tc := range cases {
		if _, code = r.ciJSON("evidence", "verify", "--file", write(tc.name+".json", tc.mut), "--json"); code != tc.want {
			t.Errorf("%s: exit %d, want %d", tc.name, code, tc.want)
		}
	}
	if _, _, code = r.ci("evidence", "verify", "--file", filepath.Join(t.TempDir(), "absent.json")); code != 11 {
		t.Errorf("unreadable file: exit %d", code)
	}
	// stale: the same document, the lockfile changed in a later commit, checked at that revision
	sha1 := r.head()
	sha2 := r.commit("go.sum", "h1:two\n")
	if _, code = r.ciJSON("evidence", "verify", "--file", file, "--at-revision", sha2, "--json"); code != 11 {
		t.Errorf("document of another revision at --at-revision: exit %d", code)
	}
	moved := write("moved.json", func(d map[string]any) { d["revision"] = sha2; d["run_id"] = "r-not-in-this-store" })
	if _, code = r.ciJSON("evidence", "verify", "--file", moved, "--at-revision", sha2, "--json"); code != 10 {
		t.Errorf("stale document: exit %d, want 10", code)
	}
	if rep, code = r.ciJSON("evidence", "verify", "--file", file, "--at-revision", sha1, "--json"); code != 0 || str(rep, "freshness", "state") != "current" {
		t.Errorf("fresh at its own revision after the tree moved on: exit %d %v", code, rep["freshness"])
	}
	if r.runCount() != 1 {
		t.Errorf("a file check ran the pipeline: %d runs", r.runCount())
	}
}

func TestStream(t *testing.T) {
	r := newEvRepo(t)
	out, _, code := r.ci("--no-status", "--json", "--stream")
	if code != 0 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	all := strings.Split(strings.TrimRight(out, "\n"), "\n")
	var prev float64
	var types []string
	logs := 0
	for i, raw := range all {
		if strings.Contains(raw, "\n") || raw == "" {
			t.Fatalf("line %d is not compact", i)
		}
		if err := output.CheckEnvelope([]byte(raw)); err != nil {
			t.Fatalf("line %d: %v\n%s", i, err, raw)
		}
		var l struct {
			Data map[string]any `json:"data"`
		}
		_ = json.Unmarshal([]byte(raw), &l)
		seq, _ := l.Data["seq"].(float64)
		if seq != prev+1 {
			t.Fatalf("line %d: seq %v after %v", i, seq, prev)
		}
		prev = seq
		types = append(types, str(l.Data, "type"))
		if i == 0 && (str(l.Data, "type") != "queued" || str(l.Data, "run_id") == "") {
			t.Fatalf("first line: %v", l.Data)
		}
		if i == len(all)-1 {
			if str(l.Data, "type") != "result" {
				t.Fatalf("last line type %q", str(l.Data, "type"))
			}
			schemaOK(t, l.Data["evidence"])
		}
		if str(l.Data, "type") == "log" {
			logs++
			payload, _ := l.Data["data"].(map[string]any)
			if len(payload) == 0 {
				t.Fatalf("a log record has no payload: %s", raw)
			}
			for k := range payload {
				if k != "bytes" && k != "ref" && k != "reason" {
					t.Fatalf("a log record carries %q (only size and reason may travel): %s", k, raw)
				}
			}
		}
	}
	if logs == 0 || strings.Contains(out, "job-output-must-not-travel") {
		t.Errorf("%d log records, or job output travelled in the stream", logs)
	}
	for _, want := range []string{"queued", "planned", "started", "check_result", "finished", "result"} {
		found := false
		for _, ty := range types {
			found = found || ty == want
		}
		if !found {
			t.Errorf("no %s record in %v", want, types)
		}
	}
	// verify --json --stream: the run's events, then one result line carrying the report
	r.commit("go.sum", "h1:two\n")
	out, _, code = r.ci("evidence", "verify", r.head(), "--json", "--stream")
	last := strings.Split(strings.TrimRight(out, "\n"), "\n")
	var end struct {
		Command string         `json:"command"`
		Data    map[string]any `json:"data"`
	}
	if err := json.Unmarshal([]byte(last[len(last)-1]), &end); err != nil || code != 0 || end.Command != "ci evidence verify" || str(end.Data, "type") != "result" {
		t.Fatalf("verify stream: exit %d %v\n%s", code, err, out)
	}
	prev = 0
	for i, raw := range last {
		var l struct {
			Data map[string]any `json:"data"`
		}
		if err := output.CheckEnvelope([]byte(raw)); err != nil || json.Unmarshal([]byte(raw), &l) != nil {
			t.Fatalf("verify stream line %d: %v\n%s", i, err, raw)
		}
		if seq, _ := l.Data["seq"].(float64); i < len(last)-1 && seq != prev+1 {
			t.Fatalf("verify stream line %d: seq %v after %v", i, seq, prev)
		} else if i < len(last)-1 {
			prev = seq
		}
		if i == 0 && str(l.Data, "type") != "queued" {
			t.Fatalf("verify stream starts with %q, want queued", str(l.Data, "type"))
		}
	}
	rep := end.Data["evidence"].(map[string]any)
	if str(rep, "revision") != r.head() || rep["quarantined"] != false || len(last) < 4 {
		t.Fatalf("result line: %v (lines %d)", rep, len(last))
	}
	schemaOK(t, rep["evidence"])
	if _, _, code = r.ci("evidence", "verify", r.head(), "--stream"); code != 1 {
		t.Errorf("--stream without --json: exit %d", code)
	}
}

type fakeReporter struct{ overdue bool }

func (f fakeReporter) Report(_ context.Context, name string) (evidence.PipelineReport, error) {
	return evidence.PipelineReport{Pipeline: name, Overdue: f.overdue}, nil
}

func TestPipelineReporter(t *testing.T) {
	r := newEvRepo(t)
	if _, _, code := r.ci("evidence", "--pipeline", "nightly"); code != 1 {
		t.Fatalf("no reporter registered: exit %d", code)
	}
	evidence.RegisterPipelineReporter("nightly", fakeReporter{overdue: true})
	evidence.RegisterPipelineReporter("weekly", fakeReporter{})
	if data, code := r.ciJSON("evidence", "--pipeline", "nightly", "--json"); code != evidence.ExitStale || data["overdue"] != true {
		t.Fatalf("overdue: exit %d %v", code, data)
	}
	if data, code := r.ciJSON("evidence", "--pipeline", "weekly", "--json"); code != 0 || data["overdue"] != false {
		t.Fatalf("none overdue: exit %d %v", code, data)
	}
}

type remoteRun struct{}

func (remoteRun) Resolve(_ context.Context, ref string) (cirun.RunView, bool, error) {
	if ref == "remote-run" {
		return cirun.RunView{ID: "remote-run", Status: "passed"}, true, nil
	}
	return cirun.RunView{}, false, nil
}

func TestEvidenceAsksRemoteResolversFirst(t *testing.T) {
	r := newEvRepo(t)
	saved := cirun.Registered.Remote
	cirun.RegisterRemote(remoteRun{})
	defer func() { cirun.Registered.Remote = saved }()
	data, code := r.ciJSON("evidence", "remote-run", "--json")
	if code != 0 || str(data, "id") != "remote-run" {
		t.Fatalf("remote run: exit %d %v", code, data)
	}
	if r.runCount() != 0 {
		t.Fatal("the local store was consulted for a remote run")
	}
	if _, code = r.ciJSON("evidence", "no-such-run", "--json"); code != 1 {
		t.Fatalf("unknown run: exit %d", code)
	}
}

// A stream consumer that goes away must not kill the process: the closed pipe
// returns EPIPE, the emitter stops and the temporary worktree is still removed.
func TestStreamSurvivesAClosedPipe(t *testing.T) {
	if os.Getenv("EV_PIPE_HELPER") == "1" {
		os.Exit(runV15(strings.Fields(os.Getenv("EV_PIPE_ARGS"))))
	}
	r := newEvRepo(t)
	if _, _, code := r.ci("--no-status", "--json"); code != 0 {
		t.Fatalf("setup run: exit %d", code)
	}
	r.commit("go.sum", "h1:two\n") // verify must run in a temporary worktree
	rd, wr, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	_ = rd.Close()
	cmd := exec.Command(os.Args[0], "-test.run=^TestStreamSurvivesAClosedPipe$")
	cmd.Env = append(os.Environ(), "EV_PIPE_HELPER=1", "EV_PIPE_ARGS=evidence verify "+r.head()+" --json --stream")
	cmd.Stdout = wr
	runErr := cmd.Run()
	_ = wr.Close()
	var ee *exec.ExitError
	if errors.As(runErr, &ee) && !ee.Exited() {
		t.Fatalf("the process was killed by a signal: %v", runErr)
	}
	if left := r.leftovers(); len(left) != 0 {
		t.Fatalf("temporary worktrees left behind: %v", left)
	}
}

// In a clone with an origin the normal selector plans only the affected jobs;
// verify must still run, and judge, the whole pipeline.
func TestVerifyRunsTheFullPipelineInACloneWithAnOrigin(t *testing.T) {
	r := newEvRepo(t)
	bare := filepath.Join(t.TempDir(), "origin.git")
	git(t, r.dir, "clone", "-q", "--bare", r.dir, bare)
	git(t, r.dir, "remote", "add", "origin", bare)
	git(t, r.dir, "fetch", "-q", "origin")
	git(t, r.dir, "remote", "set-head", "origin", "main")
	git(t, r.dir, "checkout", "-q", "-b", "feat")
	head := r.commit("note.txt", "a change that selects only part of the pipeline\n")
	data, code := r.ciJSON("evidence", "verify", head, "--json")
	if code != 0 {
		t.Fatalf("verify of a green feature commit: exit %d %v", code, data)
	}
	if mode := str(data, "evidence", "selection", "mode"); mode != "full" {
		t.Fatalf("verify ran a %q selection, want full", mode)
	}
}

// The terminal error line of a stream is the last line: everything queued
// before it reaches the consumer first.
func TestStreamErrorLineComesLast(t *testing.T) {
	rd, wr, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = wr
	got := make(chan string)
	go func() { b, _ := io.ReadAll(rd); got <- string(b) }()
	sess := &streamSession{w: wr, command: "ci evidence verify"}
	activeMu.Lock()
	activeSession = sess
	activeMu.Unlock()
	em := sess.emitter("run-1")
	for i := 0; i < 200; i++ {
		em.Emit("started", "job", "", nil)
	}
	code := evidenceFail("ci evidence verify", evidenceFlags{json: true, stream: true}, errors.New("the run did not finish"))
	activeMu.Lock()
	activeSession = nil
	activeMu.Unlock()
	_ = wr.Close()
	os.Stdout = old
	lines := strings.Split(strings.TrimRight(<-got, "\n"), "\n")
	if code == 0 || len(lines) != 201 {
		t.Fatalf("exit %d, %d lines, want 201", code, len(lines))
	}
	if !strings.Contains(lines[200], `"error"`) || strings.Contains(lines[199], `"error"`) {
		t.Fatalf("the error line is not the last one:\n%s\n%s", lines[199], lines[200])
	}
}
