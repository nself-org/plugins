package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/nself-org/nself-byok/tenantdb"
	"github.com/nself-org/nself-byok/tenantdb/tenantdbtest"
)

const scopeTenant = "22222222-2222-2222-2222-222222222222"

// TestHandlersRunUnderTenantRLS: every database statement a handler sends is inside a
// transaction that first sets app.current_tenant_id to the requesting tenant. Without that
// the forced RLS policies hide every row from a non-superuser role (D-0956).
func TestHandlersRunUnderTenantRLS(t *testing.T) {
	h := func(p *tenantdbtest.Pool) *byokHandler { return &byokHandler{db: p} }
	tests := []struct {
		name       string
		call       func(p *tenantdbtest.Pool, w http.ResponseWriter, r *http.Request)
		method     string
		path       string
		body       string
		wantStatus int
		wantTable  bool // a table statement must have run
	}{
		{"get kms", func(p *tenantdbtest.Pool, w http.ResponseWriter, r *http.Request) { h(p).GetKMSConfig(w, r) },
			http.MethodGet, "/encryption/kms", "", http.StatusNotFound, true},
		{"configure kms", func(p *tenantdbtest.Pool, w http.ResponseWriter, r *http.Request) { h(p).ConfigureKMS(w, r) },
			http.MethodPost, "/encryption/kms", `{"provider":"aws","key_ref":"k"}`, http.StatusInternalServerError, true},
		{"update kms", func(p *tenantdbtest.Pool, w http.ResponseWriter, r *http.Request) { h(p).UpdateKMSConfig(w, r) },
			http.MethodPut, "/encryption/kms", `{"key_ref":"k2"}`, http.StatusOK, true},
		{"verify kms", func(p *tenantdbtest.Pool, w http.ResponseWriter, r *http.Request) { h(p).VerifyKMS(w, r) },
			http.MethodPost, "/encryption/kms/verify", "", http.StatusNotFound, true},
		{"trigger rotation", func(p *tenantdbtest.Pool, w http.ResponseWriter, r *http.Request) { h(p).TriggerRotation(w, r) },
			http.MethodPost, "/encryption/rotate", "", http.StatusNotFound, true},
		{"rotation status", func(p *tenantdbtest.Pool, w http.ResponseWriter, r *http.Request) {
			rc := chi.NewRouteContext()
			rc.URLParams.Add("job_id", "job-1")
			h(p).RotationStatus(w, r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rc)))
		}, http.MethodGet, "/encryption/rotate/job-1", "", http.StatusNotFound, true},
		{"list key events", func(p *tenantdbtest.Pool, w http.ResponseWriter, r *http.Request) { h(p).ListKeyEvents(w, r) },
			http.MethodGet, "/encryption/key-events", "", http.StatusInternalServerError, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pool := &tenantdbtest.Pool{}
			r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			r.Header.Set("X-Tenant-ID", scopeTenant)
			w := httptest.NewRecorder()
			tc.call(pool, w, r)
			if w.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d (%s)", w.Code, tc.wantStatus, w.Body.String())
			}
			assertEveryTxSetsTenant(t, pool, tc.wantTable)
		})
	}
}

// TestRotationStatusNeedsTenant: a job id alone no longer reads a rotation event.
func TestRotationStatusNeedsTenant(t *testing.T) {
	pool := &tenantdbtest.Pool{}
	r := httptest.NewRequest(http.MethodGet, "/encryption/rotate/job-1", nil)
	rc := chi.NewRouteContext()
	rc.URLParams.Add("job_id", "job-1")
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rc))
	w := httptest.NewRecorder()
	(&byokHandler{db: pool}).RotationStatus(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
	if len(pool.Calls) != 0 {
		t.Fatalf("database touched without a tenant: %v", pool.Calls)
	}
}

// TestZeroTenantIsUnauthorized: the all-zero UUID is not a tenant; it answers 401 and never reaches the database.
func TestZeroTenantIsUnauthorized(t *testing.T) {
	for _, tenant := range []string{"00000000-0000-0000-0000-000000000000", "not-a-uuid"} {
		pool := &tenantdbtest.Pool{}
		r := httptest.NewRequest(http.MethodGet, "/encryption/key-events", nil)
		r.Header.Set("X-Tenant-ID", tenant)
		w := httptest.NewRecorder()
		(&byokHandler{db: pool}).ListKeyEvents(w, r)
		if w.Code != http.StatusUnauthorized || len(pool.Calls) != 0 {
			t.Fatalf("tenant %q: status %d, %d calls; want 401 and none", tenant, w.Code, len(pool.Calls))
		}
	}
}

func assertEveryTxSetsTenant(t *testing.T, p *tenantdbtest.Pool, wantTable bool) {
	t.Helper()
	first := map[int]bool{}
	table := false
	for _, c := range p.Calls {
		if !first[c.TxID] {
			first[c.TxID] = true
			if !strings.Contains(c.SQL, "set_config") || len(c.Args) != 2 ||
				c.Args[0] != tenantdb.Setting || c.Args[1] != scopeTenant {
				t.Fatalf("tx %d starts with %q %v, want set_config(%s, %s, true)", c.TxID, c.SQL, c.Args, tenantdb.Setting, scopeTenant)
			}
			continue
		}
		table = true
	}
	if table != wantTable {
		t.Fatalf("table statement ran = %v, want %v (%d calls)", table, wantTable, len(p.Calls))
	}
}
