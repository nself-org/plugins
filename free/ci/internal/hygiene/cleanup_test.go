package hygiene

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/exec"
)

func seedRoot(t *testing.T, h *Host, attempt string) string {
	t.Helper()
	root, err := h.JobRoot(attempt)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "scratch"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestCleanupRemovesOnlyLabelledResourcesAndRoot(t *testing.T) {
	h, rt := testHost(t)
	root := seedRoot(t, h, "att1")
	other := seedRoot(t, h, "att2")
	rt.add(exec.KindContainer, "c1", "att1")
	rt.add(exec.KindNetwork, "n1", "att1")
	rt.add(exec.KindVolume, "v1", "att1")
	rt.add(exec.KindContainer, "c2", "att2")
	rt.add(exec.KindContainer, "runner-service", "") // unlabelled: a runner service
	if err := h.Cleanup(context.Background(), "att1"); err != nil {
		t.Fatal(err)
	}
	for kind, id := range map[exec.ResourceKind]string{exec.KindContainer: "c1", exec.KindNetwork: "n1", exec.KindVolume: "v1"} {
		if rt.has(kind, id) {
			t.Errorf("%s %s survived", kind, id)
		}
	}
	if !rt.has(exec.KindContainer, "c2") || !rt.has(exec.KindContainer, "runner-service") {
		t.Fatal("cleanup touched another attempt's or an unlabelled resource")
	}
	if _, err := os.Lstat(root); !os.IsNotExist(err) {
		t.Fatal("root survived")
	}
	if _, err := os.Lstat(other); err != nil {
		t.Fatal("another attempt's root was removed")
	}
	if err := h.Cleanup(context.Background(), "att1"); err != nil {
		t.Fatalf("cleanup is not idempotent: %v", err)
	}
}

func TestCleanupReportsWhatItCannotRemove(t *testing.T) {
	h, rt := testHost(t)
	seedRoot(t, h, "att1")
	rt.add(exec.KindVolume, "stuck", "att1")
	rt.failRm = "stuck"
	err := h.Cleanup(context.Background(), "att1")
	if codeOf(err) != CodeLeak || !strings.Contains(err.Error(), "stuck") {
		t.Fatalf("want E681 naming the volume, got %v", err)
	}
	if _, serr := os.Lstat(filepath.Join(h.Base, "att1")); !os.IsNotExist(serr) {
		t.Fatal("the root must still be removed when only a resource fails")
	}
	rt.failLs = true
	if err := h.Cleanup(context.Background(), "att1"); codeOf(err) != CodeLeak {
		t.Fatalf("a listing failure must not read as clean: %v", err)
	}
}

func TestCleanupRefusesBadAttemptAndDryRunKeepsEverything(t *testing.T) {
	h, rt := testHost(t)
	root := seedRoot(t, h, "att1")
	rt.add(exec.KindContainer, "c1", "att1")
	for _, id := range []string{"", "..", "a/b"} {
		if err := h.Cleanup(context.Background(), id); codeOf(err) != CodeLeak {
			t.Errorf("attempt %q: %v", id, err)
		}
	}
	h.DryRun = true
	var lines []string
	h.Logf = func(f string, a ...any) { lines = append(lines, f) }
	if err := h.Cleanup(context.Background(), "att1"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(root); err != nil || !rt.has(exec.KindContainer, "c1") || len(lines) == 0 {
		t.Fatal("dry run must report and remove nothing")
	}
}

func TestSweepOrphansSparesLiveAndYoungAttempts(t *testing.T) {
	h, rt := testHost(t)
	live := seedRoot(t, h, "live1")
	orphan := seedRoot(t, h, "orphan1")
	young := seedRoot(t, h, "young1")
	old := time.Now().Add(-48 * time.Hour)
	for _, p := range []string{live, orphan} {
		if err := os.Chtimes(p, old, old); err != nil {
			t.Fatal(err)
		}
	}
	h.MinAge = time.Hour
	rt.add(exec.KindContainer, "c-live", "live1")
	rt.add(exec.KindContainer, "c-orphan", "orphan1")
	rt.add(exec.KindNetwork, "n-gone", "rootless9") // labelled, no root here: another home's unless OwnsRuntime
	rt.add(exec.KindContainer, "c-young", "young1")
	rt.add(exec.KindContainer, "svc", "")
	rep, err := h.SweepOrphans(context.Background(), []string{"live1"})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(rep.Removed, ","); got != "orphan1" {
		t.Fatalf("removed %q: a resource whose attempt has no root here belongs to someone else", got)
	}
	if !rt.has(exec.KindNetwork, "n-gone") {
		t.Fatal("the sweep removed a labelled resource of an attempt it does not own")
	}
	h.OwnsRuntime = true
	if rep, err = h.SweepOrphans(context.Background(), []string{"live1"}); err != nil || strings.Join(rep.Removed, ",") != "rootless9" {
		t.Fatalf("a host that owns the daemon removes it: %v %v", rep.Removed, err)
	}
	for _, p := range []string{live, young} {
		if _, err := os.Lstat(p); err != nil {
			t.Fatalf("%s was removed", p)
		}
	}
	if _, err := os.Lstat(orphan); !os.IsNotExist(err) {
		t.Fatal("orphan root survived")
	}
	for _, id := range []string{"c-live", "c-young", "svc"} {
		if !rt.has(exec.KindContainer, id) {
			t.Errorf("%s was removed", id)
		}
	}
	if rt.has(exec.KindContainer, "c-orphan") || rt.has(exec.KindNetwork, "n-gone") { // after OwnsRuntime
		t.Fatal("orphan resources survived")
	}
}

func TestSweepOrphansReportsFailures(t *testing.T) {
	h, rt := testHost(t)
	seedRoot(t, h, "orphan1")
	rt.add(exec.KindVolume, "stuck", "orphan1")
	rt.failRm = "stuck"
	rep, err := h.SweepOrphans(context.Background(), nil)
	if codeOf(err) != CodeLeak || rep.Failed["orphan1"] == nil {
		t.Fatalf("want E681 and a failure for orphan1, got %v %v", err, rep.Failed)
	}
}

func TestLeakCheckReportsLeftovers(t *testing.T) {
	h, rt := testHost(t)
	seedRoot(t, h, "att1")
	rt.add(exec.KindContainer, "c1", "att1")
	rep, err := h.LeakCheck(context.Background(), "att1", 0)
	if codeOf(err) != CodeLeak || rep.Clean() {
		t.Fatalf("want E681 with leaks, got %v %+v", err, rep)
	}
	kinds := map[LeakKind]bool{}
	for _, l := range rep.Leaks {
		kinds[l.Kind] = true
	}
	if !kinds[LeakRoot] || !kinds[LeakContainer] {
		t.Fatalf("leaks %+v", rep.Leaks)
	}
	if err := h.Cleanup(context.Background(), "att1"); err != nil {
		t.Fatal(err)
	}
	if rep, err := h.LeakCheck(context.Background(), "att1", 0); err != nil || !rep.Clean() {
		t.Fatalf("after cleanup: %v %+v", err, rep)
	}
	rt.failLs = true
	rep, err = h.LeakCheck(context.Background(), "att1", 0)
	if codeOf(err) != CodeLeak || len(rep.Leaks) == 0 || rep.Leaks[0].Kind != LeakUnverified && !hasKind(rep, LeakUnverified) {
		t.Fatalf("an unverifiable check must fail closed: %v %+v", err, rep)
	}
}

func hasKind(r Report, k LeakKind) bool {
	for _, l := range r.Leaks {
		if l.Kind == k {
			return true
		}
	}
	return false
}

func TestLeakCheckRuntimeDownIsSkippedOrALeakByHost(t *testing.T) {
	h, rt := testHost(t)
	rt.down = true
	rep, err := h.LeakCheck(context.Background(), "att1", 0)
	if err != nil || !rep.Clean() || len(rep.Skipped) == 0 {
		t.Fatalf("a silent runtime is reported as skipped on a host that does not need one: %v %+v", err, rep)
	}
	h.RuntimeRequired = true
	rep, err = h.LeakCheck(context.Background(), "att1", 0)
	if codeOf(err) != CodeLeak || !hasKind(rep, LeakUnverified) {
		t.Fatalf("a required runtime that is silent must fail closed: %v %+v", err, rep)
	}
}

type fakeJobs struct {
	killed               []int
	removed              []string
	failKill, failRemove error
}

func (f *fakeJobs) KillAll(_ context.Context, uid int) error {
	f.killed = append(f.killed, uid)
	return f.failKill
}
func (f *fakeJobs) EmptyRoot(_ context.Context, uid int, base, attempt string) error {
	f.removed = append(f.removed, base+"/"+attempt)
	return f.failRemove
}

func TestCleanupJobRunsAsTheJobUserForSeparatedAttempts(t *testing.T) {
	h, _ := testHost(t)
	jobs := &fakeJobs{}
	h.Jobs = jobs
	seedRoot(t, h, "att1")
	uid := os.Geteuid() + 7000 // a job user that is not the caller
	if err := h.CleanupJob(context.Background(), "att1", uid); err != nil {
		t.Fatal(err)
	}
	if len(jobs.killed) != 1 || jobs.killed[0] != uid || len(jobs.removed) != 1 {
		t.Fatalf("kill and removal must run as the job user: %+v", jobs)
	}
	// the caller's own uid never goes through the job runner
	jobs2 := &fakeJobs{}
	h.Jobs = jobs2
	seedRoot(t, h, "att2")
	if err := h.CleanupJob(context.Background(), "att2", os.Geteuid()); err != nil || len(jobs2.killed) != 0 {
		t.Fatalf("own uid: %v %+v", err, jobs2)
	}
	// a refused kill is a failure, never a clean cleanup
	jobs3 := &fakeJobs{failKill: errors.New("sudo: a password is required")}
	h.Jobs = jobs3
	seedRoot(t, h, "att3")
	if err := h.CleanupJob(context.Background(), "att3", uid); codeOf(err) != CodeLeak || !strings.Contains(err.Error(), "password") {
		t.Fatalf("want E681 naming the refusal, got %v", err)
	}
}
