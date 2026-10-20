package runnerjob_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/providers/runnerjob"
	"github.com/nself-org/plugins/free/ci/internal/transport"
)

func code(err error) string {
	if pe, ok := err.(*providers.Error); ok {
		return pe.Code + " " + pe.Reason
	}
	return ""
}

// TestPreparerMintsAfterAdmissionAndSetsTheLeaseCredential: delivery of a runner job mints exactly one
// credential, sets lease.runner_credential (kind, value, expiry within 60 minutes), and records the
// registration (never the credential) in the ledger.
func TestPreparerMintsAfterAdmissionAndSetsTheLeaseCredential(t *testing.T) {
	use(t)
	l, err := deliver(goodNode(), "attempt-1", runnerJob(t, "11"))
	if err != nil {
		t.Fatal(err)
	}
	rc := l.RunnerCredential
	if rc == nil || rc.Kind != testKind || rc.Value != forge.minted[0] || len(forge.minted) != 1 {
		t.Fatalf("lease credential %+v minted %v", rc, forge.minted)
	}
	if rc.ExpiresAt != "2026-01-01T13:00:00Z" {
		t.Fatalf("expiry %q: the registration lives 60 minutes", rc.ExpiresAt)
	}
	row, ok, err := ledgerRunner("attempt-1")
	if err != nil || !ok || row.Handle != "repo:101" || row.Demand != "11" || row.Repo != testRepo {
		t.Fatalf("ledger row %+v ok=%v err=%v", row, ok, err)
	}
}

// TestPreparerLeavesOtherJobsAlone: a job with no runner credential kind passes through untouched, with
// no minter call and no ledger access (every normal lease goes through the preparer).
func TestPreparerLeavesOtherJobsAlone(t *testing.T) {
	use(t)
	l, err := deliver(goodNode(), "attempt-2", model.Job{Kind: "static", Run: []string{"true"}})
	if err != nil || l.RunnerCredential != nil || len(forge.minted) != 0 {
		t.Fatalf("lease %+v err %v minted %v", l.RunnerCredential, err, forge.minted)
	}
}

// TestPreparerRefusesWhatPlacementShouldHaveKept: every shape a runner job must not have is E708 and
// nothing is minted, so a placement bug cannot put a credential on a weak node or a weak job.
func TestPreparerRefusesWhatPlacementShouldHaveKept(t *testing.T) {
	jobs := map[string]func(*model.Job){
		"not remote only": func(j *model.Job) { j.RemoteOnly = false },
		"process level":   func(j *model.Job) { j.Isolation = "process" },
		"container only":  func(j *model.Job) { j.Isolation = "container" },
		"has secrets":     func(j *model.Job) { j.Secrets = []string{"TOKEN"} },
		"image by tag":    func(j *model.Job) { j.Container = &model.ContainerConfig{Image: "alpine:3.19"} },
		"no container":    func(j *model.Job) { j.Container = nil },
		"macos platform":  func(j *model.Job) { j.Matrix = &model.JobMatrix{Platform: "darwin/arm64"} },
		"no demand":       func(j *model.Job) { delete(j.Env, "NSELF_RUNNERJOB_DEMAND") },
		"odd repo":        func(j *model.Job) { j.Env["NSELF_RUNNERJOB_REPO"] = "acme/x y" },
	}
	for name, mutate := range jobs {
		use(t)
		job := runnerJob(t, "12")
		mutate(&job)
		if _, err := deliver(goodNode(), "attempt-bad", job); code(err) == "" || !strings.HasPrefix(code(err), "E708") || len(forge.minted) != 0 {
			t.Errorf("%s: want E708 and no mint, got %v (minted %v)", name, err, forge.minted)
		}
	}
	nodes := map[string]func(*model.Capability){
		"macos node":       func(c *model.Capability) { c.Platform.OS = known("darwin") },
		"plain container":  func(c *model.Capability) { c.Trust.Isolation = known(model.Isolation("container")) },
		"no gvisor":        func(c *model.Capability) { c.Tools.GVisor = known(false) },
		"unknown gvisor":   func(c *model.Capability) { c.Tools.GVisor = model.Fact[bool]{} },
		"no restricted":    func(c *model.Capability) { c.Trust.Network = known([]model.NetworkScope{"none", "internet"}) },
		"unknown platform": func(c *model.Capability) { c.Platform.OS = model.Fact[string]{} },
	}
	for name, mutate := range nodes {
		use(t)
		node := goodNode()
		mutate(&node)
		if _, err := deliver(node, "attempt-bad", runnerJob(t, "13")); !strings.HasPrefix(code(err), "E708") || len(forge.minted) != 0 {
			t.Errorf("%s: want E708 and no mint, got %v (minted %v)", name, err, forge.minted)
		}
	}
}

// TestPreparerRefusesAPresetCredentialAndAWrongMinter: nobody else may put a credential in the lease, and a
// job naming another forge than the minter of its kind gets nothing.
func TestPreparerRefusesAPresetCredentialAndAWrongMinter(t *testing.T) {
	use(t)
	l := protocol.Lease{LeaseID: "l", AttemptID: "a", Job: runnerJob(t, "14"), RunnerCredential: &protocol.RunnerCredential{Kind: "github-jitconfig", Value: "planted", ExpiresAt: "2030-01-01T00:00:00Z"}}
	if err := transport.Deliver(context.Background(), goodNode(), l, nil); !strings.HasPrefix(code(err), "E708") || len(forge.minted) != 0 {
		t.Fatalf("a preset credential must be refused, got %v", err)
	}
	job := runnerJob(t, "15")
	job.Env["NSELF_RUNNERJOB_PROVIDER"] = "otherforge"
	if _, err := deliver(goodNode(), "a2", job); !strings.HasPrefix(code(err), "E708") || len(forge.minted) != 0 {
		t.Fatalf("a provider mismatch must be refused, got %v", err)
	}
}

// TestPreparerFailsClosedWithoutLedgerMinterOrV15: unbound, no minter for the kind, or v1.4 mode: E700 or
// E708 and nothing minted.
func TestPreparerFailsClosedWithoutLedgerMinterOrV15(t *testing.T) {
	use(t)
	job := runnerJob(t, "16")
	job.RunnerCredential = "unknown-kind"
	if _, err := deliver(goodNode(), "a", job); !strings.HasPrefix(code(err), "E700") {
		t.Fatalf("no minter: %v", err)
	}
	t.Setenv("NSELF_V15", "0")
	if _, err := deliver(goodNode(), "a", runnerJob(t, "16")); !strings.HasPrefix(code(err), "E708 compat") || len(forge.minted) != 0 {
		t.Fatalf("v1.4 mode must not mint: %v", err)
	}
}

// TestPreparerMintFailureLeavesNoCredentialAndNoRow: the forge refuses; the error carries no credential
// and the ledger stays empty.
func TestPreparerMintFailureLeavesNoCredentialAndNoRow(t *testing.T) {
	use(t)
	forge.mintErr = providers.NewError(providers.Config, "E707", "mint.rejected", 422, epoch)
	l, err := deliver(goodNode(), "attempt-f", runnerJob(t, "17"))
	if code(err) != "E707 mint.rejected" || l.RunnerCredential != nil {
		t.Fatalf("err %v lease %+v", err, l.RunnerCredential)
	}
	if _, ok, _ := ledgerRunner("attempt-f"); ok {
		t.Fatal("a failed mint must leave no ledger row")
	}
}

// TestPreparerSecondDeliveryReplacesTheRegistration: a retry of the same attempt removes the first
// registration before it mints the second (the credential is not stored, so it cannot be reused), and
// two live registrations never exist for one attempt.
func TestPreparerSecondDeliveryReplacesTheRegistration(t *testing.T) {
	use(t)
	for i := 0; i < 2; i++ {
		if _, err := deliver(goodNode(), "attempt-r", runnerJob(t, "18")); err != nil {
			t.Fatal(err)
		}
	}
	if forge.alive() != 1 || len(forge.minted) != 2 || len(forge.deregCalls) != 1 || forge.deregCalls[0] != "repo:101" {
		t.Fatalf("alive %d minted %d deregistered %v", forge.alive(), len(forge.minted), forge.deregCalls)
	}
}

// TestPreparerRefusesARunnerJobThatLostItsCredentialKind: the runner markers are in the job env but the
// json:"-" credential kind is gone (the pipeline came back from the store without Restore). It is refused,
// never run as an ordinary job on a node.
func TestPreparerRefusesARunnerJobThatLostItsCredentialKind(t *testing.T) {
	use(t)
	job := runnerJob(t, "19")
	job.RunnerCredential, job.RemoteOnly = "", false
	l, err := deliver(goodNode(), "attempt-lost", job)
	if code(err) != "E708 runner.restore_missing" || l.RunnerCredential != nil || len(forge.minted) != 0 {
		t.Fatalf("err %v lease %+v minted %v", err, l.RunnerCredential, forge.minted)
	}
}

// TestSecondDeliveryKeepsTheDemandSlot: while a retried attempt mints again (or its mint fails) another
// demand cannot take the slot of the first, so max_pending holds across a re-delivery.
func TestSecondDeliveryKeepsTheDemandSlot(t *testing.T) {
	use(t)
	led := runnerjob.CurrentLedger()
	ctx := context.Background()
	if ok, _ := led.Reserve(ctx, "fakeforge", testRepo, "20", 1); !ok {
		t.Fatal("reserve")
	}
	if _, err := deliver(goodNode(), "attempt-s", runnerJob(t, "20")); err != nil {
		t.Fatal(err)
	}
	forge.mintErr = providers.NewError(providers.Transient, "E704", "forge down", 0, epoch)
	if _, err := deliver(goodNode(), "attempt-s", runnerJob(t, "20")); err == nil {
		t.Fatal("the second mint should have failed")
	}
	if ok, _ := led.Reserve(ctx, "fakeforge", testRepo, "21", 1); ok {
		t.Fatal("another demand took the slot while the first was between registrations")
	}
	if ok, _ := led.Reserve(ctx, "fakeforge", testRepo, "20", 1); !ok {
		t.Fatal("the same demand, offered again, keeps its slot")
	}
	forge.mintErr = nil
	if _, err := deliver(goodNode(), "attempt-s", runnerJob(t, "20")); err != nil || forge.alive() != 1 {
		t.Fatalf("a later delivery mints again: %v alive %d", err, forge.alive())
	}
}

// TestLateMintCannotExceedTheCap (Opus M2): demands 1 and 2 hold the two slots, wait longer than the
// pending window for placement and expire; demands 3 and 4 are then reserved and take the slots. When the
// pipelines of 1 and 2 finally reach delivery they are refused, so two registrations exist, not four.
func TestLateMintCannotExceedTheCap(t *testing.T) {
	use(t)
	led := runnerjob.CurrentLedger()
	ctx := context.Background()
	for _, d := range []string{"1", "2"} {
		if ok, _ := led.Reserve(ctx, "fakeforge", testRepo, d, 2); !ok {
			t.Fatal("reserve", d)
		}
	}
	advance(runnerjob.PendingTTL + time.Minute)
	for _, d := range []string{"3", "4"} {
		if ok, _ := led.Reserve(ctx, "fakeforge", testRepo, d, 2); !ok {
			t.Fatal("the expired demands must have freed their slots for", d)
		}
	}
	for _, d := range []string{"1", "2"} {
		if _, err := deliverRaw(goodNode(), "late-"+d, runnerJob(t, d)); code(err) != "E703 runner.demand_not_live" {
			t.Fatalf("demand %s: a late mint over a taken cap must be refused, got %v", d, err)
		}
	}
	for _, d := range []string{"3", "4"} {
		if _, err := deliverRaw(goodNode(), "ok-"+d, runnerJob(t, d)); err != nil {
			t.Fatal(err)
		}
	}
	if forge.alive() != 2 {
		t.Fatalf("%d registrations for a cap of 2", forge.alive())
	}
}

// TestExpiredDemandWithAFreeSlotMintsAgain: the same wait, but nobody took the slot: the late pipeline
// mints, and the slot counts again.
func TestExpiredDemandWithAFreeSlotMintsAgain(t *testing.T) {
	use(t)
	led := runnerjob.CurrentLedger()
	ctx := context.Background()
	if ok, _ := led.Reserve(ctx, "fakeforge", testRepo, "1", 1); !ok {
		t.Fatal("reserve")
	}
	advance(runnerjob.PendingTTL + time.Minute)
	if _, err := deliverRaw(goodNode(), "late", runnerJob(t, "1")); err != nil {
		t.Fatalf("a late demand with a free slot may mint: %v", err)
	}
	if ok, _ := led.Reserve(ctx, "fakeforge", testRepo, "2", 1); ok {
		t.Fatal("the late registration holds the slot again")
	}
}

// TestRerunOfAFinishedDemandIsRefused (Opus M2 variant): once a demand is closed, a new attempt for the same
// demand id mints nothing; a rerun is a new demand and goes through Reserve.
func TestRerunOfAFinishedDemandIsRefused(t *testing.T) {
	use(t)
	if _, err := deliver(goodNode(), "first", runnerJob(t, "7")); err != nil {
		t.Fatal(err)
	}
	if err := finalize(context.Background(), "first"); err != nil {
		t.Fatal(err)
	}
	if _, err := deliverRaw(goodNode(), "again", runnerJob(t, "7")); code(err) != "E703 runner.demand_not_live" || forge.alive() != 0 {
		t.Fatalf("a finished demand must not mint again: %v alive %d", err, forge.alive())
	}
}
