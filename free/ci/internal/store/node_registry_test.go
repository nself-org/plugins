package store

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
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
	n, err = s.NodeGet(ctx, n.ID)
	if err != nil {
		t.Fatal(err)
	}
	n.Reason = "SECRET_FIXTURE_DO_NOT_STORE_IN_AUDIT"
	if err = s.NodeSetState(ctx, n, "admin", n.Reason); err != nil {
		t.Fatal(err)
	}
	var count int
	var details string
	if err = s.readers.QueryRowContext(ctx, `SELECT count(*),group_concat(detail) FROM audit WHERE target=?`, n.ID).Scan(&count, &details); err != nil {
		t.Fatal(err)
	}
	if count != 3 || details != "registered,age_recipient_changed,state changed" {
		t.Fatalf("audit count/details: %d %s", count, details)
	}
	if err = s.readers.QueryRowContext(ctx, `SELECT count(*) FROM audit WHERE detail LIKE '%SECRET_FIXTURE%'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("secret fixture leaked to audit")
	}
}

func TestNodeCapabilityStoreAuthority(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "state.db"), Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var c model.Capability
	// The store boundary must reject a direct caller's trust and lifecycle rewrite.
	// A valid fixture keeps the proof independent of registry sanitization.
	fixture, err := os.ReadFile("../model/testdata/capability/valid/laptop.valid.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(fixture, &c); err != nil {
		t.Fatal(err)
	}
	n := NodeRecord{ID: c.Identity.ID, Name: c.Identity.Name, State: "offline", Capability: fixture, Lifecycle: []byte(`{"discovered":true,"authorized_projects":[],"eligible":false,"revoked_at":null}`), Trust: []byte(`{}`), DeployHost: []byte(`{}`)}
	if err := s.NodePut(ctx, n, "operator"); err != nil {
		t.Fatal(err)
	}
	n, err = s.NodeGet(ctx, n.ID)
	if err != nil {
		t.Fatal(err)
	}
	before := n
	var initial model.Capability
	if err := json.Unmarshal(before.Capability, &initial); err != nil {
		t.Fatal(err)
	}
	if _, err := s.writer.ExecContext(ctx, `CREATE TRIGGER capability_guard BEFORE UPDATE OF trust_json,lifecycle,state,reason,deploy_host_json ON node BEGIN SELECT RAISE(FAIL, 'capability updated authority column'); END`); err != nil {
		t.Fatal(err)
	}
	n.Trust = []byte(`{"agent":"trusted"}`)
	n.Lifecycle = []byte(`{"discovered":true,"authorized_projects":["secret"],"eligible":true,"revoked_at":null}`)
	n.State = "online"
	n.Reason = "forged"
	c.Lifecycle.AuthorizedProjects = []string{"secret"}
	c.Lifecycle.Eligible = true
	c.Identity.Ownership.Value = new(string)
	*c.Identity.Ownership.Value = "team"
	c.Trust.Accepts.Value = &[]model.TrustClass{"untrusted"}
	n.Capability, err = json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.NodeUpdateCapability(ctx, n, "agent"); err != nil {
		t.Fatal(err)
	}
	got, err := s.NodeGet(ctx, n.ID)
	if err != nil {
		t.Fatal(err)
	}
	if string(got.Trust) != string(before.Trust) || string(got.Lifecycle) != string(before.Lifecycle) || got.State != before.State || got.Reason != before.Reason {
		t.Fatalf("authority fields changed: %+v", got)
	}
	var saved model.Capability
	if err := json.Unmarshal(got.Capability, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Lifecycle.Eligible || len(saved.Lifecycle.AuthorizedProjects) != 0 || saved.Identity.Ownership.Value != nil || !reflect.DeepEqual(saved.Trust, initial.Trust) {
		t.Fatal("capability document retained agent authority fields")
	}
}

func TestNodeRecoveryAudit(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "state.db"), Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	fixture, err := os.ReadFile("../model/testdata/capability/valid/laptop.valid.json")
	if err != nil {
		t.Fatal(err)
	}
	var c model.Capability
	if err := json.Unmarshal(fixture, &c); err != nil {
		t.Fatal(err)
	}
	n := NodeRecord{ID: c.Identity.ID, Name: c.Identity.Name, State: "offline", Capability: fixture}
	if err := s.NodePut(ctx, n, "operator"); err != nil {
		t.Fatal(err)
	}
	n, err = s.NodeGet(ctx, n.ID)
	if err != nil {
		t.Fatal(err)
	}
	n.State = "revoked"
	if err := s.NodeSetState(ctx, n, "admin", ""); err != nil {
		t.Fatal(err)
	}
	n, err = s.NodeGet(ctx, n.ID)
	if err != nil {
		t.Fatal(err)
	}
	n.State = "online"
	if err := s.NodeSetState(ctx, n, "agent", ""); err == nil {
		t.Fatal("agent recovered revoked node")
	}
	admin, err := NewAdminAuthority("admin")
	if err != nil {
		t.Fatal(err)
	}
	n.State = "offline"
	if err := s.NodeRecover(ctx, n, admin); err != nil {
		t.Fatal(err)
	}
	n, err = s.NodeGet(ctx, n.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(n.Capability, &c); err != nil {
		t.Fatal(err)
	}
	if c.Lifecycle.RevokedAt != nil {
		t.Fatal("revoked_at remains after recovery")
	}
	var count int
	if err := s.readers.QueryRowContext(ctx, `SELECT count(*) FROM audit WHERE target=? AND action='node.recover' AND actor='admin'`, n.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("recovery audit rows: %d", count)
	}
}
