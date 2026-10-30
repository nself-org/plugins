package gvisor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Fixture lines are copied from a boot log of a fork bomb on the GitHub ubuntu runner (gVisor 20261012.0).
const (
	mountLine  = `I1030 04:29:08.327981       1 vfs.go:1065] Mounted "/work/t05-fork0" to "/repo" type: bind, internal-options: "trans=fd"`
	stubLine   = `handlePtraceSyscallRequest failed: unable to create a stub process: creating stub process: resource temporarily unavailable`
	panicLine  = `panic: Unknown syscall 57 error: createStub: failed to get clone`
	sysmsgLine = `panic: Unknown syscall 57 error: createSysmsgThread: failed to get clone`
	exitedLine = `I1030 03:52:59.197388       1 boot.go:814] application exiting with exit status 2`
)

func writeLog(t *testing.T, dir, name string, lines ...string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSandboxDeath(t *testing.T) {
	dir := t.TempDir() + "/"
	began := time.Now().Add(-time.Minute)
	writeLog(t, dir, "runsc.log.20261030-000001.000001.boot.txt", mountLine, stubLine, panicLine)
	writeLog(t, dir, "runsc.log.20261030-000004.000001.boot.txt", `I1 vfs.go] Mounted "/work/t05-fork1" to "/repo" type: bind`, sysmsgLine) // the other thread-creation variant
	writeLog(t, dir, "runsc.log.20261030-000002.000001.boot.txt", `I1 vfs.go] Mounted "/work/t05-exit2" to "/repo" type: bind`, exitedLine) // a script that ran `exit 2`
	writeLog(t, dir, "runsc.log.20261030-000003.000001.create.txt", mountLine, stubLine)                                                    // not a boot log
	cases := []struct {
		name, log, workspace string
		began                time.Time
		want                 string // a substring of the answer; "" means no claim
	}{
		{"fork bomb: stub process could not be created", dir, "/work/t05-fork0", began, "creating stub process: resource temporarily unavailable"},
		{"fork bomb: sysmsg thread could not be created", dir, "/work/t05-fork1", began, "createSysmsgThread: failed to get clone"},
		{"exit 2 script: no death line in its log", dir, "/work/t05-exit2", began, ""},
		{"another attempt's workspace", dir, "/work/t05-other", began, ""},
		{"log older than the attempt", dir, "/work/t05-fork0", time.Now().Add(time.Hour), ""},
		{"no debug log configured", "", "/work/t05-fork0", began, ""},
		{"log directory missing", dir + "absent/", "/work/t05-fork0", began, ""},
		{"no workspace", dir, "", began, ""},
	}
	for _, c := range cases {
		got := sandboxDeath(c.log, c.workspace, c.began)
		if (c.want == "") != (got == "") || !strings.Contains(got, c.want) {
			t.Errorf("%s: got %q, want it to contain %q", c.name, got, c.want)
		}
	}
	if got := sandboxDeath(dir, "/work/t05-fork0", began); !strings.Contains(got, ".boot.txt") {
		t.Errorf("the answer must cite the log file: %q", got)
	}
}

func TestDebugLogOf(t *testing.T) {
	for args, want := range map[string]string{"": "", "--platform=ptrace": "", "--debug-log=/var/log/runsc/": "/var/log/runsc/", "--debug-log /x/ --strace": "/x/"} {
		if got := debugLogOf(strings.Fields(args)); got != want {
			t.Errorf("%q: got %q, want %q", args, got, want)
		}
	}
}
