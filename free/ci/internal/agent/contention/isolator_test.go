package contention

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/nodes/reservation"
)

type noopIsolator struct{ err error }

func (n noopIsolator) Prepare(context.Context, *exec.JobSpec) error { return n.err }

// fakeLinux is a cgroup v2 tree under a temp dir: the agent's own cgroup is /agent.
func fakeLinux(t *testing.T, delegated bool) Platform {
	t.Helper()
	root := t.TempDir()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.WriteFile(filepath.Join(root, "cgroup.controllers"), []byte("cpu memory"), 0o644))
	must(os.MkdirAll(filepath.Join(root, "agent"), 0o755))
	self := filepath.Join(root, "proc-self-cgroup")
	must(os.WriteFile(self, []byte("0::/agent\n"), 0o644))
	if !delegated {
		must(os.Chmod(filepath.Join(root, "agent"), 0o555)) // a read-only cgroup: nothing can be created
	}
	t.Cleanup(func() { _ = os.Chmod(filepath.Join(root, "agent"), 0o755) })
	return Platform{GOOS: "linux", CgroupRoot: root, SelfCgroup: self, Has: func(p string) bool {
		_, err := os.Stat(p)
		return err == nil || p == "/usr/bin/nice" || p == "/bin/sh"
	}}
}

func prepare(t *testing.T, pl Platform, sh reservation.Share, spec exec.JobSpec) (exec.JobSpec, Priority) {
	t.Helper()
	var got Priority
	i := &Isolator{Inner: noopIsolator{}, Share: func() reservation.Share { return sh }, Platform: &pl, Report: func(_ string, p Priority) { got = p }}
	if err := i.Prepare(context.Background(), &spec); err != nil {
		t.Fatal(err)
	}
	return spec, got
}

func TestLinuxPriorityAndCgroup(t *testing.T) {
	pl := fakeLinux(t, true)
	sh := reservation.Compute(exec.Capacity{CPUs: 8, MemoryMB: 16384}, reservation.Settings{ReservedCPU: 4, ReservedMemMB: 4096}, Need{})
	spec, p := prepare(t, pl, sh, exec.JobSpec{AttemptID: "att-1", Command: []string{"make", "test"}})
	if p.Nice != Applied || p.CPU != Applied || p.Memory != Applied || p.Reason != "" {
		t.Fatalf("priority = %+v", p)
	}
	own := filepath.Join(pl.CgroupRoot, "agent", parentGroup)
	for file, want := range map[string]string{"cpu.max": "400000 100000", "memory.max": "12884901888"} {
		if b, err := os.ReadFile(filepath.Join(own, file)); err != nil || string(b) != want {
			t.Errorf("%s = %q %v, want %q", file, b, err, want)
		}
	}
	if st, err := os.Stat(filepath.Join(own, "j-att-1")); err != nil || !st.IsDir() {
		t.Fatalf("leaf missing: %v", err)
	}
	wantTail := []string{"/usr/bin/nice", "-n", "10", "make", "test"}
	if len(spec.Command) < 10 || spec.Command[0] != "/bin/sh" || !reflect.DeepEqual(spec.Command[len(spec.Command)-5:], wantTail) || spec.Command[3] != filepath.Join(own, "j-att-1") || !strings.HasSuffix(spec.Command[4], "mkdir") {
		t.Fatalf("command = %q", spec.Command)
	}
	pl.ReleaseGroup("att-1")
	if _, err := os.Stat(filepath.Join(own, "j-att-1")); err == nil {
		t.Fatal("leaf survived ReleaseGroup")
	}
	if err := os.Mkdir(filepath.Join(own, "dead"), 0o755); err != nil {
		t.Fatal(err)
	}
	pl.Sweep(0)
	if _, err := os.Stat(filepath.Join(own, "dead")); err == nil {
		t.Fatal("Sweep left an empty group")
	}
}

func TestUnwritableCgroupIsReportedUnknown(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory modes")
	}
	pl := fakeLinux(t, false)
	sh := reservation.Compute(exec.Capacity{CPUs: 8, MemoryMB: 16384}, reservation.Settings{ReservedCPU: 4}, Need{})
	spec, p := prepare(t, pl, sh, exec.JobSpec{AttemptID: "att-1", Command: []string{"true"}})
	if p.CPU != NoInfo || p.Nice != Applied || !strings.Contains(p.Reason, "cgroup") {
		t.Fatalf("priority = %+v", p)
	}
	if !reflect.DeepEqual(spec.Command, []string{"/usr/bin/nice", "-n", "10", "true"}) {
		t.Fatalf("command = %q", spec.Command)
	}
	pl.SelfCgroup = filepath.Join(t.TempDir(), "missing")
	if _, p = prepare(t, pl, sh, exec.JobSpec{AttemptID: "att-1", Command: []string{"true"}}); p.CPU != NoInfo {
		t.Fatalf("no cgroup file: %+v", p)
	}
}

func TestNoReservationMeansNoCgroup(t *testing.T) {
	pl := fakeLinux(t, true)
	spec, p := prepare(t, pl, reservation.Share{CPU: 8, MemMB: 1}, exec.JobSpec{AttemptID: "a", Command: []string{"true"}})
	if p.CPU != Off || p.Memory != Off || p.Nice != Applied || spec.Command[0] != "/usr/bin/nice" {
		t.Fatalf("%+v %q", p, spec.Command)
	}
	if _, err := os.Stat(filepath.Join(pl.CgroupRoot, "agent", parentGroup)); err == nil {
		t.Fatal("a cgroup was created without a reservation")
	}
}

func TestDarwinTaskpolicyPrefix(t *testing.T) {
	pl := Platform{GOOS: "darwin", Has: func(p string) bool { return p == "/usr/sbin/taskpolicy" }}
	spec, p := prepare(t, pl, reservation.Share{}, exec.JobSpec{AttemptID: "a", Command: []string{"make"}})
	if p.Nice != Applied || !reflect.DeepEqual(spec.Command, []string{"/usr/sbin/taskpolicy", "-b", "make"}) {
		t.Fatalf("%+v %q", p, spec.Command)
	}
	pl.Has = func(string) bool { return false }
	spec, p = prepare(t, pl, reservation.Share{CPUCapped: true, CPU: 2}, exec.JobSpec{AttemptID: "a", Command: []string{"make"}})
	if p.Nice != NoInfo || p.CPU != NoInfo || !reflect.DeepEqual(spec.Command, []string{"make"}) {
		t.Fatalf("missing taskpolicy: %+v %q", p, spec.Command)
	}
}

func TestContainerJobsAndInnerErrors(t *testing.T) {
	pl := fakeLinux(t, true)
	spec, p := prepare(t, pl, reservation.Share{}, exec.JobSpec{AttemptID: "a", Job: model.Job{Isolation: "container"}, Command: []string{"make"}})
	if p.Nice != NoInfo || !reflect.DeepEqual(spec.Command, []string{"make"}) {
		t.Fatalf("container job was wrapped: %+v %q", p, spec.Command)
	}
	boom := errors.New("unsupported")
	i := &Isolator{Inner: noopIsolator{err: boom}, Platform: &pl}
	s := exec.JobSpec{Command: []string{"x"}}
	if err := i.Prepare(context.Background(), &s); !errors.Is(err, boom) || len(s.Command) != 1 {
		t.Fatalf("inner error lost: %v %q", err, s.Command)
	}
	if err := (&Isolator{}).Prepare(context.Background(), &s); err == nil {
		t.Fatal("a missing inner isolator was accepted")
	}
}

func TestUnsafeAttemptIDNeverReachesTheFilesystem(t *testing.T) {
	pl := fakeLinux(t, true)
	sh := reservation.Compute(exec.Capacity{CPUs: 8, MemoryMB: 1024}, reservation.Settings{ReservedCPU: 4}, Need{})
	_, p := prepare(t, pl, sh, exec.JobSpec{AttemptID: "../../escape", Command: []string{"true"}})
	if p.CPU != NoInfo {
		t.Fatalf("%+v", p)
	}
	if _, err := os.Stat(filepath.Join(pl.CgroupRoot, "escape")); err == nil {
		t.Fatal("path escaped the group")
	}
}

func TestUncappedResourceIsResetToMax(t *testing.T) {
	pl := fakeLinux(t, true)
	both := reservation.Compute(exec.Capacity{CPUs: 8, MemoryMB: 16384}, reservation.Settings{ReservedCPU: 4, ReservedMemMB: 4096}, Need{})
	prepare(t, pl, both, exec.JobSpec{AttemptID: "a1", Command: []string{"true"}})
	cpuOnly := reservation.Compute(exec.Capacity{CPUs: 8, MemoryMB: 16384}, reservation.Settings{ReservedCPU: 2}, Need{})
	_, p := prepare(t, pl, cpuOnly, exec.JobSpec{AttemptID: "a2", Command: []string{"true"}})
	own := filepath.Join(pl.CgroupRoot, "agent", parentGroup)
	if b, _ := os.ReadFile(filepath.Join(own, "memory.max")); string(b) != "max" {
		t.Fatalf("a dropped memory reservation left memory.max = %q", b)
	}
	if b, _ := os.ReadFile(filepath.Join(own, "cpu.max")); string(b) != "600000 100000" {
		t.Fatalf("cpu.max = %q", b)
	}
	if p.CPU != Applied || p.Memory != Off {
		t.Fatalf("%+v", p)
	}
}

func TestOneLimitFailingIsReportedPerResource(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores file modes")
	}
	pl := fakeLinux(t, true)
	sh := reservation.Compute(exec.Capacity{CPUs: 8, MemoryMB: 16384}, reservation.Settings{ReservedCPU: 4, ReservedMemMB: 4096}, Need{})
	own := filepath.Join(pl.CgroupRoot, "agent", parentGroup)
	if err := os.MkdirAll(own, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(own, "memory.max"), []byte("max"), 0o444); err != nil {
		t.Fatal(err)
	}
	_, p := prepare(t, pl, sh, exec.JobSpec{AttemptID: "a1", Command: []string{"true"}})
	if p.CPU != Applied || p.Memory != NoInfo || !strings.Contains(p.Reason, "memory.max") {
		t.Fatalf("%+v", p)
	}
}

func TestSweepSparesYoungGroups(t *testing.T) {
	pl := fakeLinux(t, true)
	sh := reservation.Compute(exec.Capacity{CPUs: 8, MemoryMB: 16384}, reservation.Settings{ReservedCPU: 4}, Need{})
	prepare(t, pl, sh, exec.JobSpec{AttemptID: "young", Command: []string{"true"}})
	prepare(t, pl, sh, exec.JobSpec{AttemptID: "next", Command: []string{"true"}}) // sweeps with the 2 minute bar
	if _, err := os.Stat(filepath.Join(pl.CgroupRoot, "agent", parentGroup, "j-young")); err != nil {
		t.Fatalf("a group younger than the bar was swept: %v", err)
	}
}

func TestOwnCgroupIgnoresTheAgentsOwnChildGroup(t *testing.T) {
	pl := fakeLinux(t, true)
	if err := os.WriteFile(pl.SelfCgroup, []byte("0::/agent/"+selfGroup+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := pl.ownCgroup()
	if err != nil || got != filepath.Join(pl.CgroupRoot, "agent") {
		t.Fatalf("ownCgroup = %q %v", got, err)
	}
}

func TestLeafNamesCannotCollideWithCgroupFiles(t *testing.T) {
	pl := fakeLinux(t, true)
	sh := reservation.Compute(exec.Capacity{CPUs: 8, MemoryMB: 16384}, reservation.Settings{ReservedCPU: 4}, Need{})
	_, p := prepare(t, pl, sh, exec.JobSpec{AttemptID: "cpu.max", Command: []string{"true"}})
	own := filepath.Join(pl.CgroupRoot, "agent", parentGroup)
	if b, _ := os.ReadFile(filepath.Join(own, "cpu.max")); string(b) != "400000 100000" || p.CPU != Applied {
		t.Fatalf("an attempt named cpu.max clobbered the interface file: %q %+v", b, p)
	}
}

func TestCIEnvironmentSaysWhyTheReservationIsOff(t *testing.T) {
	pl := fakeLinux(t, true)
	sh := reservation.Compute(exec.Capacity{CPUs: 8, MemoryMB: 16384, CI: true}, reservation.Settings{ReservedCPU: 4}, Need{})
	_, p := prepare(t, pl, sh, exec.JobSpec{AttemptID: "a", Command: []string{"true"}})
	if p.CPU != Off || !strings.Contains(p.Reason, "CI=true") {
		t.Fatalf("%+v", p)
	}
}
