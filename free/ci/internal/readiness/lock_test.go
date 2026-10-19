package readiness

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
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
