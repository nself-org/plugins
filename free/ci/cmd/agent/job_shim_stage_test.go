package main

// Purpose: the job-shim workspace hand-off and cleanup (D-1230): a process step on a separated node reaches
// the shim in a 0700 workspace the job user created and owns; another job user cannot reach it; a planted
// link redirects nothing; cleanup kills the job user's processes and removes its files as that user; a
// failed cleanup drains the node.
// Inputs:  a fake shim runner (fakeShim) that runs the shim's ops in-process as the test user, which plays
//          the job user; a fake sudo script for the step itself. Constraints: no real sudo, no real kill.

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

// shimRecord is one call the fake runner received.
type shimRecord struct {
	user string
	req  shimRequest
}

// fakeShimRig is a jobs directory with the slot roots of two job users and a recorder of shim calls.
type fakeShimRig struct {
	jobs  string
	mu    sync.Mutex
	calls []shimRecord
	kills int
	fail  map[string]error // op -> forced failure
}

// fakeShim points the shim seams at a temporary jobs directory. Each request crosses as its JSON line and is
// parsed by the shim's own parser before the op runs, as on a node.
func fakeShim(t *testing.T) *fakeShimRig {
	t.Helper()
	rig := &fakeShimRig{jobs: t.TempDir(), fail: map[string]error{}}
	for _, n := range []string{"1", "2"} {
		if err := os.Mkdir(filepath.Join(rig.jobs, n), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	jd, kill, call, uid := shimJobsDir, shimKillAll, shimCall, shimLookupUID
	shimJobsDir = func() string { return rig.jobs }
	shimKillAll = func() error { rig.mu.Lock(); rig.kills++; rig.mu.Unlock(); return nil }
	shimCall = func(_ context.Context, user string, req shimRequest, in io.Reader, out io.Writer) error {
		line, err := json.Marshal(req)
		if err != nil {
			return err
		}
		parsed, err := parseShimRequest(line)
		if err != nil {
			return err
		}
		rig.mu.Lock()
		rig.calls = append(rig.calls, shimRecord{user, parsed})
		forced := rig.fail[parsed.Op]
		rig.mu.Unlock()
		if forced != nil {
			return forced
		}
		if out == nil {
			out = io.Discard
		}
		return runShimOp(parsed, user, in, out)
	}
	shimLookupUID = func(name string) (int, error) {
		if !jobUserName.MatchString(name) {
			return 0, errors.New("no such user")
		}
		return 4000 + len(name), nil // never the test's own UID
	}
	t.Cleanup(func() {
		shimJobsDir, shimKillAll, shimCall, shimLookupUID = jd, kill, call, uid
		nodeDrain.Lock()
		nodeDrain.reason = ""
		nodeDrain.Unlock()
	})
	return rig
}

func ownerOnly(t *testing.T, dir string) {
	t.Helper()
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := os.Lstat(p)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink == 0 && info.Mode().Perm()&0o077 != 0 {
			t.Errorf("%s has mode %v: group or world bits", p, info.Mode().Perm())
		}
		if !ownedBySelf(info) {
			t.Errorf("%s is not owned by the job user", p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestProcessJobReachesTheShimInAWorkspaceTheJobUserOwns(t *testing.T) {
	rig := fakeShim(t)
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	if err := os.MkdirAll(filepath.Join(src, "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "pkg", "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(dir, "outside")
	if err := os.Mkdir(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(outside, "node.key")
	if err := os.WriteFile(secret, []byte("k"), 0o600); err != nil {
		t.Fatal(err)
	}
	stdinFile := filepath.Join(dir, "stdin")
	fake := filepath.Join(dir, "sudo")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\ncat > '"+stdinFile+"'\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	old := shimSudo
	shimSudo = fake
	t.Cleanup(func() { shimSudo = old })
	asNode(t, "/var/lib/nself-ci-agent", "nself-ci", "/opt/nself-ci-agent/bin/nself-ci-agent")
	ex := &ciexec.Executor{Isolation: systemShim(0)(ciexec.BaselineIsolator{})}
	// links planted in the agent's workspace before the step: an absolute one to the key, one to a directory
	plant := func(_ context.Context, ws *os.Root) error {
		if err := ws.Symlink(secret, "key-link"); err != nil {
			return err
		}
		return ws.Symlink(outside, "dir-link")
	}
	res := ex.Run(context.Background(), ciexec.JobSpec{AttemptID: "stage1", CoordinatorID: "coord", Home: t.TempDir(), SourceDir: src,
		Job: model.Job{Workdir: "pkg"}, Command: []string{"/bin/sh", "-c", "true"}, BeforeJob: plant})
	if res.Attempt.State != "passed" {
		t.Fatalf("state=%s err=%v", res.Attempt.State, res.Err)
	}
	raw, _ := os.ReadFile(stdinFile)
	line, _, _ := strings.Cut(string(raw), "\n")
	req, err := parseShimRequest([]byte(line))
	if err != nil {
		t.Fatal(err)
	}
	attempt := filepath.Join(rig.jobs, "1", "stage1")
	if req.Dir != filepath.Join(attempt, "ws", "pkg") {
		t.Fatalf("the step runs in %q, not in the job user's copy", req.Dir)
	}
	env := strings.Join(req.Env, "\n")
	if !strings.Contains(env, "HOME="+filepath.Join(attempt, "home")+"\n") || !strings.Contains(env, "TMPDIR="+filepath.Join(attempt, "tmp")) {
		t.Fatalf("HOME and TMPDIR are not the job user's: %q", req.Env)
	}
	if strings.Count(env, "HOME=") != 1 {
		t.Fatalf("HOME is set twice: %q", req.Env)
	}
	if got, _ := os.ReadFile(filepath.Join(attempt, "ws", "pkg", "main.go")); string(got) != "package main\n" {
		t.Fatalf("the source did not reach the job user's copy: %q", got)
	}
	ownerOnly(t, attempt)
	// the links crossed as links: nothing outside changed mode, and the key was not copied in
	for _, l := range []string{"key-link", "dir-link"} {
		info, err := os.Lstat(filepath.Join(attempt, "ws", l))
		if err != nil || info.Mode()&os.ModeSymlink == 0 {
			t.Fatalf("%s is not a link in the job copy: %v %v", l, info, err)
		}
	}
	if info, _ := os.Stat(secret); info.Mode().Perm() != 0o600 {
		t.Fatalf("the hand-off changed the key's mode: %v", info.Mode())
	}
	if info, _ := os.Stat(outside); info.Mode().Perm() != 0o755 {
		t.Fatalf("the hand-off changed a linked directory: %v", info.Mode())
	}
	if entries, _ := os.ReadDir(outside); len(entries) != 1 {
		t.Fatalf("the hand-off wrote through a link: %v", entries)
	}
	for _, c := range rig.calls {
		if c.user != "nself-ci-job-1" || c.req.Dir != "" || len(c.req.Argv) != 0 {
			t.Fatalf("an op request carried a path or ran as another user: %+v", c)
		}
	}
}

func TestAnotherJobUserCannotReachTheWorkspace(t *testing.T) {
	rig := fakeShim(t)
	ws := t.TempDir()
	if err := os.WriteFile(filepath.Join(ws, "f"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	dir, _, err := shimStager{user: "nself-ci-job-1"}.StageIn(context.Background(), "a1", ws)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{filepath.Join(rig.jobs, "1"), filepath.Dir(dir), dir} {
		if info, _ := os.Lstat(p); info.Mode().Perm() != 0o700 {
			t.Fatalf("%s is %v: another user could enter it", p, info.Mode().Perm())
		}
	}
	// job user 2's shim works only under its own slot root: it can neither read nor remove job 1's copy
	out := t.TempDir()
	if err := (shimStager{user: "nself-ci-job-2"}).StageOut(context.Background(), "a1", out); err == nil {
		t.Fatal("job user 2 read job user 1's attempt")
	}
	if err := (shimJobs{user: "nself-ci-job-2", uid: 4015}).EmptyRoot(context.Background(), 4015, rig.jobs+"/1", "a1"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "f")); err != nil {
		t.Fatalf("job user 2's clean reached job user 1's files: %v", err)
	}
	// a request can never name a path
	for _, raw := range []string{`{"op":"clean","attempt":"../1/a1"}`, `{"op":"clean","attempt":"a1","dir":"/x"}`, `{"op":"stage-in","attempt":"a1","argv":["sh"]}`, `{"op":"chown","attempt":"a1"}`} {
		if _, err := parseShimRequest([]byte(raw)); err == nil {
			t.Fatalf("request %s was accepted", raw)
		}
	}
	// a runner never acts for another UID
	if err := (shimJobs{user: "nself-ci-job-1", uid: 4015}).KillAll(context.Background(), 4016); err == nil {
		t.Fatal("the runner killed another UID's processes")
	}
}

func TestPlantedLinksRedirectNothing(t *testing.T) {
	rig := fakeShim(t)
	target := t.TempDir()
	// an attempt directory planted as a link before stage-in is refused, never reused
	if err := os.Symlink(target, filepath.Join(rig.jobs, "1", "a2")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := (shimStager{user: "nself-ci-job-1"}).StageIn(context.Background(), "a2", t.TempDir()); err == nil {
		t.Fatal("stage-in reused a planted attempt link")
	}
	if entries, _ := os.ReadDir(target); len(entries) != 0 {
		t.Fatalf("stage-in wrote through a planted link: %v", entries)
	}
	// a slot root that is a link is refused
	if err := os.Rename(filepath.Join(rig.jobs, "2"), filepath.Join(rig.jobs, "2-real")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(rig.jobs, "2")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := (shimStager{user: "nself-ci-job-2"}).StageIn(context.Background(), "a3", t.TempDir()); err == nil {
		t.Fatal("a linked slot root was used")
	}
	// a tree whose link is followed by an entry beneath it cannot write outside
	pr, pw := io.Pipe()
	go func() {
		src, _ := os.OpenRoot(t.TempDir())
		_ = src.Symlink(target, "out")
		_ = pw.CloseWithError(packTree(src, pw))
	}()
	dst, _ := os.OpenRoot(t.TempDir())
	if err := unpackTree(dst, pr); err != nil {
		t.Fatal(err)
	}
	if f, err := dst.OpenFile("out/x", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600); err == nil {
		_ = f.Close()
		t.Fatal("a write went through an escaping link")
	}
	// the job's links come back as links on stage-out; cleanup removes them without following
	ws := t.TempDir()
	if _, _, err := (shimStager{user: "nself-ci-job-1"}).StageIn(context.Background(), "a4", ws); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(rig.jobs, "1", "a4", "ws", "evil")); err != nil {
		t.Fatal(err)
	}
	if err := (shimStager{user: "nself-ci-job-1"}).StageOut(context.Background(), "a4", ws); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Lstat(filepath.Join(ws, "evil")); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("the job's link did not come back as a link: %v", err)
	}
	if err := cleanupShimAttempt(context.Background(), "nself-ci-job-1", t.TempDir(), "a4"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("cleanup followed a link: %v", err)
	}
}

func TestCleanupKillsAndRemovesAsTheJobUser(t *testing.T) {
	rig := fakeShim(t)
	if _, _, err := (shimStager{user: "nself-ci-job-1"}).StageIn(context.Background(), "a5", t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rig.jobs, "1", "a5", "tmp", "left"), []byte("x"), 0o400); err != nil {
		t.Fatal(err)
	}
	if err := cleanupShimAttempt(context.Background(), "nself-ci-job-1", t.TempDir(), "a5"); err != nil {
		t.Fatalf("cleanup of a healthy attempt failed: %v", err)
	}
	if drainedError() != nil {
		t.Fatal("a healthy cleanup drained the node")
	}
	if rig.kills != 1 {
		t.Fatalf("the job user's processes were killed %d times", rig.kills)
	}
	if _, err := os.Lstat(filepath.Join(rig.jobs, "1", "a5")); !os.IsNotExist(err) {
		t.Fatalf("the job user's files are left: %v", err)
	}
	if _, err := os.Stat(filepath.Join(rig.jobs, "1")); err != nil {
		t.Fatalf("the slot root itself was removed: %v", err)
	}
	ops := []string{}
	for _, c := range rig.calls {
		if c.user != "nself-ci-job-1" {
			t.Fatalf("an op ran as %s", c.user)
		}
		ops = append(ops, c.req.Op)
	}
	if strings.Join(ops, ",") != "stage-in,kill,clean" {
		t.Fatalf("ops = %v", ops)
	}
}

func TestFailedCleanupDrainsTheNode(t *testing.T) {
	rig := fakeShim(t)
	rig.fail[opClean] = errors.New("sudo: a password is required")
	err := cleanupShimAttempt(context.Background(), "nself-ci-job-1", t.TempDir(), "a6")
	if err == nil || !strings.Contains(err.Error(), "E681") {
		t.Fatalf("err = %v", err)
	}
	if drainedError() == nil {
		t.Fatal("a failed cleanup left the node taking work")
	}
	marker := filepath.Join(t.TempDir(), "ran")
	spec := execSpec{AttemptID: "a7", Revision: strings.Repeat("a", 40), Job: model.Job{Run: []string{"sh", "-c", "touch '" + marker + "'"}}}
	res, _, err := execute(context.Background(), spec, jobInputs{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Attempt.State == "passed" || !res.Refused || res.Err == nil || !strings.Contains(res.Err.Error(), "E681") {
		t.Fatalf("a drained node ran a job: %+v", res)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("the payload ran on a drained node")
	}
	// a lookup failure drains too
	nodeDrain.Lock()
	nodeDrain.reason = ""
	nodeDrain.Unlock()
	if err := cleanupShimAttempt(context.Background(), "root", t.TempDir(), "a8"); err == nil || drainedError() == nil {
		t.Fatalf("an unknown job user did not drain: %v", err)
	}
}
