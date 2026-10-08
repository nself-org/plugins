package store

import (
	"context"
	"path/filepath"
	"testing"
)

func TestNodeAggregateAudit(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "state.db"), Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	n := NodeRecord{ID: "node-1", Name: "node", State: "offline", Lifecycle: []byte(`{"discovered":true}`), Trust: []byte(`{}`), DeployHost: []byte(`{}`), Capability: []byte(`{"token":"SECRET_FIXTURE_DO_NOT_STORE_IN_AUDIT"}`)}
	if err = s.NodePut(ctx, n, "admin"); err != nil {
		t.Fatal(err)
	}
	n, err = s.NodeGet(ctx, n.ID)
	if err != nil {
		t.Fatal(err)
	}
	if n.Version != 1 || n.Digest == "" {
		t.Fatal("missing version or digest")
	}
	n.State = "maintenance"
	n.Reason = "age_recipient_changed"
	if err = s.NodeSetState(ctx, n, "admin", n.Reason); err != nil {
		t.Fatal(err)
	}
	if err = s.NodeSetState(ctx, n, "admin", n.Reason); err == nil {
		t.Fatal("stale version accepted")
	}
	var count int
	var details string
	if err = s.readers.QueryRowContext(ctx, `SELECT count(*),group_concat(detail) FROM audit WHERE target=?`, n.ID).Scan(&count, &details); err != nil {
		t.Fatal(err)
	}
	if count != 2 || details != "registered,age_recipient_changed" {
		t.Fatalf("audit count/details: %d %s", count, details)
	}
	if err = s.readers.QueryRowContext(ctx, `SELECT count(*) FROM audit WHERE detail LIKE '%SECRET_FIXTURE%'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("secret fixture leaked to audit")
	}
}
