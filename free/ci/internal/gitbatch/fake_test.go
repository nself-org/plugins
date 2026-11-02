//go:build !windows

package gitbatch

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

const fakeOID = "1111111111111111111111111111111111111111"

// fakeGit puts a `git` on PATH that records its pid, arguments and environment, then answers every request
// with the shell snippet reply (which may print anything, to test the protocol checks).
func fakeGit(t *testing.T, reply string) (pids, record string) {
	t.Helper()
	bin := t.TempDir()
	pids, record = filepath.Join(bin, "pids"), filepath.Join(bin, "record")
	script := fmt.Sprintf("#!/bin/sh\necho $$ >> %s\necho \"$@\" > %s\nenv >> %s\nn=0\nwhile read -r line; do\n n=$((n+1))\n%s\ndone\n", pids, record, record, reply)
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return pids, record
}

func lastPID(t *testing.T, pids string) int {
	t.Helper()
	b, err := os.ReadFile(pids)
	if err != nil {
		t.Fatal(err)
	}
	f := strings.Fields(string(b))
	pid, err := strconv.Atoi(f[len(f)-1])
	if err != nil {
		t.Fatal(err)
	}
	return pid
}

func waitGone(t *testing.T, pid int, what string) {
	t.Helper()
	for end := time.Now().Add(5 * time.Second); time.Now().Before(end); time.Sleep(20 * time.Millisecond) {
		if errors.Is(syscall.Kill(pid, 0), syscall.ESRCH) {
			return
		}
	}
	t.Fatalf("%s: process %d is still running", what, pid)
}

const answerInfo = ` echo "` + fakeOID + ` commit 5"`

func TestMalformedReplyFailsClosedAndPoisons(t *testing.T) {
	pids, _ := fakeGit(t, ` if [ $n -eq 1 ]; then echo "`+fakeOID+` commit 5"; else echo "this is not a reply"; fi`)
	s := newTestSession(t, t.TempDir(), Options{})
	ctx := context.Background()
	if obj, err := s.Stat(ctx, "HEAD"); err != nil || obj.OID != fakeOID {
		t.Fatalf("first reply: %+v %v", obj, err)
	}
	if _, err := s.Stat(ctx, "HEAD"); !errors.Is(err, ErrMalformed) {
		t.Fatalf("a malformed reply after a good one must fail the read: %v", err)
	}
	waitGone(t, lastPID(t, pids), "after a malformed reply")
	if _, err := s.Stat(ctx, "HEAD"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("a poisoned session answers nothing: %v", err)
	}
}

func TestBadHeadersFailClosed(t *testing.T) {
	for name, reply := range map[string]string{
		"bad oid":        ` if [ $n -eq 1 ]; then echo "` + fakeOID + ` commit 5"; else echo "ZZZZ commit 5"; fi`,
		"unknown type":   ` if [ $n -eq 1 ]; then echo "` + fakeOID + ` commit 5"; else echo "` + fakeOID + ` banana 5"; fi`,
		"negative size":  ` if [ $n -eq 1 ]; then echo "` + fakeOID + ` commit 5"; else echo "` + fakeOID + ` blob -3"; fi`,
		"missing body":   ` if [ $n -eq 1 ]; then echo "` + fakeOID + ` commit 5"; else echo "` + fakeOID + ` blob 4"; echo "ab"; fi`,
		"unclosed body":  ` if [ $n -eq 1 ]; then echo "` + fakeOID + ` commit 5"; else printf "` + fakeOID + ` blob 2\nabX"; fi`,
		"extra fields":   ` if [ $n -eq 1 ]; then echo "` + fakeOID + ` commit 5"; else echo "` + fakeOID + ` commit 5 6"; fi`,
		"process exited": ` if [ $n -eq 1 ]; then echo "` + fakeOID + ` commit 5"; else exit 3; fi`,
	} {
		t.Run(name, func(t *testing.T) {
			fakeGit(t, reply)
			s := newTestSession(t, t.TempDir(), Options{ReadTimeout: 2 * time.Second})
			ctx := context.Background()
			if _, err := s.Stat(ctx, "HEAD"); err != nil {
				t.Fatalf("first: %v", err)
			}
			verb := s.Stat
			if name == "missing body" || name == "unclosed body" {
				verb = func(ctx context.Context, n string) (Object, error) { o, _, err := s.Read(ctx, n); return o, err }
			}
			if _, err := verb(ctx, "HEAD"); err == nil || errors.Is(err, ErrNotFound) {
				t.Fatalf("%s must be an error, got %v", name, err)
			}
		})
	}
}

func TestMalformedBeforeAnyReplyIsLeftToGit(t *testing.T) {
	fakeGit(t, ` echo "usage: git cat-file (-t | -s)"`)
	s := newTestSession(t, t.TempDir(), Options{})
	if _, err := s.Stat(context.Background(), "HEAD"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("a git without --batch-command is ErrUnavailable: %v", err)
	}
}

func TestTimeoutKillsTheProcess(t *testing.T) {
	pids, _ := fakeGit(t, ` sleep 30`)
	s := newTestSession(t, t.TempDir(), Options{ReadTimeout: 300 * time.Millisecond})
	start := time.Now()
	if _, err := s.Stat(context.Background(), "HEAD"); !errors.Is(err, ErrTimeout) {
		t.Fatalf("want ErrTimeout, got %v", err)
	}
	if time.Since(start) > 4*time.Second {
		t.Fatalf("the per-read timeout did not bound the read: %v", time.Since(start))
	}
	waitGone(t, lastPID(t, pids), "after a timeout")
	if _, err := s.Stat(context.Background(), "HEAD"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("a timed-out session is not reused: %v", err)
	}
}

func TestCancelAndCloseEndTheProcess(t *testing.T) {
	pids, _ := fakeGit(t, answerInfo)
	parent, cancel := context.WithCancel(context.Background())
	s := NewSession(parent, t.TempDir(), Options{})
	if _, err := s.Stat(context.Background(), "HEAD"); err != nil {
		t.Fatal(err)
	}
	pid := lastPID(t, pids)
	cancel()
	waitGone(t, pid, "after the run context was cancelled")
	if _, err := s.Stat(context.Background(), "HEAD"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("a read after cancel is left to git: %v", err)
	}
	s.Close()

	s2 := newTestSession(t, t.TempDir(), Options{})
	if _, err := s2.Stat(context.Background(), "HEAD"); err != nil {
		t.Fatal(err)
	}
	pid2 := lastPID(t, pids)
	s2.Close()
	waitGone(t, pid2, "after Close")
	s2.Close() // twice is fine
	if _, err := s2.Stat(context.Background(), "HEAD"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("a closed session: %v", err)
	}
}

func TestCancelledReadKillsAndReturnsTheContextError(t *testing.T) {
	pids, _ := fakeGit(t, ` sleep 30`)
	s := newTestSession(t, t.TempDir(), Options{ReadTimeout: time.Minute})
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	if _, err := s.Stat(ctx, "HEAD"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("want the context error, got %v", err)
	}
	waitGone(t, lastPID(t, pids), "after a cancelled read")
}

func TestIdleProcessExitsAndIsRestarted(t *testing.T) {
	pids, _ := fakeGit(t, answerInfo)
	s := newTestSession(t, t.TempDir(), Options{Idle: 200 * time.Millisecond})
	if _, err := s.Stat(context.Background(), "HEAD"); err != nil {
		t.Fatal(err)
	}
	first := lastPID(t, pids)
	waitGone(t, first, "after the idle period")
	if _, err := s.Stat(context.Background(), "HEAD"); err != nil {
		t.Fatalf("read after the idle exit: %v", err)
	}
	if second := lastPID(t, pids); second == first {
		t.Fatal("the next read must run a new process")
	}
}

func TestProcessRunsUnderTheFunnelGuarantees(t *testing.T) {
	_, record := fakeGit(t, answerInfo)
	t.Setenv("GIT_DIR", "/nonexistent/elsewhere")
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_OBJECT_DIRECTORY", "/nonexistent/objects")
	t.Setenv("SECRET_TOKEN", "must-not-leak")
	root := t.TempDir()
	s := newTestSession(t, root, Options{})
	if _, err := s.Stat(context.Background(), "HEAD"); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(record)
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	lines := strings.Split(text, "\n")
	if want := "--no-replace-objects -c core.fsmonitor=false -C " + root + " cat-file --batch-command"; lines[0] != want {
		t.Fatalf("argv = %q, want %q", lines[0], want)
	}
	for _, bad := range []string{"GIT_DIR=", "GIT_CONFIG_COUNT=", "GIT_OBJECT_DIRECTORY=", "SECRET_TOKEN="} {
		if strings.Contains(text, "\n"+bad) {
			t.Fatalf("%s reached git: the environment allowlist was bypassed", bad)
		}
	}
	for _, want := range []string{"\nGIT_TERMINAL_PROMPT=0\n", "\nGIT_NO_LAZY_FETCH=1\n"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in the git environment", strings.TrimSpace(want))
		}
	}
}

func TestSetClosesEverySession(t *testing.T) {
	pids, _ := fakeGit(t, answerInfo)
	set := NewSet(context.Background(), Options{})
	ctx := WithSet(context.Background(), set)
	if !Has(ctx) || Has(context.Background()) {
		t.Fatal("Has")
	}
	if For(ctx, "") != nil || For(context.Background(), "/x") != nil {
		t.Fatal("no root or no set means no session")
	}
	root := t.TempDir()
	if For(ctx, root) != For(ctx, root+"/") {
		t.Fatal("one session per cleaned root")
	}
	if _, err := For(ctx, root).Stat(ctx, "HEAD"); err != nil {
		t.Fatal(err)
	}
	pid := lastPID(t, pids)
	set.Close()
	waitGone(t, pid, "after the set closed")
	if For(ctx, root) != nil {
		t.Fatal("a closed set hands out no session")
	}
}
