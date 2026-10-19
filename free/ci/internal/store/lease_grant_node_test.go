package store

import (
	"context"
	"errors"
	"testing"
)

// putGateNode inserts a bare registry row; LeaseGrant reads only state and lifecycle.
func putGateNode(t *testing.T, s *Store, id, state, lifecycle string) {
	t.Helper()
	if _, err := s.writer.ExecContext(context.Background(), `INSERT INTO node(id,name,version,lifecycle,trust_json,deploy_host_json,state,reason,created_at,updated_at) VALUES(?,?,1,?,'{}','{}',?,'',1,1)`, id, id, lifecycle, state); err != nil {
		t.Fatal(err)
	}
}

// TestLeaseGrantRefusesUnavailableNode pins D-1086: the real grant path (not a
// caller's optional pre-check) refuses a revoked, draining, unknown or
// unauthorized node and leaves no lease or reservation behind.
func TestLeaseGrantRefusesUnavailableNode(t *testing.T) {
	const authorized = `{"discovered":false,"authorized_projects":["proj"],"eligible":true,"revoked_at":null}`
	cases := []struct {
		name, node, state, lifecycle, project, want string
	}{
		{"revoked", "n1", "revoked", authorized, "proj", "revoked"},
		{"draining", "n1", "draining", authorized, "proj", "draining"},
		{"unknown node", "gone", "online", authorized, "proj", "unknown"},
		{"revoked_at set", "n1", "online", `{"discovered":false,"authorized_projects":["proj"],"revoked_at":"2026-10-19T00:00:00Z"}`, "proj", "unauthorized"},
		{"project not authorized", "n1", "online", authorized, "other", "unauthorized"},
		{"no project", "n1", "online", authorized, "", "unauthorized"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := testStore(t)
			ctx := context.Background()
			putGateNode(t, s, "n1", tc.state, tc.lifecycle)
			admitSeed(t, s, "pipe", "repo", "job", "a1")
			d := admitDemand("a1", "r1")
			d.NodeID, d.Project = tc.node, tc.project
			_, err := s.LeaseGrant(ctx, "host", d, 1)
			var un *NodeUnavailableError
			if !errors.As(err, &un) || un.State != tc.want {
				t.Fatalf("LeaseGrant = %v, want NodeUnavailableError %q", err, tc.want)
			}
			var n int
			if err := s.readers.QueryRowContext(ctx, "SELECT (SELECT count(*) FROM lease)+(SELECT count(*) FROM host_reservation)").Scan(&n); err != nil || n != 0 {
				t.Fatalf("refused grant left %d rows (err %v)", n, err)
			}
		})
	}
	t.Run("available node is granted", func(t *testing.T) {
		s := testStore(t)
		putGateNode(t, s, "n1", "online", authorized)
		admitSeed(t, s, "pipe", "repo", "job", "a1")
		d := admitDemand("a1", "r1")
		d.NodeID, d.Project = "n1", "proj"
		if fence, err := s.LeaseGrant(context.Background(), "host", d, 1); err != nil || fence == 0 {
			t.Fatalf("LeaseGrant = %d, %v", fence, err)
		}
	})
}
