package store

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

func lifecycleStore(t *testing.T) (*Store, NodeRecord) {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "state.db"), Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	b, err := os.ReadFile("../model/testdata/capability/valid/laptop.valid.json")
	if err != nil {
		t.Fatal(err)
	}
	var c model.Capability
	if err = json.Unmarshal(b, &c); err != nil {
		t.Fatal(err)
	}
	c.Lifecycle = model.CapabilityLifecycle{AuthorizedProjects: []string{"proj-1"}}
	doc, _ := json.Marshal(c)
	return s, NodeRecord{ID: c.Identity.ID, Name: c.Identity.Name, State: "online", Reason: "added", Capability: doc}
}

func countAudit(t *testing.T, s *Store, action string) (n int) {
	t.Helper()
	if err := s.readers.QueryRow("SELECT count(*) FROM audit WHERE action=?", action).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestNodeAddNeedsAnAdmin(t *testing.T) {
	s, n := lifecycleStore(t)
	for _, actor := range []string{"", "agent", "probe"} {
		if _, err := NewAdminAuthority(actor); err == nil {
			t.Errorf("actor %q became an admin", actor)
		}
	}
	if err := s.NodeAdd(context.Background(), n, AdminAuthority{}); err == nil {
		t.Fatal("zero authority accepted")
	}
	admin, _ := NewAdminAuthority("cli:test")
	if err := s.NodeAdd(context.Background(), n, admin); err != nil {
		t.Fatal(err)
	}
	if countAudit(t, s, "node.add") != 1 {
		t.Fatal("node.add audit row missing")
	}
	got, err := s.NodeGet(context.Background(), n.ID)
	if err != nil || got.State != "online" {
		t.Fatalf("stored node: %+v %v", got, err)
	}
	n.Capability = []byte(`{"identity":{"id":"other"}}`)
	if err = s.NodeAdd(context.Background(), n, admin); err == nil {
		t.Fatal("mismatched document accepted")
	}
}

func TestNodeRevokeIsOneTransaction(t *testing.T) {
	ctx := context.Background()
	s, n := lifecycleStore(t)
	admin, _ := NewAdminAuthority("cli:test")
	if err := s.NodeAdd(ctx, n, admin); err != nil {
		t.Fatal(err)
	}
	boom := errors.New("boom")
	ran := 0
	_, err := s.NodeRevoke(ctx, n.ID, "cli:test", []RevokeHook{
		func(_ context.Context, tx *RevokeTx, id string) error {
			ran++
			return tx.AppendAudit("cli:test", "deny.add", id, "x")
		},
		func(context.Context, *RevokeTx, string) error { return boom },
	})
	if !errors.Is(err, boom) || ran != 1 {
		t.Fatalf("err=%v ran=%d", err, ran)
	}
	got, _ := s.NodeGet(ctx, n.ID)
	if got.State == "revoked" || countAudit(t, s, "deny.add") != 0 || countAudit(t, s, "node.revoke") != 0 {
		t.Fatal("a failed hook left part of the revoke behind")
	}
	res, err := s.NodeRevoke(ctx, n.ID, "cli:test", nil)
	if err != nil || res.Removed {
		t.Fatalf("revoke: %+v %v", res, err)
	}
	got, _ = s.NodeGet(ctx, n.ID)
	var c model.Capability
	_ = json.Unmarshal(got.Capability, &c)
	if got.State != "revoked" || c.Lifecycle.RevokedAt == nil || c.Availability.State.Value == nil || *c.Availability.State.Value != "revoked" {
		t.Fatalf("not revoked in row and document: %+v", got)
	}
	if countAudit(t, s, "node.revoke") != 1 {
		t.Fatal("one audit row expected")
	}
}

func TestNodeRemoveKeepsAudit(t *testing.T) {
	ctx := context.Background()
	s, n := lifecycleStore(t)
	admin, _ := NewAdminAuthority("cli:test")
	if err := s.NodeAdd(ctx, n, admin); err != nil {
		t.Fatal(err)
	}
	res, err := s.NodeRemove(ctx, n.ID, "cli:test", nil)
	if err != nil || !res.Removed {
		t.Fatalf("remove: %+v %v", res, err)
	}
	if _, err = s.NodeGet(ctx, n.ID); err == nil {
		t.Fatal("node survived remove")
	}
	if countAudit(t, s, "node.add") != 1 || countAudit(t, s, "node.remove") != 1 {
		t.Fatal("audit history must survive remove")
	}
	if _, err = s.NodeRevoke(ctx, "", "cli:test", nil); err == nil {
		t.Fatal("empty node id accepted")
	}
	if _, err = s.NodeRevoke(ctx, n.ID, "cli:test", nil); err == nil || err.Error()[:4] != "E650" {
		t.Fatalf("revoke of a removed node: %v", err)
	}
}
