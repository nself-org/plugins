package lifecycle

// Purpose: tests of P7-NODE-30 in the lifecycle package: `nodes show|list --json` report the assigned
// labels as identity.labels (source assigned), and removing a project from a node refuses new leases (E657)
// while an attempt already running there is left alone.
// Inputs:  the kit of testkit_test.go. Outputs: pass or fail.
// Constraints: no network.

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/nodes/labels"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func TestShowAndListReportAssignedLabels(t *testing.T) {
	k := newKit(t)
	k.addNode("lb1")
	k.addNode("lb2")
	e := labels.Env{Store: k.st, Registry: k.d.Registry, Actor: "cli:test"}
	if _, err := e.Edit(context.Background(), "lb1", []string{"xcode", "arch=arm64"}, nil); err != nil {
		t.Fatal(err)
	}
	want := []LabelView{{"arch", "arm64", "assigned"}, {"xcode", "", "assigned"}}
	code, out, _ := k.run1("show", "lb1", "--json")
	var s struct {
		Node struct {
			Identity IdentityView `json:"identity"`
		} `json:"node"`
	}
	if err := json.Unmarshal(decode(t, out).Data, &s); err != nil || code != 0 || len(s.Node.Identity.Labels) != 2 || s.Node.Identity.Labels[0] != want[0] || s.Node.Identity.Labels[1] != want[1] {
		t.Fatalf("show: code %d %v labels %+v", code, err, s.Node.Identity.Labels)
	}
	code, out, _ = k.run1("list", "--json")
	var l struct {
		Nodes []struct {
			ID       string       `json:"id"`
			Identity IdentityView `json:"identity"`
		} `json:"nodes"`
	}
	if err := json.Unmarshal(decode(t, out).Data, &l); err != nil || code != 0 || len(l.Nodes) != 2 {
		t.Fatalf("list: code %d %v %s", code, err, out)
	}
	for _, n := range l.Nodes {
		if n.ID == "lb1" && len(n.Identity.Labels) != 2 || n.ID == "lb2" && (n.Identity.Labels == nil || len(n.Identity.Labels) != 0) {
			t.Fatalf("list labels of %s: %+v", n.ID, n.Identity.Labels)
		}
	}
}

func TestProjectsRemoveRefusesNewLeasesRunningAttemptFinishes(t *testing.T) {
	ctx := context.Background()
	k := newKit(t)
	k.addNode("rm1")
	if err := Gate(ctx, k.d.Registry, "rm1", "proj-1"); err != nil {
		t.Fatalf("authorized node refused: %v", err)
	}
	p := store.PipelineRow{ID: "pipe", Project: "proj-1", Revision: "rev", Trigger: "local", SourceTrust: "owner", PrivacyZone: "local-only", PolicyDigest: "p", InputDigest: "i", SelectionMode: "full", Status: "queued"}
	if err := k.st.CreatePipeline(ctx, p); err != nil {
		t.Fatal(err)
	}
	if err := k.st.CreateJob(ctx, store.JobRow{ID: "job1", PipelineID: p.ID, Name: "j", Kind: "test", Idempotent: true, InfraMax: 1, InputDigest: "i"}); err != nil {
		t.Fatal(err)
	}
	if err := k.st.CreateAttempt(ctx, store.AttemptRow{ID: "att1", JobID: "job1", N: 1}, ""); err != nil {
		t.Fatal(err)
	}
	if err := k.st.AdmitAndLease(ctx, "host", store.Demand{AttemptID: "att1", RunnerID: "rm1", LeaseID: "lease1", CPU: 1, MemMB: 1, CapacityCPU: 4, CapacityMemMB: 64}); err != nil {
		t.Fatal(err)
	}
	state := func() string {
		atts, err := k.st.PipelineAttempts(ctx, p.ID)
		if err != nil || len(atts) != 1 {
			t.Fatalf("attempts %v %v", atts, err)
		}
		return string(atts[0].State)
	}
	before := state()
	e := labels.Env{Store: k.st, Registry: k.d.Registry, Actor: "cli:test"}
	removed, err := e.RemoveProjects(ctx, "rm1", []string{"proj-1"}, false)
	if err != nil || len(removed) != 1 {
		t.Fatalf("remove: %v %v", removed, err)
	}
	errCode(t, Gate(ctx, k.d.Registry, "rm1", "proj-1"), CodeRevoked) // E657: no new lease for the removed project
	if after := state(); after != before {
		t.Fatalf("the running attempt was touched: %s -> %s", before, after)
	}
}
