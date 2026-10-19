package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/agent/session"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

const rev40 = "0123456789abcdef0123456789abcdef01234567"

// TestMain lets the test binary stand in for the agent binary in a child
// process (cred-wrapper replaces its process, so it cannot run in-process).
func TestMain(m *testing.M) {
	if os.Getenv("NSELF_AGENT_TEST_CHILD") == "1" {
		os.Exit(run(context.Background(), os.Args[1:], IO{Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr}))
	}
	os.Exit(m.Run())
}

func skipUnixOnly(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell jobs")
	}
}

func useState(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "agent-state")
	t.Setenv("NSELF_CI_AGENT_HOME", dir)
	t.Setenv("NSELF_CI_AGENT_WORK", filepath.Join(t.TempDir(), "work"))
	return dir
}

func runCmd(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errb bytes.Buffer
	code = run(context.Background(), args, IO{Stdin: strings.NewReader(""), Stdout: &out, Stderr: &errb})
	return code, out.String(), errb.String()
}

func writeSpec(t *testing.T, runArgs ...string) string {
	t.Helper()
	spec := execSpec{AttemptID: "attempt-1", CoordinatorID: "local", JobID: "fixture", Revision: rev40, Job: model.Job{Kind: "static", Run: runArgs}}
	b, _ := json.Marshal(spec)
	p := filepath.Join(t.TempDir(), "job.json")
	if err := os.WriteFile(p, b, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func validEvidence(t *testing.T, doc []byte) map[string]any {
	t.Helper()
	schemas, err := model.Schemas()
	if err != nil {
		t.Fatal(err)
	}
	if err := model.ValidateJSON(schemas["ci-evidence.v1.schema.json"], doc); err != nil {
		t.Fatalf("not ci.evidence/v1: %v\n%s", err, doc)
	}
	var m map[string]any
	_ = json.Unmarshal(doc, &m)
	return m
}

// TestExecWritesValidEvidence proves the acceptance line: `exec --spec
// --evidence` runs the fixture job and writes evidence that validates against
// schemas/ci-evidence.v1.schema.json; a failing job exits 1 with a failing,
// still valid, document.
func TestExecWritesValidEvidence(t *testing.T) {
	skipUnixOnly(t)
	useState(t)
	ev := filepath.Join(t.TempDir(), "e.json")
	code, out, errs := runCmd(t, "exec", "--spec", writeSpec(t, "sh", "-c", "echo fixture-ran"), "--evidence", ev)
	if code != 0 || !strings.Contains(out, "fixture-ran") {
		t.Fatalf("code %d out %q err %q", code, out, errs)
	}
	doc, err := os.ReadFile(ev)
	if err != nil {
		t.Fatal(err)
	}
	m := validEvidence(t, doc)
	if m["result"] != "pass" || m["revision"] != rev40 {
		t.Fatalf("evidence %v", m)
	}
	if info, _ := os.Stat(ev); runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("evidence mode %v", info.Mode())
	}
	ev2 := filepath.Join(t.TempDir(), "f.json")
	code, _, _ = runCmd(t, "exec", "--spec", writeSpec(t, "sh", "-c", "echo broke; exit 3"), "--evidence", ev2)
	doc, _ = os.ReadFile(ev2)
	if m := validEvidence(t, doc); code != 1 || m["result"] != "fail" {
		t.Fatalf("code %d evidence %v", code, m["result"])
	}
}

func TestExecUsage(t *testing.T) {
	useState(t)
	for _, args := range [][]string{{"exec"}, {"exec", "--spec", "x"}, {"exec", "--spec", "/nonexistent", "--evidence", "e"}, {"exec", "--bogus"}} {
		if code, _, _ := runCmd(t, args...); code != 2 {
			t.Errorf("%v exit %d, want 2", args, code)
		}
	}
	bad := filepath.Join(t.TempDir(), "bad.json")
	_ = os.WriteFile(bad, []byte(`{"unknown_field":1}`), 0o600)
	if code, _, _ := runCmd(t, "exec", "--spec", bad, "--evidence", "e"); code != 2 {
		t.Errorf("unknown spec field exit %d, want 2", code)
	}
	noRev := filepath.Join(t.TempDir(), "norev.json")
	_ = os.WriteFile(noRev, []byte(`{"attempt_id":"a","coordinator_id":"c","job":{"kind":"static","run":["true"]}}`), 0o600)
	if code, _, _ := runCmd(t, "exec", "--spec", noRev, "--evidence", filepath.Join(t.TempDir(), "e")); code != 1 {
		t.Errorf("missing revision exit %d, want 1", code)
	}
}

func TestVersionAndUnknownCommand(t *testing.T) {
	code, out, _ := runCmd(t, "version")
	if code != 0 || !strings.HasPrefix(out, "nself-ci-agent "+Version+" ") {
		t.Fatalf("%d %q", code, out)
	}
	if code, _, _ := runCmd(t, "frobnicate"); code != 2 {
		t.Fatalf("unknown exit %d", code)
	}
	if code, _, errs := runCmd(t); code != 2 || !strings.Contains(errs, "serve") {
		t.Fatalf("no-arg exit %d %q", code, errs)
	}
}

// resetRegistry restores the registry after a test registers extras.
func resetRegistry(t *testing.T) {
	t.Helper()
	regMu.Lock()
	savedCmds, savedPre := map[string]command{}, append([]preStart(nil), prestart...)
	for k, v := range commands {
		savedCmds[k] = v
	}
	regMu.Unlock()
	t.Cleanup(func() { regMu.Lock(); commands, prestart = savedCmds, savedPre; regMu.Unlock() })
}

type tripReader struct {
	t    *testing.T
	read bool
}

func (r *tripReader) Read([]byte) (int, error) { r.read = true; return 0, io.EOF }

// TestRegistryRunsSubcommandAndFailingPreStartStopsServe proves the acceptance
// line: a test subcommand and a failing PreStart hook registered from another
// file run through the registry, and the failing hook stops serve before any
// read from stdin (and before any write to stdout).
func TestRegistryRunsSubcommandAndFailingPreStartStopsServe(t *testing.T) {
	useState(t)
	resetRegistry(t)
	must(RegisterSubcommand("pingtest", "test only", func(_ context.Context, args []string, sio IO) int {
		_, _ = io.WriteString(sio.Stdout, "pong "+strings.Join(args, ","))
		return 0
	}))
	if code, out, _ := runCmd(t, "pingtest", "a", "b"); code != 0 || out != "pong a,b" {
		t.Fatalf("%d %q", code, out)
	}
	if err := RegisterSubcommand("pingtest", "dup", func(context.Context, []string, IO) int { return 0 }); err == nil {
		t.Fatal("accepted a duplicate subcommand")
	}
	var seen []string
	var mu sync.Mutex
	must(RegisterPreStart("ok-hook", func(_ context.Context, c string) error { mu.Lock(); seen = append(seen, c); mu.Unlock(); return nil }))
	must(RegisterPreStart("refuse", func(context.Context, string) error { return io.ErrUnexpectedEOF }))
	in, out := &tripReader{t: t}, &bytes.Buffer{}
	code := run(context.Background(), []string{"serve", "--stdio"}, IO{Stdin: in, Stdout: out, Stderr: io.Discard})
	if code != 1 || in.read || out.Len() != 0 {
		t.Fatalf("code %d read %v wrote %q", code, in.read, out.String())
	}
	if len(seen) != 1 || seen[0] != "serve" {
		t.Fatalf("hooks saw %v", seen)
	}
}

type stdioPeer struct {
	t     *testing.T
	toIn  *io.PipeWriter
	enc   *protocol.Encoder
	dec   *protocol.Decoder
	done  chan int
	ended chan struct{}
	seq   uint64
}

// startServe runs `serve --stdio` in-process against pipes and returns the
// scripted coordinator's end, with the preamble already consumed.
func startServe(t *testing.T, extra ...string) *stdioPeer {
	t.Helper()
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	p := &stdioPeer{t: t, toIn: inW, enc: protocol.NewEncoder(inW), done: make(chan int, 1), ended: make(chan struct{})}
	go func() {
		code := run(context.Background(), append([]string{"serve", "--stdio", "--slots", "2"}, extra...), IO{Stdin: inR, Stdout: outW, Stderr: io.Discard})
		_ = outW.Close()
		p.done <- code
		close(p.ended)
	}()
	rd, err := protocol.SkipPreamble(outR)
	if err != nil {
		t.Fatal(err)
	}
	p.dec = protocol.NewDecoder(rd, protocol.CarrierPolicy{})
	t.Cleanup(func() { // stop serve before the state dir is removed
		_ = inW.Close()
		select {
		case <-p.ended:
		case <-time.After(15 * time.Second):
			t.Error("serve did not stop")
		}
	})
	return p
}

func (p *stdioPeer) send(kind string, body any, epoch uint64, lease string) {
	p.t.Helper()
	p.seq++
	m := protocol.Message{V: 1, Type: kind, Seq: p.seq, Body: body, LeaseID: lease}
	if epoch != 0 {
		m.Epoch = &epoch
	}
	if err := p.enc.Encode(m); err != nil {
		p.t.Fatal(err)
	}
}

func (p *stdioPeer) next(want string) protocol.Message {
	p.t.Helper()
	for {
		m, err := p.dec.Decode()
		if err != nil {
			p.t.Fatalf("waiting for %s: %v", want, err)
		}
		if m.Type == want {
			return m
		}
	}
}

// TestServeStdioRunsALeaseAndKeepsSecretsOut proves serve --stdio speaks v1
// end to end: hello carries the node key id (stable across a restart), a
// lease runs through the executor, the result carries valid evidence, and
// neither the node key file's contents nor the job token reach the job's
// environment or output.
func TestServeStdioRunsALeaseAndKeepsSecretsOut(t *testing.T) {
	skipUnixOnly(t)
	state := useState(t)
	var ids []string
	for round := range 2 {
		p := startServe(t, "--coordinator", "peer-1")
		h := p.next("hello").Body.(*protocol.Hello)
		ids = append(ids, h.NodeKey.KeyID)
		if len(h.NodeKey.KeyID) != 16 || h.NodeKey.Alg != "ed25519" || h.Slots.Total != min(2, max(1, runtime.NumCPU()/2)) || h.AgentVersion != Version {
			t.Fatalf("hello %+v", h)
		}
		p.send("welcome", protocol.Welcome{Version: 1, MinAgentVersion: "1.4.0", SessionID: "s", HeartbeatMS: 60000, AgentTTLMS: 120000}, 0, "")
		if round == 0 {
			lease := protocol.Lease{LeaseID: "l1", AttemptID: "attempt-l1", Epoch: 1, Token: "JOBTOKEN-7f3a9c", CompatMode: protocol.Compat15,
				Job: model.Job{Kind: "static", Run: []string{"sh", "-c", "env; echo job-done"}}, Checkout: protocol.Checkout{Revision: rev40},
				Secrets: []protocol.SecretRef{}, ExpiresAt: "2030-01-01T00:00:00Z"}
			p.send("lease", lease, 1, "l1")
			var logs []string
			var res *protocol.Result
			for res == nil {
				m, err := p.dec.Decode()
				if err != nil {
					t.Fatal(err)
				}
				switch b := m.Body.(type) {
				case *protocol.Log:
					logs = append(logs, b.Text)
				case *protocol.Result:
					res = b
				}
			}
			all := strings.Join(logs, "\n")
			keyPEM, _ := os.ReadFile(filepath.Join(state, "node.key"))
			body := strings.Split(strings.TrimSpace(string(keyPEM)), "\n")[1]
			if !strings.Contains(all, "job-done") || strings.Contains(all, "JOBTOKEN") || strings.Contains(all, body) || strings.Contains(all, "NSELF_CI_AGENT_HOME") {
				t.Fatalf("job output leaked or missing:\n%s", all)
			}
			if m := validEvidence(t, res.Evidence); m["result"] != "pass" {
				t.Fatalf("evidence %v", m)
			}
			if res.Signature != (protocol.Signature{}) {
				t.Fatalf("unsigned result has a signature: %+v", res.Signature)
			}
		}
		p.send("drain", protocol.Drain{Deadline: time.Now().Add(time.Minute).UTC().Format(time.RFC3339)}, 0, "")
		p.next("ack")
		if code := <-p.done; code != 0 {
			t.Fatalf("serve exit %d", code)
		}
	}
	if ids[0] != ids[1] {
		t.Fatalf("key id changed across restart: %v", ids)
	}
	recs, err := session.ReadCoordinators(state)
	if err != nil || len(recs) != 1 || recs[0].ID != "peer-1" {
		t.Fatalf("coordinators %v %v", recs, err)
	}
}

func TestServeUsage(t *testing.T) {
	useState(t)
	for _, args := range [][]string{{"serve"}, {"serve", "--stdio", "extra"}, {"serve", "--stdio", "--coordinator", "bad id"}} {
		if code, _, _ := runCmd(t, args...); code != 2 {
			t.Errorf("%v exit %d, want 2", args, code)
		}
	}
}

const stubRecipient = "age1stubrecipient0000000000000000000000000000000000000000000"

// TestServeSendsAgeRecipientAndKeepsIdentityOutOfJobs proves, through serve,
// that hello carries the recipient from <state>/age.key (a stub age-keygen is
// on PATH) and that the identity file is in neither the job's environment nor
// its workspace.
func TestServeSendsAgeRecipientAndKeepsIdentityOutOfJobs(t *testing.T) {
	skipUnixOnly(t)
	state := useState(t)
	bin := t.TempDir()
	script := "#!/bin/sh\n[ \"$1\" = \"-o\" ] || exit 2\nprintf '# public key: " + stubRecipient + "\\nAGE-SECRET-KEY-STUBSTUB\\n' > \"$2\"\n"
	if err := os.WriteFile(filepath.Join(bin, "age-keygen"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	p := startServe(t)
	h := p.next("hello").Body.(*protocol.Hello)
	if h.AgeRecipient == nil || *h.AgeRecipient != stubRecipient {
		t.Fatalf("age_recipient %v", h.AgeRecipient)
	}
	if info, err := os.Stat(filepath.Join(state, "age.key")); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("age.key: %v %v", info, err)
	}
	p.send("welcome", protocol.Welcome{Version: 1, MinAgentVersion: "1.4.0", SessionID: "s", HeartbeatMS: 60000, AgentTTLMS: 120000}, 0, "")
	lease := protocol.Lease{LeaseID: "l1", AttemptID: "attempt-l1", Epoch: 1, Token: "T", CompatMode: protocol.Compat15,
		Job: model.Job{Kind: "static", Run: []string{"sh", "-c", "env; ls -a; echo done"}}, Checkout: protocol.Checkout{Revision: rev40},
		Secrets: []protocol.SecretRef{}, ExpiresAt: "2030-01-01T00:00:00Z"}
	p.send("lease", lease, 1, "l1")
	var logs []string
	for {
		m, err := p.dec.Decode()
		if err != nil {
			t.Fatal(err)
		}
		if lg, ok := m.Body.(*protocol.Log); ok {
			logs = append(logs, lg.Text)
		}
		if m.Type == "result" {
			break
		}
	}
	all := strings.Join(logs, "\n")
	if !strings.Contains(all, "done") || strings.Contains(all, "age.key") || strings.Contains(all, "AGE-SECRET") || strings.Contains(all, "node.key") {
		t.Fatalf("identity file visible to the job:\n%s", all)
	}
}

// TestServeMasksSecretsInTheEvidenceExcerpt proves a delivered secret that a
// failing job prints is masked in the evidence excerpt as well as in the log.
func TestServeMasksSecretsInTheEvidenceExcerpt(t *testing.T) {
	skipUnixOnly(t)
	useState(t)
	t.Setenv("PATH", filepath.Dir(mustLook(t, "sh")))
	p := startServe(t, "--accept-secret-values")
	p.next("hello")
	p.send("welcome", protocol.Welcome{Version: 1, MinAgentVersion: "1.4.0", SessionID: "s", HeartbeatMS: 60000, AgentTTLMS: 120000}, 0, "")
	lease := protocol.Lease{LeaseID: "l1", AttemptID: "attempt-l1", Epoch: 1, Token: "T", CompatMode: protocol.Compat15,
		Job:      model.Job{Kind: "static", Run: []string{"sh", "-c", `echo "got $TOKEN"; exit 3`}, Secrets: []string{"TOKEN"}},
		Checkout: protocol.Checkout{Revision: rev40}, Secrets: []protocol.SecretRef{{Ref: "project/TOKEN", Class: "project"}}, ExpiresAt: "2030-01-01T00:00:00Z"}
	p.send("lease", lease, 1, "l1")
	p.send("secrets", protocol.Secrets{LeaseID: "l1", Values: []protocol.SecretValue{{Ref: "project/TOKEN", Value: "opaqueValue123"}}}, 1, "l1")
	res := p.next("result").Body.(*protocol.Result)
	m := validEvidence(t, res.Evidence)
	if strings.Contains(string(res.Evidence), "opaqueValue123") || m["result"] != "fail" {
		t.Fatalf("evidence leaks the secret or did not fail: %s", res.Evidence)
	}
}

func mustLook(t *testing.T, name string) string {
	t.Helper()
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if p := filepath.Join(dir, name); fileExists(p) {
			return p
		}
	}
	t.Fatalf("%s not on PATH", name)
	return ""
}

func fileExists(p string) bool { _, err := os.Stat(p); return err == nil }

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@example.invalid", "-c", "commit.gpgsign=false"}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// TestExecEvidenceBindsOnlyToTheCheckedRevision proves evidence for a revision
// the source tree is not at is unbound (sha_mismatch, freshness unknown), and
// that the matching revision of a clean tree stays bound.
func TestExecEvidenceBindsOnlyToTheCheckedRevision(t *testing.T) {
	skipUnixOnly(t)
	useState(t)
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	repo := t.TempDir()
	git(t, repo, "init", "-q")
	if err := os.WriteFile(filepath.Join(repo, "f.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, repo, "add", "f.txt")
	git(t, repo, "commit", "-q", "-m", "a")
	head := git(t, repo, "rev-parse", "HEAD")
	for _, c := range []struct{ rev, binding string }{{head, "bound"}, {strings.Repeat("1", 40), "unbound"}} {
		spec := execSpec{AttemptID: "a-" + c.binding, CoordinatorID: "local", JobID: "j", Revision: c.rev, SourceDir: repo, Job: model.Job{Kind: "static", Run: []string{"true"}}}
		b, _ := json.Marshal(spec)
		sp := filepath.Join(t.TempDir(), "s.json")
		_ = os.WriteFile(sp, b, 0o600)
		ev := filepath.Join(t.TempDir(), "e.json")
		if code, _, errs := runCmd(t, "exec", "--spec", sp, "--evidence", ev); code != 0 {
			t.Fatalf("exec %d %s", code, errs)
		}
		doc, _ := os.ReadFile(ev)
		m := validEvidence(t, doc)
		bind, _ := m["binding"].(map[string]any)
		if bind["state"] != c.binding {
			t.Fatalf("revision %.8s: binding %v, want %s", c.rev, bind, c.binding)
		}
		if c.binding == "unbound" && (bind["reason"] != "sha_mismatch" || m["freshness"].(map[string]any)["state"] != "unknown") {
			t.Fatalf("unbound evidence: %v", m)
		}
	}
}

// TestExecWorkAreaIsOutsideTheStateDir proves executor work directories are
// not created under the agent state dir (which holds the keys).
func TestExecWorkAreaIsOutsideTheStateDir(t *testing.T) {
	skipUnixOnly(t)
	state := useState(t)
	t.Setenv("NSELF_CI_AGENT_WORK", filepath.Join(t.TempDir(), "work"))
	ev := filepath.Join(t.TempDir(), "e.json")
	if code, _, errs := runCmd(t, "exec", "--spec", writeSpec(t, "true"), "--evidence", ev); code != 0 {
		t.Fatalf("exec %d %s", code, errs)
	}
	if _, err := os.Stat(filepath.Join(state, "work")); err == nil {
		t.Fatal("work area created under the state dir")
	}
}

// TestSlotsCommandAndServeCannotRaiseTheCap proves `slots --set` is the
// operator's lever and that a larger --slots on the serve command line is ignored.
func TestSlotsCommandAndServeCannotRaiseTheCap(t *testing.T) {
	useState(t)
	if code, out, _ := runCmd(t, "slots", "--set", "1"); code != 0 || out != "cap 1 used 0\n" {
		t.Fatalf("%d %q", code, out)
	}
	p := startServe(t, "--slots", "64")
	h := p.next("hello").Body.(*protocol.Hello)
	if h.Slots.Total != 1 {
		t.Fatalf("hello slots %+v, want the operator's cap 1", h.Slots)
	}
	if code, _, _ := runCmd(t, "slots", "--set", "-1"); code != 2 {
		t.Fatalf("negative set exit %d", code)
	}
}

// TestSkippedJobsHaveSchemaValidEvidence proves a job that was cancelled before
// it started, or whose secrets never came, still yields ci.evidence/v1
// (cancelled / an infra check that ended error, so the run is error as in the local run, D-1098), not an ad-hoc document the coordinator would reject.
func TestSkippedJobsHaveSchemaValidEvidence(t *testing.T) {
	useState(t)
	for skip, want := range map[string]string{"cancelled": "cancelled", "secrets_missing": "error"} {
		spec := execSpec{AttemptID: "a-" + skip, CoordinatorID: "c", JobID: "j", Revision: rev40, Job: model.Job{Kind: "static", Run: []string{"true"}}}
		res, err := runSpec(context.Background(), spec, jobInputs{Skip: skip})
		if err != nil {
			t.Fatal(err)
		}
		if m := validEvidence(t, res.Evidence); m["result"] != want {
			t.Fatalf("%s: result %v, want %s", skip, m["result"], want)
		}
	}
}

// TestNoSourceTreeIsUnbound proves a job run without a source directory never
// produces bound or current evidence (and git never reads the agent's cwd).
func TestNoSourceTreeIsUnbound(t *testing.T) {
	skipUnixOnly(t)
	useState(t)
	spec := execSpec{AttemptID: "a1", CoordinatorID: "c", JobID: "j", Revision: rev40, Job: model.Job{Kind: "static", Run: []string{"true"}}}
	res, err := runSpec(context.Background(), spec, jobInputs{})
	if err != nil {
		t.Fatal(err)
	}
	m := validEvidence(t, res.Evidence)
	if m["binding"].(map[string]any)["state"] != "unbound" || m["freshness"].(map[string]any)["state"] != "unknown" {
		t.Fatalf("evidence %v", m)
	}
}

// TestWorkBaseRefusesASymlink proves the executor work area is a real private
// directory: a symlink there is an error, a loose directory is tightened to 0700.
func TestWorkBaseRefusesASymlink(t *testing.T) {
	skipUnixOnly(t)
	root := t.TempDir()
	loose := filepath.Join(root, "loose")
	_ = os.Mkdir(loose, 0o777)
	_ = os.Chmod(loose, 0o777)
	t.Setenv("NSELF_CI_AGENT_WORK", loose)
	if dir, err := workBase(); err != nil || dir != loose {
		t.Fatalf("%q %v", dir, err)
	}
	if info, _ := os.Stat(loose); info.Mode().Perm() != 0o700 {
		t.Fatalf("mode %v", info.Mode().Perm())
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(loose, link); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NSELF_CI_AGENT_WORK", link)
	if _, err := workBase(); err == nil {
		t.Fatal("followed a symlink")
	}
}
