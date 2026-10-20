package demand_test

import (
	"context"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/providers/github/demand"
	"github.com/nself-org/plugins/free/ci/internal/providers/github/jit"
	"github.com/nself-org/plugins/free/ci/internal/providers/runnerjob"
)

func at(t time.Time) func() time.Time { return func() time.Time { return t } }

func minterOn(t *testing.T, f *fakeGH, now time.Time) *demand.Minter {
	reg := f.registrar(t, jit.DefaultSettings(), at(now))
	return &demand.Minter{Registrar: func(context.Context) (*jit.Registrar, error) { return reg, nil }}
}

// TestMinterRegistersOnlyTheFixedLabels: the registration carries exactly self-hosted, the operator's
// label, linux, the arch label and its own unique label (never a label from the demand), at repo scope in
// the default runner group, with the 60 minute credential lifetime.
func TestMinterRegistersOnlyTheFixedLabels(t *testing.T) {
	gh := newFakeGH(t)
	m := minterOn(t, gh, epoch)
	for _, tc := range []struct{ arch, label string }{{"amd64", "X64"}, {"arm64", "ARM64"}} {
		reg, err := m.Mint(context.Background(), runnerjob.Request{Repo: testRepo, Demand: "1", AttemptID: "att", Arch: tc.arch})
		if err != nil {
			t.Fatal(err)
		}
		body := gh.minted[len(gh.minted)-1]
		labels := body["labels"].([]any)
		name := body["name"].(string)
		if len(labels) != 5 || labels[0] != "self-hosted" || labels[1] != "nself" || labels[2] != "linux" || labels[3] != tc.label || labels[4] != name {
			t.Fatalf("labels %v name %s", labels, name)
		}
		if !regexp.MustCompile(`^nself-jit-[0-9A-HJKMNP-TV-Z]{26}$`).MatchString(name) || body["runner_group_id"].(float64) != 1 {
			t.Fatalf("name %q group %v", name, body["runner_group_id"])
		}
		if !strings.HasPrefix(reg.Credential, "synthetic-jitconfig-") || !reg.ExpiresAt.Equal(epoch.Add(time.Hour)) || !strings.HasPrefix(reg.Handle, "repo:") {
			t.Fatalf("registration %+v", reg)
		}
	}
	if m.Provider() != "github" {
		t.Fatal("provider")
	}
}

// TestMinterDeregisterProvesAbsenceAndOrphansAreOldAndIdle.
func TestMinterDeregisterProvesAbsenceAndOrphansAreOldAndIdle(t *testing.T) {
	gh := newFakeGH(t)
	old := minterOn(t, gh, epoch.Add(-3*time.Hour))
	now := minterOn(t, gh, epoch)
	ctx := context.Background()
	oldReg, err := old.Mint(ctx, runnerjob.Request{Repo: testRepo, Demand: "1", AttemptID: "a", Arch: "amd64"})
	if err != nil {
		t.Fatal(err)
	}
	freshReg, err := now.Mint(ctx, runnerjob.Request{Repo: testRepo, Demand: "2", AttemptID: "b", Arch: "amd64"})
	if err != nil {
		t.Fatal(err)
	}
	gh.runners = append(gh.runners, fakeRunner{ID: 999, Name: "persistent-runner"}) // not ours: never listed, never deleted
	got, err := now.Orphans(ctx, testRepo, epoch.Add(-70*time.Minute))
	if err != nil || len(got) != 1 || got[0] != oldReg.Handle {
		t.Fatalf("orphans %v err %v (old %s fresh %s)", got, err, oldReg.Handle, freshReg.Handle)
	}
	if err = now.Deregister(ctx, testRepo, oldReg.Handle); err != nil || gh.count() != 2 {
		t.Fatalf("deregister: %v, runners left %d", err, gh.count())
	}
	if err = now.Deregister(ctx, testRepo, oldReg.Handle); err != nil {
		t.Fatalf("an absent runner is success: %v", err)
	}
	if err = now.Deregister(ctx, testRepo, "repo:999"); err == nil {
		t.Fatal("a runner that is not nself-jit-<ulid> must never be deleted")
	}
	gh.keepDel = true
	if err = now.Deregister(ctx, testRepo, freshReg.Handle); err == nil || !strings.Contains(err.Error(), "E707") {
		t.Fatalf("a runner that stays after the DELETE must be E707, got %v", err)
	}
}
