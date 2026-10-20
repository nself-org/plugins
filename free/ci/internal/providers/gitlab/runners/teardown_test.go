package runners_test

// Purpose: the teardown acceptance tests: Deregister (including 404 and the absence check) and Orphans.
// Inputs:  fakeGL with runners born at chosen times.
// Outputs: pass or fail.
// Constraints: no live network; descriptions are built by a Registrar under a shifted clock so their
//          embedded birth time is real.

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/providers/gitlab/runners"
)

func ref(id string) providers.RunnerRef { return providers.RunnerRef{ID: id, Scope: repoScope} }

func code(err error) string {
	var pe *providers.Error
	if errors.As(err, &pe) {
		return pe.Code
	}
	return ""
}

func reason(err error) string {
	var pe *providers.Error
	if errors.As(err, &pe) {
		return pe.Reason
	}
	return ""
}

// TestDeregister proves acceptance 1 (delete incl. 404, verify absence): a minted runner is deleted and
// proven absent; deleting it again is nothing to do; a runner that went away between the read and the
// delete (404 on DELETE) is done; a delete that did not take is E707 deregister_unverified; a runner that
// is not ours is never touched.
func TestDeregister(t *testing.T) {
	ctx := context.Background()
	f := newFakeGL(t)
	r := f.registrar(clock)
	m, err := r.MintRunner(ctx, runnerReq(), runners.MintOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err = r.Deregister(ctx, m.Ref()); err != nil || f.count() != 0 {
		t.Fatalf("deregister: %v, left %d", err, f.count())
	}
	if err = r.Deregister(ctx, m.Ref()); err != nil || f.called("DELETE") != 1 {
		t.Fatalf("a second deregister must be a no-op: %v, deletes %d", err, f.called("DELETE"))
	}
	race := f.registrar(clock)
	m2, _ := race.MintRunner(ctx, runnerReq(), runners.MintOptions{})
	f.raceDelete = true
	if err = race.Deregister(ctx, m2.Ref()); err != nil {
		t.Fatalf("a 404 on the delete means gone: %v", err)
	}
	f.raceDelete, f.keepDel = false, true
	m3, _ := race.MintRunner(ctx, runnerReq(), runners.MintOptions{})
	if err = race.Deregister(ctx, m3.Ref()); code(err) != "E707" {
		t.Fatalf("a runner still listed after the delete must be E707, got %v", err)
	}
	foreign := f.add("persistent-runner", false)
	f.keepDel = false
	if err = r.Deregister(ctx, ref(itoa(foreign.ID))); code(err) != "E707" || f.count() == 0 {
		t.Fatalf("a foreign runner must be refused and kept: %v", err)
	}
	for _, bad := range []string{"", "runner-1", "12a", "../1", "1/2", "99999999999999999999"} {
		if err = r.Deregister(ctx, ref(bad)); code(err) != "E702" {
			t.Errorf("id %q: want E702, got %v", bad, err)
		}
	}
	f.statusFor["GET /runners/"+itoa(foreign.ID)] = 500
	if err = r.Deregister(ctx, ref(itoa(foreign.ID))); providers.ClassOf(err) != providers.Transient {
		t.Fatalf("a server error stays transient: %v", err)
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

// TestOrphans proves acceptance 2 (an orphan older than 70 minutes is found): idle nself-glr- runners older
// than the cutoff are listed; a young one, one with a running job, one whose jobs cannot be read, and a
// runner that is not ours are not.
func TestOrphans(t *testing.T) {
	ctx := context.Background()
	f := newFakeGL(t)
	old := f.registrar(func() time.Time { return epoch.Add(-3 * time.Hour) })
	young := f.registrar(func() time.Time { return epoch.Add(-10 * time.Minute) })
	mintAt := func(r *runners.Registrar) runners.Minted {
		m, err := r.MintRunner(ctx, runnerReq(), runners.MintOptions{})
		if err != nil {
			t.Fatal(err)
		}
		return m
	}
	idle, busy, unread, fresh := mintAt(old), mintAt(old), mintAt(old), mintAt(young)
	f.runners[1].Running = true
	f.add("persistent-runner", false)
	f.statusFor["GET /runners/"+unread.RunnerID+"/jobs"] = 500
	reg := f.registrar(clock)
	got, err := reg.Orphans(ctx, repoScope, time.Time{})
	if err != nil || len(got) != 1 || got[0].ID != idle.RunnerID {
		t.Fatalf("orphans %v err %v (idle %s busy %s unread %s fresh %s)", got, err, idle.RunnerID, busy.RunnerID, unread.RunnerID, fresh.RunnerID)
	}
	if all, _ := reg.Orphans(ctx, repoScope, epoch.Add(time.Hour)); len(all) != 2 {
		t.Fatalf("a later cutoff also lists the young idle runner (2), never the busy one, the unread one or the foreign one: %v", all)
	}
	if _, err = reg.Orphans(ctx, providers.RunnerScope{Repo: "bad"}, time.Time{}); err == nil {
		t.Fatal("a bad scope must be refused")
	}
	f.statusFor["GET /projects/acme%2Fwidgets/runners"] = 500
	if _, err = reg.Orphans(ctx, repoScope, time.Time{}); providers.ClassOf(err) != providers.Transient {
		t.Fatalf("a failed list stays transient: %v", err)
	}
}
