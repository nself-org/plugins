//go:build linux

package hygiene

import (
	"strings"
	"syscall"
	"testing"
)

func TestKillProcsSkipsAReusedPID(t *testing.T) {
	_, _, child := daemonise(t, "reuse1")
	procs, err := scanProcs("reuse1", 0)
	if err != nil || len(procs) != 1 {
		t.Fatalf("scan: %v %+v", err, procs)
	}
	stale := procs[0]
	stale.Start = "1" // the PID now belongs to a different process than the one scanned
	if err := killProcs([]procEntry{stale}); err != nil {
		t.Fatal(err)
	}
	if syscall.Kill(child.Process.Pid, 0) != nil {
		t.Fatal("a process whose start time differs from the scan was killed")
	}
	if err := killProcs(procs); err != nil {
		t.Fatal(err)
	}
	if alive(child) {
		t.Fatal("the scanned process survived")
	}
}

func TestMountsUnderFindsMountPointsAndDecodesEscapes(t *testing.T) {
	if got, ok := mountsUnder("/proc"); !ok || len(got) == 0 {
		t.Fatalf("/proc is a mount point and must be reported: %v %v", got, ok)
	}
	if got, ok := mountsUnder("/nonexistent-root-for-hygiene"); !ok || len(got) != 0 {
		t.Fatalf("unexpected mounts %v %v", got, ok)
	}
	if got := unescapeMount(`/mnt/a\040b`); got != "/mnt/a b" || !strings.Contains(unescapeMount(`x\134y`), `\`) {
		t.Fatalf("decoded %q", got)
	}
}
