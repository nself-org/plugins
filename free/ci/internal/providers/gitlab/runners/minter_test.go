package runners_test

// Purpose: the minter tests: the launcher's view of the registrar (Mint, Deregister, Orphans) and the rule
// that every launched runner is restricted to protected refs.
// Inputs:  fakeGL behind runners.UseRegistrar.
// Outputs: pass or fail.
// Constraints: no network; the process-wide registrar override is reset after each test.

import (
	"context"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/providers/gitlab/runners"
	"github.com/nself-org/plugins/free/ci/internal/providers/runnerjob"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

func useFake(t *testing.T, f *fakeGL, clk func() time.Time) *runners.Minter {
	t.Helper()
	reg := f.registrar(clk)
	runners.UseRegistrar(func(context.Context) (*runners.Registrar, error) { return reg, nil })
	t.Cleanup(func() { runners.UseRegistrar(nil) })
	return &runners.Minter{}
}

// TestMinterRoundTrip: mint, the handle is the runner id, the credential is the glrt- token, a protected
// branch or tag ref registers ref_protected; deregister proves the
// runner absent and orphans lists the old idle ones.
func TestMinterRoundTrip(t *testing.T) {
	ctx := context.Background()
	f := newFakeGL(t)
	f.branch["main"] = true
	f.jobs[1] = fakeJob{Ref: "main", Status: "pending", Tags: []string{"nself"}}
	f.tags["v1"] = true
	f.jobs[2] = fakeJob{Ref: "v1", Tag: true, Status: "pending"}
	f.jobs[3] = fakeJob{Ref: "main", Status: "pending"}
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
	if _, err = m.Mint(ctx, runnerjob.Request{Repo: testProject, Demand: "3", AttemptID: "a3", Arch: "amd64"}); err != nil {
		t.Fatal(err)
	}
	if got := []any{f.mints[0]["access_level"], f.mints[1]["access_level"], f.mints[2]["access_level"]}; got[0] != "ref_protected" || got[1] != "ref_protected" || got[2] != "ref_protected" {
		t.Fatalf("access levels %v", got)
	}
	if tl := f.mints[0]["tag_list"].([]any); len(tl) != 2 || tl[0] != "nself" {
		t.Fatalf("the runner carries its job's tags and its unique one: %v", tl)
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

// TestMinterWithoutAPolicyHasNoToken: with opt-in but no runner token variable the minter answers E700 and
// makes no request; it never reaches for the dispatch provider's token.
func TestMinterWithoutAPolicyHasNoToken(t *testing.T) {
	f := newFakeGL(t)
	runners.UseRegistrar(nil)
	eff, err := merge(t, scope(trust.SourceUser, map[string]string{runners.KeyProtectedRefsOnly: `true`}))
	if err != nil {
		t.Fatal(err)
	}
	runners.UsePolicy(func(context.Context) (*policy.Effective, error) { return &eff, nil })
	t.Cleanup(func() { runners.UsePolicy(nil) })
	t.Setenv("NSELF_CI_GITLAB_TOKEN", devToken)
	_, err = (&runners.Minter{}).Mint(context.Background(), runnerjob.Request{Repo: testProject, Demand: "1", AttemptID: "a1", Arch: "amd64"})
	if code(err) != "E700" || len(f.calls) != 0 {
		t.Fatalf("want E700 and no request on the fake, got %v", err)
	}
}
