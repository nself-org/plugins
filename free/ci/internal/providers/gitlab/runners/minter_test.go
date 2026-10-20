package runners_test

// Purpose: the minter tests: the launcher's view of the registrar (Mint, Deregister, Orphans) and the rule
// that a failed protection proof registers not_protected instead of failing the job.
// Inputs:  fakeGL behind runners.UseRegistrar.
// Outputs: pass or fail.
// Constraints: no network; the process-wide registrar override is reset after each test.

import (
	"context"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/providers/gitlab/runners"
	"github.com/nself-org/plugins/free/ci/internal/providers/runnerjob"
)

func useFake(t *testing.T, f *fakeGL, clk func() time.Time) *runners.Minter {
	t.Helper()
	reg := f.registrar(clk)
	runners.UseRegistrar(func(context.Context) (*runners.Registrar, error) { return reg, nil })
	t.Cleanup(func() { runners.UseRegistrar(nil) })
	return &runners.Minter{}
}

// TestMinterRoundTrip: mint, the handle is the runner id, the credential is the glrt- token, a protected
// job ref registers ref_protected, an unprotected or unreadable one not_protected; deregister proves the
// runner absent and orphans lists the old idle ones.
func TestMinterRoundTrip(t *testing.T) {
	ctx := context.Background()
	f := newFakeGL(t)
	f.branch["main"] = true
	f.jobs[1] = fakeJob{Ref: "main", Status: "pending"}
	f.jobs[2] = fakeJob{Ref: "topic", Status: "pending"}
	m := useFake(t, f, clock)
	if m.Provider() != "gitlab" {
		t.Fatalf("provider %q", m.Provider())
	}
	reg1, err := m.Mint(ctx, runnerjob.Request{Repo: testProject, Demand: "1", AttemptID: "a1", Arch: "amd64"})
	if err != nil || reg1.Credential != "glrt-synthetic-101" || reg1.Handle != "101" || reg1.ExpiresAt.Sub(epoch) != runners.CredentialLifetime {
		t.Fatalf("mint: %+v %v", reg1, err)
	}
	if _, err = m.Mint(ctx, runnerjob.Request{Repo: testProject, Demand: "2", AttemptID: "a2", Arch: "amd64"}); err != nil {
		t.Fatal(err)
	}
	f.statusFor["GET /projects/acme%2Fwidgets/jobs/3"] = 500
	if _, err = m.Mint(ctx, runnerjob.Request{Repo: testProject, Demand: "3", AttemptID: "a3", Arch: "amd64"}); err != nil {
		t.Fatalf("a failed proof must not fail the mint: %v", err)
	}
	if got := []any{f.mints[0]["access_level"], f.mints[1]["access_level"], f.mints[2]["access_level"]}; got[0] != "ref_protected" || got[1] != "not_protected" || got[2] != "not_protected" {
		t.Fatalf("access levels %v", got)
	}
	if err = m.Deregister(ctx, testProject, reg1.Handle); err != nil || f.count() != 2 {
		t.Fatalf("deregister: %v, left %d", err, f.count())
	}
	oldReg := f.registrar(func() time.Time { return epoch.Add(-3 * time.Hour) })
	o, _ := oldReg.MintRunner(ctx, runnerReq(), runners.MintOptions{})
	handles, err := m.Orphans(ctx, testProject, time.Time{})
	if err != nil || len(handles) != 1 || handles[0] != o.RunnerID {
		t.Fatalf("orphans %v %v", handles, err)
	}
}

// TestMinterWithoutAPolicyHasNoToken: with no runner token variable the default minter answers E700 and
// makes no request; it never reaches for the dispatch provider's token.
func TestMinterWithoutAPolicyHasNoToken(t *testing.T) {
	f := newFakeGL(t)
	runners.UseRegistrar(nil)
	t.Setenv("NSELF_CI_GITLAB_TOKEN", devToken)
	_, err := (&runners.Minter{}).Mint(context.Background(), runnerjob.Request{Repo: testProject, Demand: "1", AttemptID: "a1", Arch: "amd64"})
	if code(err) != "E700" || len(f.calls) != 0 {
		t.Fatalf("want E700 and no request on the fake, got %v", err)
	}
}
