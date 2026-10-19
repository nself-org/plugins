package readiness

import (
	"bufio"
	"errors"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestRunLockExclusive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "readiness.lock")
	first, err := AcquireLock(path)
	if err != nil {
		t.Fatal(err)
	}
	second, err := AcquireLock(path)
	var coded *Error
	if second != nil || !errors.As(err, &coded) || coded.Code != "E799" {
		t.Fatalf("a second holder must get E799, got %v %v", second, err)
	}
	if !strings.Contains(err.Error(), "(pid "+strconv.Itoa(os.Getpid())+")") {
		t.Fatalf("the message names the holder: %v", err)
	}
	if err := first.Release(); err != nil {
		t.Fatal(err)
	}
	if err := first.Release(); err != nil {
		t.Fatalf("release twice: %v", err)
	}
	third, err := AcquireLock(path)
	if err != nil {
		t.Fatalf("the lock is free after release: %v", err)
	}
	_ = third.Release()
}

func TestRunLockStaleFileIsTakenOver(t *testing.T) {
	path := filepath.Join(t.TempDir(), "readiness.lock")
	// A holder that died left its pid behind; nothing holds the flock any more.
	if err := os.WriteFile(path, []byte("2147483646\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	l, err := AcquireLock(path)
	if err != nil {
		t.Fatalf("a stale file must be taken over: %v", err)
	}
	defer func() { _ = l.Release() }()
	b, _ := os.ReadFile(path)
	if strings.TrimSpace(string(b)) != strconv.Itoa(os.Getpid()) {
		t.Fatalf("the file must now name this process: %q", b)
	}
	var nilLock *Lock
	if err := nilLock.Release(); err != nil {
		t.Fatal(err)
	}
}

// TestLockHolderProcess is the child of TestRunLockDiesWithItsHolder: it takes the lock,
// says so and waits to be killed.
func TestLockHolderProcess(t *testing.T) {
	args := flag.Args()
	if len(args) != 2 || args[0] != "hold" {
		return
	}
	if _, err := AcquireLock(args[1]); err != nil {
		println("lock failed:", err.Error())
		os.Exit(1)
	}
	println("locked")
	time.Sleep(time.Minute)
}

// The kernel drops a flock when its holder dies, so a SIGKILLed watcher never strands the lock.
func TestRunLockDiesWithItsHolder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "readiness.lock")
	child := exec.Command(os.Args[0], "-test.run=^TestLockHolderProcess$", "--", "hold", path)
	out, err := child.StderrPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = child.Process.Kill(); _ = child.Wait() }()
	sc := bufio.NewScanner(out)
	for sc.Scan() && sc.Text() != "locked" {
	}
	if _, err := AcquireLock(path); err == nil || !strings.Contains(err.Error(), "E799") || !strings.Contains(err.Error(), "(pid "+strconv.Itoa(child.Process.Pid)+")") {
		t.Fatalf("a live holder in another process must block us and be named: %v", err)
	}
	if err := child.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = child.Wait()
	l, err := AcquireLock(path)
	if err != nil {
		t.Fatalf("the lock must be free once its holder was killed: %v", err)
	}
	_ = l.Release()
}

// A lock on an unlinked inode excludes nobody: after the file is removed the next holder
// must lock the file that is now at the path, and a second one must still be refused.
func TestRunLockSurvivesAnUnlinkedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "readiness.lock")
	first, err := AcquireLock(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = first.Release() }()
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	second, err := AcquireLock(path) // new inode at the path: this is a different, valid lock
	if err != nil {
		t.Fatalf("a fresh file at the path is lockable: %v", err)
	}
	defer func() { _ = second.Release() }()
	if third, err := AcquireLock(path); third != nil || err == nil {
		t.Fatalf("the lock on the file now at the path must exclude a third holder: %v", err)
	}
}

func TestRunLockHeldNoticesDeletionAndRelease(t *testing.T) {
	path := filepath.Join(t.TempDir(), "readiness.lock")
	l, err := AcquireLock(path)
	if err != nil {
		t.Fatal(err)
	}
	if !l.Held() {
		t.Fatal("a fresh lock is held")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if l.Held() {
		t.Fatal("a lock whose file was deleted no longer excludes a second watcher: Held must say so")
	}
	_ = l.Release()
	var none *Lock
	if l.Held() || none.Held() {
		t.Fatal("a released or nil lock is not held")
	}
}

func TestRunLockSurvivesGarbageCollection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "readiness.lock")
	func() {
		if _, err := AcquireLock(path); err != nil { // the caller drops the *Lock
			t.Fatal(err)
		}
	}()
	for i := 0; i < 3; i++ {
		runtime.GC()
	}
	if second, err := AcquireLock(path); err == nil {
		_ = second.Release()
		t.Fatal("a dropped *Lock must keep holding the flock: the os.File finalizer released it")
	}
}
