package crypto

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/nself-org/nself-byok/tenantdb"
	"github.com/nself-org/nself-byok/tenantdb/tenantdbtest"
)

// TestRotatorRunsUnderTenantRLS: the rotation job's statements each open with the tenant
// setting, so forced RLS lets a non-superuser role see the tenant's rows (D-0956). The fake
// pool returns no rows, so Run stops at the record count; both statements before that must be scoped.
func TestRotatorRunsUnderTenantRLS(t *testing.T) {
	tenant := uuid.MustParse("33333333-3333-3333-3333-333333333333")
	pool := &tenantdbtest.Pool{}
	_, err := NewRotator(pool, nil, 10, 10).Run(context.Background(), tenant, uuid.New(), "key-1")
	if err == nil || !strings.Contains(err.Error(), "count records") {
		t.Fatalf("Run err = %v, want the count-records error", err)
	}
	if pool.Begins != 2 {
		t.Fatalf("transactions = %d, want 2 (start event, count)", pool.Begins)
	}
	seen := map[int]bool{}
	for _, c := range pool.Calls {
		if seen[c.TxID] {
			continue
		}
		seen[c.TxID] = true
		if !strings.Contains(c.SQL, "set_config") || c.Args[0] != tenantdb.Setting || c.Args[1] != tenant.String() {
			t.Fatalf("tx %d starts with %q %v, want the tenant setting", c.TxID, c.SQL, c.Args)
		}
	}
}
