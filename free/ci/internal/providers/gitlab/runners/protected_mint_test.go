package runners_test

// Purpose: D-1390 protection boundary: no launcher fallback issues an unrestricted runner token.
// Inputs: demand ids, job/protection API replies and registration options.
// Outputs: protected credentials or refusal before registration.
// Constraints: synthetic credentials and the existing local TLS fake only.

import (
	"context"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/providers/gitlab/runners"
	"github.com/nself-org/plugins/free/ci/internal/providers/runnerjob"
)

func TestMinterRequiresProtectedRef(t *testing.T) {
	for _, tc := range []struct {
		name, demand, ref, failure, wantCode string
		tag, protected                       bool
	}{
		{name: "protected branch", demand: "1", ref: "main", protected: true},
		{name: "protected tag", demand: "1", ref: "v1", tag: true, protected: true},
		{name: "unprotected branch", demand: "1", ref: "topic", wantCode: "E708"},
		{name: "unprotected tag", demand: "1", ref: "v1", tag: true, wantCode: "E708"},
		{name: "merge request", demand: "1", ref: "refs/merge-requests/7/head", wantCode: "E708"},
		{name: "missing job", demand: "99", wantCode: "E708"},
		{name: "job read fails", demand: "1", ref: "main", failure: "jobs/1", wantCode: "E704"},
		{name: "protection read fails", demand: "1", ref: "main", failure: "protected_branches/main", wantCode: "E704"},
		{name: "invalid demand", demand: "oops", wantCode: "E702"},
		{name: "zero demand", demand: "0", wantCode: "E702"},
		{name: "negative demand", demand: "-1", wantCode: "E702"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeGL(t)
			f.jobs[1] = fakeJob{Ref: tc.ref, Tag: tc.tag, Status: "pending", Tags: []string{"nself"}}
			f.branch[tc.ref], f.tags[tc.ref] = tc.protected, tc.protected
			if tc.failure != "" {
				f.statusFor["GET /projects/acme%2Fwidgets/"+tc.failure] = 500
			}
			m := useFake(t, f, clock)
			got, err := m.Mint(context.Background(), runnerjob.Request{Repo: testProject, Demand: tc.demand, AttemptID: "a1", Arch: "amd64"})
			if code(err) != tc.wantCode {
				t.Fatalf("mint error = %v, want %s", err, tc.wantCode)
			}
			if tc.wantCode != "" {
				if got.Handle != "" || got.Credential != "" || f.called("POST /user/runners") != 0 || f.count() != 0 {
					t.Fatal("unproved protection must not create a runner or return a credential")
				}
				return
			}
			if got.Credential == "" || len(f.mints) != 1 || f.mints[0]["access_level"] != runners.AccessRefProtected {
				t.Fatalf("protected demand must receive a protected runner: %v", f.mints)
			}
		})
	}
}

func TestMintCannotIssueUnrestrictedToken(t *testing.T) {
	for _, tc := range []struct {
		name string
		opt  runners.MintOptions
	}{{"zero options", runners.MintOptions{}}, {"explicit tags", runners.MintOptions{Tags: []string{"nself"}}}} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeGL(t)
			got, err := f.registrar(clock).MintRunner(context.Background(), runnerReq(), tc.opt)
			if err != nil || got.AccessLevel != runners.AccessRefProtected || len(f.mints) != 1 || f.mints[0]["access_level"] != runners.AccessRefProtected {
				t.Fatalf("every token must be restricted to protected refs: %v %v", f.mints, err)
			}
		})
	}
}
