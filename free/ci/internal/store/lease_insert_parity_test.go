package store

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestLeaseRowsHaveOneInsertSite fails when a second copy of the lease or
// host_reservation insert appears in non-test store code (D-1045): a column
// added to one copy would drift from the other.
func TestLeaseRowsHaveOneInsertSite(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{"INSERT INTO lease(", "INSERT INTO host_reservation("} {
		var sites []string
		for _, f := range files {
			if strings.HasSuffix(f, "_test.go") {
				continue
			}
			src, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			if n := strings.Count(string(src), stmt); n > 0 {
				sites = append(sites, fmt.Sprintf("%s x%d", f, n))
			}
		}
		if len(sites) != 1 || !strings.HasSuffix(sites[0], " x1") {
			t.Errorf("%q must be written at exactly one site, found %v", stmt, sites)
		}
	}
}

// leaseRowCells reads every column of the lease and host_reservation rows made
// for lease id, except the identity columns that differ per attempt.
func leaseRowCells(t *testing.T, s *Store, table, leaseID string) map[string]any {
	t.Helper()
	key := "id"
	if table == "host_reservation" {
		key = "lease_id"
	}
	rows, err := s.readers.QueryContext(context.Background(), "SELECT * FROM "+table+" WHERE "+key+"=?", leaseID)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	cols, err := rows.Columns()
	if err != nil {
		t.Fatal(err)
	}
	if !rows.Next() {
		t.Fatalf("%s row for %s missing", table, leaseID)
	}
	vals := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	if err := rows.Scan(ptrs...); err != nil {
		t.Fatal(err)
	}
	out := map[string]any{}
	for i, c := range cols {
		switch c {
		case "id", "lease_id", "attempt_id":
			continue
		}
		out[c] = vals[i]
	}
	return out
}

// TestLeaseGrantAndAdmitPlacementWriteSameRows pins that both grant paths write
// the same lease and host_reservation columns for the same demand.
func TestLeaseGrantAndAdmitPlacementWriteSameRows(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	s.now = func() time.Time { return time.Unix(100, 0) }
	admitSeed(t, s, "pipe", "proj", "build", "g1")
	admitSeed(t, s, "pipe", "proj", "build", "p1")
	fenceG, err := s.LeaseGrant(ctx, "host", admitDemand("g1", "r1"), 7)
	if err != nil {
		t.Fatal(err)
	}
	fenceP, err := s.AdmitPlacement(ctx, "host", admitDemand("p1", "r1"), AdmitRequest{Incarnation: 7})
	if err != nil {
		t.Fatal(err)
	}
	if fenceG != fenceP {
		t.Fatalf("fence differs: grant=%d admit=%d", fenceG, fenceP)
	}
	for _, table := range []string{"lease", "host_reservation"} {
		g := leaseRowCells(t, s, table, "lease-g1")
		p := leaseRowCells(t, s, table, "lease-p1")
		if len(g) == 0 || fmt.Sprint(g) != fmt.Sprint(p) {
			t.Errorf("%s columns differ:\n grant=%v\n admit=%v", table, g, p)
		}
	}
}
