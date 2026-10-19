//go:build integration && !windows

package detach

// Integration proof of Epic D30 with the real plugin binary (P7-CI-63): a
// detached run outlives the process group that started it, `runs cancel` stops
// it, a killed child turns the run lost for `runs wait`, `runs follow` ends
// with the evidence, and `runs prune` removes the stream file.
// Run with: INTEGRATION=1 go test -tags integration -run TestDetachSurvivesParent ./internal/detach/
// A missing prerequisite is a failure, never a skip.

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

type itEnv struct {
	t         *testing.T
	bin, repo string
	home, tmp string
	env       []string
}

func newItEnv(t *testing.T) *itEnv {
	t.Helper()
	if os.Getenv("INTEGRATION") != "1" {
		t.Fatal("INTEGRATION=1 is required for this test")
	}
	tmp := t.TempDir()
	e := &itEnv{t: t, tmp: tmp, bin: filepath.Join(tmp, "nself-ci"), repo: filepath.Join(tmp, "repo"), home: filepath.Join(tmp, "home")}
	build := exec.Command("go", "build", "-mod=vendor", "-o", e.bin, "../../cmd")
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	if err := os.MkdirAll(filepath.Join(e.repo, ".nself"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := "version: 1\njobs:\n  slow:\n    kind: test\n    run: [sh, -c, \"sleep 4; echo done\"]\n    required: true\n"
	if err := os.WriteFile(filepath.Join(e.repo, ".nself", "ci.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, a := range [][]string{{"init", "-q"}, {"add", "-A"}, {"-c", "user.email=a@b.test", "-c", "user.name=t", "commit", "-qm", "init"}} {
		if out, err := exec.Command("git", append([]string{"-C", e.repo}, a...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", a, err, out)
		}
	}
	e.env = append(os.Environ(), "NSELF_V15=1", "NSELF_CLI_VERSION=1.5.0", "NSELF_CI_HOME="+e.home, "NSELF_CI_REPO="+e.repo, "NSELF_MCP_TOKEN=fake-mcp-token-value")
	return e
}

func (e *itEnv) cmd(args ...string) *exec.Cmd {
	c := exec.Command(e.bin, args...)
	c.Env, c.Dir = e.env, e.repo
	return c
}

func (e *itEnv) run(args ...string) (int, string) {
	e.t.Helper()
	out, err := e.cmd(args...).Output()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		e.t.Fatalf("%v: %v", args, err)
	}
	return code, string(out)
}

// detachRun starts a detached run through a parent shell of its own process group
// and returns the handle and the parent's pid (the group leader).
func (e *itEnv) detachRun() (Handle, int) {
	e.t.Helper()
	handlePath := filepath.Join(e.tmp, "handle-"+time.Now().Format("150405.000000")+".json")
	parent := exec.Command("sh", "-c", `"$1" --detach --json --no-status > "$2"; sleep 60`, "sh", e.bin, handlePath)
	parent.Env, parent.Dir = e.env, e.repo
	parent.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	start := time.Now()
	if err := parent.Start(); err != nil {
		e.t.Fatal(err)
	}
	e.t.Cleanup(func() { _ = syscall.Kill(-parent.Process.Pid, syscall.SIGKILL); _ = parent.Wait() })
	var env struct{ Data Handle }
	for time.Since(start) < 5*time.Second {
		if b, err := os.ReadFile(handlePath); err == nil && json.Unmarshal(b, &env) == nil && env.Data.RunID != "" {
			return env.Data, parent.Process.Pid
		}
		time.Sleep(25 * time.Millisecond)
	}
	e.t.Fatal("no run handle within 5 s")
	return Handle{}, 0
}

func alive(pid int) bool { return syscall.Kill(pid, 0) == nil }

func (e *itEnv) waitState(runID string, timeout string) (int, string) {
	e.t.Helper()
	code, out := e.run("runs", "wait", runID, "--timeout", timeout, "--json")
	var w struct{ Data struct{ State string } }
	if err := json.Unmarshal([]byte(out), &w); err != nil {
		e.t.Fatalf("wait output: %v\n%s", err, out)
	}
	return code, w.Data.State
}

func TestDetachSurvivesParent(t *testing.T) {
	e := newItEnv(t)

	t.Run("a SIGKILL to the invoking process group does not stop the run", func(t *testing.T) {
		h, parent := e.detachRun()
		if !alive(h.PID) || h.State != "queued" || h.Schema != Schema {
			t.Fatalf("handle %+v", h)
		}
		if err := syscall.Kill(-parent, syscall.SIGKILL); err != nil {
			t.Fatal(err)
		}
		time.Sleep(600 * time.Millisecond)
		if !alive(h.PID) {
			t.Fatal("the detached child died with the parent's process group")
		}
		if code, state := e.waitState(h.RunID, "60s"); code != 0 || state != "passed" {
			t.Fatalf("wait exit %d state %q", code, state)
		}
		code, out := e.run("runs", "follow", h.RunID, "--from-seq", "0", "--json")
		lines := strings.Split(strings.TrimSpace(out), "\n")
		last, _ := ParseLine([]byte(lines[len(lines)-1]))
		prev := int64(0)
		for _, l := range lines {
			r, ok := ParseLine([]byte(l))
			if !ok || r.Seq <= prev {
				t.Fatalf("seq not strictly increasing at %q (after %d)", l, prev)
			}
			prev = r.Seq
		}
		if code != 0 || last.Type != "result" {
			t.Fatalf("follow exit %d last %+v", code, last)
		}
		if _, err := os.Stat(h.StreamPath); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("runs cancel stops a detached run", func(t *testing.T) {
		h, _ := e.detachRun()
		time.Sleep(time.Second)
		if code, out := e.run("runs", "cancel", h.RunID); code != 0 {
			t.Fatalf("cancel exit %d %s", code, out)
		}
		if code, state := e.waitState(h.RunID, "30s"); code != 0 || state != "cancelled" {
			t.Fatalf("wait exit %d state %q", code, state)
		}
	})

	t.Run("a killed child makes the run lost and prune removes its stream", func(t *testing.T) {
		h, _ := e.detachRun()
		time.Sleep(time.Second)
		if err := syscall.Kill(h.PID, syscall.SIGKILL); err != nil {
			t.Fatal(err)
		}
		if code, state := e.waitState(h.RunID, "30s"); code != 0 || state != "lost" {
			t.Fatalf("wait exit %d state %q", code, state)
		}
		if code, _ := e.run("runs", "follow", h.RunID); code != 2 {
			t.Fatalf("follow of a lost run: exit %d, want 2", code)
		}
		newer, _ := e.detachRun() // the lost run must not be the newest one: prune keeps that
		if code, out := e.run("runs", "prune", "--keep", "1", "--json"); code != 0 {
			t.Fatalf("prune exit %d %s", code, out)
		}
		if _, err := os.Stat(newer.StreamPath); err != nil {
			t.Fatalf("prune removed the stream of a kept run: %v", err)
		}
		_, _ = e.run("runs", "cancel", newer.RunID)
		if _, err := os.Stat(h.StreamPath); !os.IsNotExist(err) {
			t.Fatalf("the stream file of a pruned run survived: %v", err)
		}
	})

}
