package store

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

func settingsNode(t *testing.T, s *Store) NodeRecord {
	t.Helper()
	raw, err := os.ReadFile("../model/testdata/capability/valid/laptop.valid.json")
	if err != nil {
		t.Fatal(err)
	}
	var c model.Capability
	if err = json.Unmarshal(raw, &c); err != nil {
		t.Fatal(err)
	}
	n := NodeRecord{ID: c.Identity.ID, Name: c.Identity.Name, State: "offline", Capability: raw, Lifecycle: []byte(`{"discovered":true,"authorized_projects":[],"eligible":false,"revoked_at":null}`), Trust: []byte(`{}`), DeployHost: []byte(`{}`)}
	if err = s.NodePut(context.Background(), n, "operator"); err != nil {
		t.Fatal(err)
	}
	n, err = s.NodeGet(context.Background(), n.ID)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func goodSettings(id string) NodeSettings {
	return NodeSettings{NodeID: id, ReservedCPU: 2, ReservedMemMB: 4096, Interactive: InteractiveProtect, OnBattery: OnBatteryPause, Persistence: PersistenceInterm}
}

func TestNodeSettingsRoundTripAndSnapshot(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	n := settingsNode(t, s)
	if _, ok, err := s.NodeSettingsGet(ctx, n.ID); ok || err != nil {
		t.Fatalf("fresh node has settings: ok=%v err=%v", ok, err)
	}
	admin, _ := NewAdminAuthority("cli:admin")
	got, err := s.NodePutSettings(ctx, goodSettings(n.ID), admin)
	if err != nil || got.Version != 1 || got.ReservedCPU != 2 {
		t.Fatalf("put: %+v %v", got, err)
	}
	next := goodSettings(n.ID)
	next.Version = got.Version
	next.OnBattery, next.Persistence = OnBatteryAllow, PersistencePersist
	if got, err = s.NodePutSettings(ctx, next, admin); err != nil || got.Version != 2 || got.OnBattery != OnBatteryAllow {
		t.Fatalf("update: %+v %v", got, err)
	}
	var detail string
	if err := s.readers.QueryRowContext(ctx, `SELECT detail FROM audit WHERE action='node.settings' ORDER BY seq DESC LIMIT 1`).Scan(&detail); err != nil || detail != "reserved_cpu=2 reserved_mem_mb=4096 interactive=protect on_battery=allow persistence=persistent" {
		t.Fatalf("audit detail %q: %v", detail, err)
	}
	after, err := s.NodeGet(ctx, n.ID)
	if err != nil || after.Version != n.Version+2 {
		t.Fatalf("node version %d -> %d: %v", n.Version, after.Version, err)
	}
	var c model.Capability
	if err = json.Unmarshal(after.Capability, &c); err != nil {
		t.Fatal(err)
	}
	r := c.Resources.Reserved
	if r.CPU.Value == nil || *r.CPU.Value != 2 || r.MemMB.Value == nil || *r.MemMB.Value != 4096 || r.CPU.Source != "assigned" {
		t.Fatalf("reserved facts: %+v", r)
	}
	a := c.Availability
	if a.OnBattery.Value == nil || *a.OnBattery.Value != "allow" || a.Persistence.Value == nil || !*a.Persistence.Value || *a.InteractivePolicy.Value != "protect" {
		t.Fatalf("availability facts: %+v", a)
	}
}

func TestNodeSettingsSurviveAgentCapabilityReport(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	n := settingsNode(t, s)
	admin, _ := NewAdminAuthority("cli:admin")
	if _, err := s.NodePutSettings(ctx, goodSettings(n.ID), admin); err != nil {
		t.Fatal(err)
	}
	n, _ = s.NodeGet(ctx, n.ID)
	var c model.Capability
	if err := json.Unmarshal(n.Capability, &c); err != nil {
		t.Fatal(err)
	}
	zero := 0.0
	c.Resources.Reserved.CPU.Value = &zero // an agent report may not rewrite an assigned fact
	n.Capability, _ = json.Marshal(c)
	if err := s.NodeUpdateCapability(ctx, n, "agent"); err != nil {
		t.Fatal(err)
	}
	n, _ = s.NodeGet(ctx, n.ID)
	if err := json.Unmarshal(n.Capability, &c); err != nil || c.Resources.Reserved.CPU.Value == nil || *c.Resources.Reserved.CPU.Value != 2 {
		t.Fatalf("agent report changed the reservation: %v", err)
	}
}

func TestNodeSettingsRejections(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	n := settingsNode(t, s)
	admin, _ := NewAdminAuthority("cli:admin")
	bad := map[string]func(*NodeSettings){
		"negative cpu": func(v *NodeSettings) { v.ReservedCPU = -1 },
		"negative mem": func(v *NodeSettings) { v.ReservedMemMB = -1 },
		"interactive":  func(v *NodeSettings) { v.Interactive = "maybe" },
		"on_battery":   func(v *NodeSettings) { v.OnBattery = "" },
		"persistence":  func(v *NodeSettings) { v.Persistence = "always" },
	}
	for name, mutate := range bad {
		v := goodSettings(n.ID)
		mutate(&v)
		if _, err := s.NodePutSettings(ctx, v, admin); !codeIs(err, "E651") {
			t.Errorf("%s: err = %v, want E651", name, err)
		}
	}
	if _, err := s.NodePutSettings(ctx, goodSettings("nope"), admin); !codeIs(err, "E650") {
		t.Errorf("unknown node: %v", err)
	}
	if _, err := s.NodePutSettings(ctx, goodSettings(n.ID), AdminAuthority{}); !codeIs(err, "E651") {
		t.Errorf("the zero authority was accepted: %v", err)
	}
	if _, ok, _ := s.NodeSettingsGet(ctx, n.ID); ok {
		t.Fatal("a refused write left a row")
	}
}

func TestNodeSettingsRefuseAStaleVersion(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	n := settingsNode(t, s)
	admin, _ := NewAdminAuthority("cli:admin")
	first, err := s.NodePutSettings(ctx, goodSettings(n.ID), admin)
	if err != nil {
		t.Fatal(err)
	}
	// Two operators read version 1; the first writes, the second is told to read again.
	a, b := first, first
	a.ReservedCPU = 4
	b.OnBattery = OnBatteryAllow
	if _, err := s.NodePutSettings(ctx, a, admin); err != nil {
		t.Fatal(err)
	}
	if _, err := s.NodePutSettings(ctx, b, admin); !codeIs(err, "E605") {
		t.Fatalf("a stale write was accepted: %v", err)
	}
	got, _, _ := s.NodeSettingsGet(ctx, n.ID)
	if got.ReservedCPU != 4 || got.OnBattery != OnBatteryPause {
		t.Fatalf("the first writer's value was lost: %+v", got)
	}
	// A node with no row accepts only version 0.
	other := goodSettings(n.ID)
	other.Version = 5
	s2 := testStore(t)
	n2 := settingsNode(t, s2)
	other.NodeID = n2.ID
	if _, err := s2.NodePutSettings(ctx, other, admin); !codeIs(err, "E605") {
		t.Fatalf("a made-up version was accepted: %v", err)
	}
}
