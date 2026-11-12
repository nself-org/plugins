package main

// Purpose: P7-WIRE-11 through the real argv entry: `nodes revoke` revokes the node's signed evidence (the
// audit row evidence.revoke names the node), and a revoke of an unknown node writes none.

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func wire11Audit(t *testing.T, home string) (actions []string) {
	t.Helper()
	st, err := store.Open(filepath.Join(home, "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	es, err := st.AuditEntries(context.Background(), 0, 1000)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range es {
		actions = append(actions, e.Action+" "+e.Target)
	}
	return actions
}

func TestWire11NodesRevokeRevokesEvidence(t *testing.T) {
	home := pairHome(t)
	st, err := store.Open(filepath.Join(home, "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile("../internal/model/testdata/capability/valid/laptop.valid.json")
	if err != nil {
		t.Fatal(err)
	}
	var c model.Capability
	if err = json.Unmarshal(b, &c); err != nil {
		t.Fatal(err)
	}
	c.Identity.ID, c.Identity.Name = "n-w11", "n-w11"
	doc, _ := json.Marshal(c)
	admin, _ := store.NewAdminAuthority("cli:test")
	if err = st.NodeAdd(context.Background(), store.NodeRecord{ID: "n-w11", Name: "n-w11", State: "online", Reason: "added", Capability: doc}, admin); err != nil {
		t.Fatal(err)
	}
	_ = st.Close()

	if code, _, errs := capture(t, func() int { return runV15([]string{"nodes", "revoke", "nope"}) }); code == 0 {
		t.Fatalf("revoke of an unknown node succeeded: %s", errs)
	}
	for _, a := range wire11Audit(t, home) {
		if a == "evidence.revoke nope" {
			t.Fatal("an evidence revocation was written for a node that was never revoked")
		}
	}
	if code, _, errs := capture(t, func() int { return runV15([]string{"nodes", "revoke", "n-w11"}) }); code != 0 {
		t.Fatalf("nodes revoke exit %d: %s", code, errs)
	}
	found := false
	for _, a := range wire11Audit(t, home) {
		found = found || a == "evidence.revoke n-w11"
	}
	if !found {
		t.Fatalf("nodes revoke did not revoke the node's evidence; audit: %v", wire11Audit(t, home))
	}
}
