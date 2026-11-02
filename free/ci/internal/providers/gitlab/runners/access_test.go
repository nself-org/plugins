package runners_test

import (
	"context"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/providers/gitlab/runners"
)

// TestAccessLevel proves the restriction assessment and that every mint path enforces ref_protected.
func TestAccessLevel(t *testing.T) {
	ctx := context.Background()
	f := newFakeGL(t)
	r := f.registrar(clock)
	f.branch["main"], f.branch["release/1.0"], f.branch["release-*"], f.tags["v1.0"] = true, true, true, true
	f.jobs[1] = fakeJob{Ref: "main", Status: "pending"}
	f.jobs[2] = fakeJob{Ref: "feature/x", Status: "pending"}
	f.jobs[3] = fakeJob{Ref: "v1.0", Tag: true, Status: "pending"}
	f.jobs[4] = fakeJob{Ref: "refs/merge-requests/7/head", Status: "pending"}
	f.jobs[5] = fakeJob{Ref: "v2.0", Tag: true, Status: "pending"}
	f.jobs[6] = fakeJob{Ref: "release/1.0", Status: "pending"} // a protected branch with a slash: the %2F path encoding
	f.jobs[7] = fakeJob{Ref: "release-1", Status: "pending"}   // matched only by the wildcard rule release-*: unproven, not protected
	f.jobs[8] = fakeJob{Ref: "main2", Status: "pending"}
	f.garbage["main2"] = true // the protection read answers 200 with a body that is not that protection
	for job, want := range map[int64]bool{0: false, 1: true, 2: false, 3: true, 4: false, 5: false, 6: true, 7: false, 99: false} {
		got, err := r.Restriction(ctx, repoScope, job)
		if err != nil || got.ProtectedOnly != want {
			t.Errorf("job %d: protected %v err %v, want %v", job, got.ProtectedOnly, err, want)
		}
	}
	if got, err := r.Restriction(ctx, repoScope, 8); got.ProtectedOnly || err == nil {
		t.Errorf("a 200 that is not JSON must never prove protection and surfaces as an error: %+v %v", got, err)
	}
	f.statusFor["GET /projects/acme%2Fwidgets/jobs/1"] = 500
	if _, err := r.Restriction(ctx, repoScope, 1); err == nil || providers.ClassOf(err) != providers.Transient {
		t.Errorf("a failed job read must surface as a transient error, got %v", err)
	}
	for _, opt := range []runners.MintOptions{{}, {Tags: []string{"nself"}}} {
		if _, err := r.MintRunner(ctx, runnerReq(), opt); err != nil {
			t.Fatal(err)
		}
	}
	if f.mints[0]["access_level"] != "ref_protected" || f.mints[1]["access_level"] != "ref_protected" {
		t.Fatalf("access levels %v %v", f.mints[0]["access_level"], f.mints[1]["access_level"])
	}
	if _, err := r.Mint(ctx, runnerReq()); err != nil || f.mints[2]["access_level"] != "ref_protected" {
		t.Fatalf("the interface Mint must register ref_protected: %v %v", err, f.mints[2]["access_level"])
	}
}
