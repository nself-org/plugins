package main

// Purpose: tests of `--coordinator` and the read commands against a live local serve (P7-NODE-19): the
// flag exists only on the V15 bare flag set, a bare run is sent to the serve instead of the local engine,
// refused flags stop before anything is sent, a failed target never falls back to the local store, and the
// committed HEAD of a github.com repository is what is submitted.
// Inputs:  a real unix-socket user API with a scripted backend and a real SQLite store for the role service.
// Outputs: pass or fail.
// Constraints: no network; the socket lives in a short temp dir (sun_path is 104 bytes on macOS).

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/audit"
	"github.com/nself-org/plugins/free/ci/internal/coordclient"
	"github.com/nself-org/plugins/free/ci/internal/coordinator/clientapi"
	"github.com/nself-org/plugins/free/ci/internal/detach"
	"github.com/nself-org/plugins/free/ci/internal/logs"
	"github.com/nself-org/plugins/free/ci/internal/roles"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/schedpolicy"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

const coordRev = "0123456789abcdef0123456789abcdef01234567"

// scriptedBackend is a user API backend whose single run is already finished.
type scriptedBackend struct {
	evidence json.RawMessage // when set, returned instead of the golden document
	mu       sync.Mutex
	submits  []clientapi.SubmitRequest
	cancels  int
}

func (b *scriptedBackend) item() clientapi.RunItem {
	return clientapi.RunItem{RunView: cirun.RunView{ID: "run-77", Revision: coordRev, Status: "passed", Jobs: []cirun.JobView{{Name: "build", State: "passed"}}}, Project: "acme/app"}
}

func (b *scriptedBackend) Submit(_ context.Context, r clientapi.SubmitRequest, _ string) (clientapi.SubmitResult, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.submits = append(b.submits, r)
	return clientapi.SubmitResult{ID: "in-77", State: "queued", Outcome: "enqueued"}, nil
}
func (b *scriptedBackend) List(context.Context, int) ([]clientapi.RunItem, error) {
	return []clientapi.RunItem{b.item()}, nil
}
func (b *scriptedBackend) Get(_ context.Context, ref string) (clientapi.RunItem, bool, error) {
	if ref == "run-77" || ref == "in-77" || ref == "latest" {
		return b.item(), true, nil
	}
	return clientapi.RunItem{}, false, nil
}
func (b *scriptedBackend) Cancel(context.Context, string, string) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.cancels++
	return 2, nil
}
func (b *scriptedBackend) Logs(_ context.Context, q clientapi.LogQuery, fn func(logs.Line) error) (bool, error) {
	if q.Job != "build" {
		return false, nil
	}
	return true, fn(logs.Line{TS: "2026-10-19T00:00:00Z", Stream: "out", Seq: 1, Text: "hello from the coordinator"})
}
func (b *scriptedBackend) Evidence(context.Context, string) (json.RawMessage, error) {
	if b.evidence != nil {
		return b.evidence, nil
	}
	raw, _ := os.ReadFile("../internal/attest/testdata/adversarial/good.json")
	var f struct{ Evidence json.RawMessage }
	_ = json.Unmarshal(raw, &f)
	return f.Evidence, nil
}

// localServe starts a user API on serve.sock under a fresh CI home and points NSELF_CI_HOME at it.
func localServe(t *testing.T) (*scriptedBackend, string) {
	t.Helper()
	home, err := os.MkdirTemp("", "ci-cmd")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(home) })
	st, err := store.Open(filepath.Join(t.TempDir(), "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	release, err := coordclient.AcquireServeLock(home)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)
	ln, err := clientapi.ListenUnix(coordclient.SocketPath(home))
	if err != nil {
		t.Fatal(err)
	}
	be := &scriptedBackend{}
	api := clientapi.New(clientapi.Config{Roles: roles.New(st, &audit.Log{St: st}), Local: true, Operator: "cli:alice", Backend: be, Poll: 10 * time.Millisecond})
	ctx, stop := context.WithCancel(context.Background())
	t.Cleanup(stop)
	go func() { _ = clientapi.Serve(ctx, ln, api.Handler()) }()
	t.Setenv("NSELF_CI_HOME", home)
	t.Setenv("NSELF_CI_COORDINATOR", "")
	return be, home
}

// fakeRepo makes the hook see a clean github.com repository at coordRev.
func fakeRepo(t *testing.T, remote, dirty string) {
	t.Helper()
	old := coordGit
	coordGit = func(_ string, args ...string) (string, bool) {
		switch strings.Join(args, " ") {
		case "rev-parse HEAD":
			return coordRev + "\n", true
		case "config --get remote.origin.url":
			return remote + "\n", remote != ""
		case "status --porcelain --untracked-files=no":
			return dirty, true
		}
		return "", false
	}
	t.Cleanup(func() { coordGit = old })
}

// hook runs coordinatorOptions with captured output and exit.
func hook(t *testing.T, o RunOptions) (exit int, exited bool, out, errw string) {
	t.Helper()
	var ob, eb bytes.Buffer
	oldO, oldE, oldX := coordOut, coordErr, coordExit
	coordOut, coordErr = &ob, &eb
	coordExit = func(c int) { exit, exited = c, true }
	defer func() { coordOut, coordErr, coordExit = oldO, oldE, oldX }()
	if err := coordinatorOptions(&o); err != nil {
		t.Fatal(err)
	}
	return exit, exited, ob.String(), eb.String()
}

func TestCoordinatorFlagIsV15Only(t *testing.T) {
	if bareFlags.Lookup("coordinator") == nil {
		t.Fatal("--coordinator is not on the V15 bare flag set")
	}
	if _, ok := subcommands["serve"]; !ok {
		t.Fatal("the frozen v1.4 serve entry is gone")
	}
}

func TestCoordinatorHookSendsBareRunToLocalServe(t *testing.T) {
	be, _ := localServe(t)
	fakeRepo(t, "git@github.com:acme/app.git", " M a.go\n")
	exit, exited, out, errw := hook(t, RunOptions{Root: t.TempDir(), Pipeline: "default"})
	if !exited || exit != 0 {
		t.Fatalf("exit %d (exited %v)\n%s\n%s", exit, exited, out, errw)
	}
	if len(be.submits) != 1 || be.submits[0].Project != "acme/app" || be.submits[0].Revision != coordRev || be.submits[0].Pipeline != "default" {
		t.Fatalf("submitted %+v", be.submits)
	}
	if !strings.Contains(out, "run run-77: passed") || !strings.Contains(errw, "uncommitted changes are not part of the run") {
		t.Fatalf("out %q err %q", out, errw)
	}
}

func TestCoordinatorHookJSONCarriesTheContractLines(t *testing.T) {
	localServe(t)
	fakeRepo(t, "https://github.com/acme/app.git", "")
	exit, _, out, _ := hook(t, RunOptions{Root: t.TempDir(), JSON: true})
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if exit != 0 || len(lines) < 3 || !strings.Contains(lines[0], `"type":"queued"`) || !strings.Contains(lines[len(lines)-1], `"type":"result"`) {
		t.Fatalf("exit %d, lines:\n%s", exit, out)
	}
}

func TestCoordinatorHookRefusesWhatItCannotHonor(t *testing.T) {
	be, _ := localServe(t)
	fakeRepo(t, "git@github.com:acme/app.git", "")
	for name, o := range map[string]RunOptions{
		"--job": {Job: "build"}, "--service": {Services: []string{"db"}}, "--network": {Network: "none"}, "--container": {Container: true},
		"--timeout": {Timeout: time.Minute}, "--trust": {Trust: "owner"}, "ad hoc": {Adhoc: []string{"true"}},
		"--sha": {SHA: "abc123"}, "--force": {Force: true}, "--no-status": {NoStatus: true}, "--confirm-flaky": {ConfirmFlaky: 2}, "--trigger": {Trigger: "push"},
	} {
		o.Root = t.TempDir()
		exit, exited, _, errw := hook(t, o)
		if !exited || exit != 1 || !strings.Contains(errw, "cannot be used with a coordinator") {
			t.Errorf("%s: exit %d exited %v %q", name, exit, exited, errw)
		}
	}
	if len(be.submits) != 0 {
		t.Fatal("a refused run reached the coordinator")
	}
}

func TestCoordinatorHookNeedsAGithubCommit(t *testing.T) {
	be, _ := localServe(t)
	fakeRepo(t, "https://gitlab.com/acme/app.git", "")
	if exit, exited, _, errw := hook(t, RunOptions{Root: t.TempDir()}); !exited || exit != 1 || !strings.Contains(errw, "not a github.com repository") {
		t.Fatalf("%d %v %q", exit, exited, errw)
	}
	fakeRepo(t, "", "")
	if exit, exited, _, errw := hook(t, RunOptions{Root: t.TempDir()}); !exited || exit != 1 || !strings.Contains(errw, "not a github.com repository") {
		t.Fatalf("%d %v %q", exit, exited, errw)
	}
	if len(be.submits) != 0 {
		t.Fatal("nothing may be submitted without a project")
	}
}

func TestCoordinatorHookStaysOutOfTheWay(t *testing.T) {
	t.Setenv("NSELF_CI_HOME", t.TempDir())
	t.Setenv("NSELF_CI_COORDINATOR", "")
	if _, exited, _, _ := hook(t, RunOptions{Root: t.TempDir()}); exited {
		t.Fatal("with no coordinator and no live serve the run stays in-process")
	}
	localServe(t)
	detachFlag = true
	defer func() { detachFlag = false }()
	if _, exited, _, _ := hook(t, RunOptions{Root: t.TempDir()}); exited {
		t.Fatal("--detach hands the run to its child, which submits")
	}
}

func TestCoordinatorBadAddressNeverRunsLocally(t *testing.T) {
	t.Setenv("NSELF_CI_HOME", t.TempDir())
	t.Setenv("NSELF_CI_COORDINATOR", "http://ci.example:3865")
	fakeRepo(t, "git@github.com:acme/app.git", "")
	exit, exited, _, errw := hook(t, RunOptions{Root: t.TempDir()})
	if !exited || exit != 1 || !strings.Contains(errw, "plain_http") {
		t.Fatalf("a plain http coordinator must stop the run with E664 plain_http: %d %v %q", exit, exited, errw)
	}
}

func TestCoordinatorReadCommandsUseTheServe(t *testing.T) {
	be, _ := localServe(t)
	ctx := context.Background()
	code, out, errs := capture(t, func() int { c, _ := coordRuns(ctx, "", false); return c })
	if code != 0 || !strings.Contains(out, "run-77") {
		t.Fatalf("runs: %d %q %q", code, out, errs)
	}
	code, out, _ = capture(t, func() int { c, _ := coordRuns(ctx, "run-77", true); return c })
	if code != 0 || !strings.Contains(out, `"id": "run-77"`) {
		t.Fatalf("show: %d %q", code, out)
	}
	if code, out, errs = capture(t, func() int { c, ok := coordRuns(ctx, "no-such", false); return map[bool]int{true: c, false: -1}[ok] }); code != 1 || !strings.Contains(errs, "not_found") {
		t.Fatalf("a missing run on an active target is an error, never a local lookup: %d %q %q", code, out, errs)
	}
	code, out, _ = capture(t, func() int { c, _ := coordCancel(ctx, "run-77"); return c })
	if code != 0 || be.cancels != 1 || !strings.Contains(out, "cancel requested for 2 attempt(s)") {
		t.Fatalf("cancel: %d %q", code, out)
	}
	code, out, _ = capture(t, func() int { c, _ := coordLogs(ctx, logsFlags{run: "run-77"}, "build"); return c })
	if code != 0 || !strings.Contains(out, "hello from the coordinator") {
		t.Fatalf("logs: %d %q", code, out)
	}
	code, out, _ = capture(t, func() int { c, _ := coordEvidence(ctx, evidenceFlags{json: true}, "run-77"); return c })
	if code != 0 || !strings.Contains(out, `"ci.evidence/v1"`) || !strings.Contains(out, `"coordinator-recorded"`) {
		t.Fatalf("evidence: %d %q", code, out)
	}
}

func TestCoordinatorReadCommandsLeaveLocalAloneWhenInactive(t *testing.T) {
	t.Setenv("NSELF_CI_HOME", t.TempDir())
	t.Setenv("NSELF_CI_COORDINATOR", "")
	for name, f := range map[string]func() bool{
		"runs":     func() bool { _, ok := coordRuns(context.Background(), "", false); return ok },
		"cancel":   func() bool { _, ok := coordCancel(context.Background(), "x"); return ok },
		"logs":     func() bool { _, ok := coordLogs(context.Background(), logsFlags{}, "build"); return ok },
		"evidence": func() bool { _, ok := coordEvidence(context.Background(), evidenceFlags{}, "x"); return ok },
	} {
		if f() {
			t.Errorf("%s was handled with no coordinator", name)
		}
	}
}

func TestGithubRemoteForms(t *testing.T) {
	for raw, want := range map[string]string{
		"https://github.com/acme/app.git":         "acme/app",
		"https://github.com/acme/app":             "acme/app",
		"git@github.com:acme/app.git":             "acme/app",
		"ssh://git@github.com/acme/app.git":       "acme/app",
		"https://github.com/acme/my.app-1/":       "acme/my.app-1",
		"https://evilgithub.com/acme/app":         "",
		"https://github.com.evil.io/a/b":          "",
		"https://github.com/acme/app/extra":       "",
		"https://user" + ":pw@github.com/acme/ap": "",
		"git@gitlab.com:acme/app.git":             "",
	} {
		m := githubRe.FindStringSubmatch(raw)
		got := ""
		if m != nil {
			got = m[1] + "/" + m[2]
		}
		if got != want {
			t.Errorf("%q: %q, want %q", raw, got, want)
		}
	}
}

func TestCoordinatorHookRefusesConfigAndBase(t *testing.T) {
	be, _ := localServe(t)
	fakeRepo(t, "git@github.com:acme/app.git", "")
	for name, set := range map[string]func(){"--config": func() { bare.cfg = "/tmp/x.yaml" }, "--base": func() { bare.base = "abc" }} {
		set()
		exit, exited, _, errw := hook(t, RunOptions{Root: t.TempDir()})
		bare.cfg, bare.base = "", ""
		if !exited || exit != 1 || !strings.Contains(errw, name) {
			t.Errorf("%s: exit %d exited %v %q", name, exit, exited, errw)
		}
	}
	if len(be.submits) != 0 {
		t.Fatal("a refused run reached the coordinator")
	}
}

func TestCoordinatorHelpNeverSubmits(t *testing.T) {
	be, _ := localServe(t)
	fakeRepo(t, "git@github.com:acme/app.git", "")
	bare.help = true
	defer func() { bare.help = false }()
	if _, exited, _, _ := hook(t, RunOptions{Root: t.TempDir()}); exited || len(be.submits) != 0 {
		t.Fatalf("--help must print help and send nothing (exited %v, submits %d)", exited, len(be.submits))
	}
}

// TestCoordinatorPathsNeverCreateALocalStore is the file probe: the CI home holds only the serve's lock and
// socket, and neither the submit hook nor any read command may leave a database or a journal beside them.
func TestCoordinatorPathsNeverCreateALocalStore(t *testing.T) {
	_, home := localServe(t)
	fakeRepo(t, "git@github.com:acme/app.git", "")
	hook(t, RunOptions{Root: t.TempDir()})
	ctx := context.Background()
	coordRuns(ctx, "", false)
	coordRuns(ctx, "run-77", false)
	coordCancel(ctx, "run-77")
	coordLogs(ctx, logsFlags{run: "run-77"}, "build")
	coordEvidence(ctx, evidenceFlags{}, "run-77")
	entries, _ := os.ReadDir(home)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if strings.Join(names, ",") != "serve.lock,serve.sock" {
		t.Fatalf("the CI home holds %v; a state.db or journal means a path opened the local store", names)
	}
}

func TestStoreWritersRefuseWhileAServeOwnsTheHome(t *testing.T) {
	localServe(t)
	if code, out, errs := capture(t, func() int { return runRunsPrune([]string{}) }); code != 1 || !strings.Contains(errs, "serve_unreachable") {
		t.Fatalf("prune: %d %q %q", code, out, errs)
	}
	if code, _, errs := capture(t, func() int { return runResolve([]string{"att-1", "--idempotency-key", "k-1", "--abandon", "--yes"}) }); code != 1 || !strings.Contains(errs, "serve_unreachable") {
		t.Fatalf("resolve: %d %q", code, errs)
	}
	if _, err := os.Stat(filepath.Join(os.Getenv("NSELF_CI_HOME"), "state.db")); err == nil {
		t.Fatal("a refused writer still created the database")
	}
}

func FuzzGithubRemote(f *testing.F) {
	for _, s := range []string{"https://github.com/acme/app.git", "git@github.com:acme/app.git", "ssh://git@github.com/a/b", "https://github.com/a/b/c", "https://github.com/-a/b", "", "\x00"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		m := githubRe.FindStringSubmatch(raw)
		if m == nil {
			return
		}
		if strings.ContainsAny(m[1]+m[2], "/@:? \t\r\n\x00") || !strings.Contains(raw, "github.com") {
			t.Fatalf("%q produced %q/%q", raw, m[1], m[2])
		}
	})
}

// The scheduling and image flags are applied by hooks that run after the coordinator hook, so the
// refusal reads their values: a run must never go out with one of them silently dropped.
func TestCoordinatorHookRefusesFlagsAppliedByLaterHooks(t *testing.T) {
	be, _ := localServe(t)
	fakeRepo(t, "git@github.com:acme/app.git", "")
	defer func() { schedFlags = schedpolicy.Flags{}; adhocFlags.Image = "" }()
	for name, set := range map[string]func(){
		"--prefer": func() { schedFlags.Prefer = "fast" }, "--runner": func() { schedFlags.Runner = "mm4" },
		"--max-queue": func() { schedFlags.MaxQueue = "5m" }, "--image": func() { adhocFlags.Image = "x@sha256:" + strings.Repeat("a", 64) },
	} {
		schedFlags, adhocFlags.Image = schedpolicy.Flags{}, ""
		set()
		exit, exited, _, errw := hook(t, RunOptions{Root: t.TempDir()})
		if !exited || exit != 1 || !strings.Contains(errw, name) {
			t.Errorf("%s: exit %d exited %v %q", name, exit, exited, errw)
		}
	}
	if len(be.submits) != 0 {
		t.Fatal("a refused run reached the coordinator")
	}
}

// `--detach` under a live serve must not verify against the serve's database (that opens it for writing):
// the detached child submits over the socket and its first record is the proof.
func TestDetachUnderALiveServeOpensNoStore(t *testing.T) {
	localServe(t)
	oldExe, oldArgs, oldTune, oldExit := detachExe, detachArgs, detachTune, detachExit
	defer func() { detachExe, detachArgs, detachTune, detachExit = oldExe, oldArgs, oldTune, oldExit }()
	detachExe = func() (string, error) { return "/usr/bin/true", nil }
	detachArgs = func() []string { return []string{"--json"} }
	var verifier = "unset"
	detachTune = func(p *detach.Params) {
		verifier = "none"
		if p.Verify != nil {
			verifier = "local store"
		}
	}
	var out bytes.Buffer
	oldOut := detachOut
	detachOut = &out
	defer func() { detachOut = oldOut }()
	_ = runDetach(&RunOptions{JSON: true, Home: os.Getenv("NSELF_CI_HOME")})
	if verifier != "none" {
		t.Fatalf("detach verifier = %s; under a live serve it must not read the local store", verifier)
	}
}

// The evidence a coordinator returns is verified before it is printed; a document that contradicts itself
// is refused, not shown.
func TestCoordinatorEvidenceIsVerifiedBeforePrinting(t *testing.T) {
	be, _ := localServe(t)
	good, _ := be.Evidence(context.Background(), "x")
	var m map[string]any
	if err := json.Unmarshal(good, &m); err != nil || m["pipeline"] == nil {
		t.Fatalf("fixture: %v", err)
	}
	delete(m, "pipeline") // a document that is not a valid ci.evidence/v1
	tampered, _ := json.Marshal(m)
	be.evidence = tampered
	code, out, errs := capture(t, func() int { c, _ := coordEvidence(context.Background(), evidenceFlags{}, "run-77"); return c })
	if code == 0 || !strings.Contains(errs, "does not verify") || strings.Contains(out, "run-77") {
		t.Fatalf("a tampered document was shown: exit %d out %q err %q", code, out, errs)
	}
}

// `runs prune` goes through pruneStreams, which used to open the store before any refusal: under a live serve
// nothing may be opened or created.
func TestPruneThroughTheRegisteredEntryTouchesNoStoreUnderAServe(t *testing.T) {
	_, home := localServe(t)
	prune := runsSubs["prune"]
	if prune == nil {
		t.Fatal("runs prune is not registered")
	}
	code, _, errs := capture(t, func() int { return prune([]string{}) })
	if code != 1 || !strings.Contains(errs, "serve_unreachable") {
		t.Fatalf("exit %d %q", code, errs)
	}
	entries, _ := os.ReadDir(home)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if strings.Join(names, ",") != "serve.lock,serve.sock" {
		t.Fatalf("prune left %v in the CI home; it opened the store", names)
	}
}
