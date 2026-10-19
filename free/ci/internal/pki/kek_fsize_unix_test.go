//go:build unix

package pki

// Purpose: the write-failure path of newKEK (disk full, file size limit): no KEK is returned and no
//          partial file is left behind.
// Constraints: the failure is real, not mocked: a child process runs with RLIMIT_FSIZE below the KEK size.

import (
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"testing"
)

func TestLoadKEKWriteFailure(t *testing.T) {
	if dir := os.Getenv("PKI_KEK_FSIZE_DIR"); dir != "" { // child
		signal.Ignore(syscall.SIGXFSZ)
		var lim syscall.Rlimit
		if err := syscall.Getrlimit(syscall.RLIMIT_FSIZE, &lim); err != nil {
			t.Fatal(err)
		}
		orig := lim
		lim.Cur = 16
		if err := syscall.Setrlimit(syscall.RLIMIT_FSIZE, &lim); err != nil {
			t.Fatal(err)
		}
		// restore before the test binary writes its coverage data at exit
		defer func() { _ = syscall.Setrlimit(syscall.RLIMIT_FSIZE, &orig) }()
		key, err := LoadKEK(dir, true)
		if err == nil || key != nil {
			t.Fatalf("a failed write returned a key: %d bytes, err=%v", len(key), err)
		}
		if _, serr := os.Stat(KEKPath(dir)); !os.IsNotExist(serr) {
			t.Fatalf("a partial KEK file was left behind: %v", serr)
		}
		return
	}
	dir := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=^TestLoadKEKWriteFailure$", "-test.count=1")
	cmd.Env = append(os.Environ(), "PKI_KEK_FSIZE_DIR="+dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("child: %v\n%s", err, out)
	}
	if _, err := os.Stat(KEKPath(dir)); !os.IsNotExist(err) {
		t.Fatalf("partial KEK left behind: %v", err)
	}
}
