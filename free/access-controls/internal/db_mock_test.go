// Purpose: Cover the DB-backed handler paths using pgxmock, so the real
// query/scan/error-handling logic is exercised without a live Postgres.
// Inputs: pgxmock-driven Handlers wired to expected SQL + rows.
// Outputs: asserted HTTP status/body for success, not-found, and DB-error paths.
// Constraints: SQL matching uses pgxmock's default regex matcher (substrings
// of the real query), so refactors that reorder columns need matching updates.
package internal

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/pashagolub/pgxmock/v4"
)

func newMockHandlers(t *testing.T) (*Handlers, pgxmock.PgxPoolIface) {
	t.Helper()
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock.NewPool: %v", err)
	}
	t.Cleanup(mock.Close)
	return &Handlers{DB: &DB{Pool: mock}, Cache: newPermCache(time.Minute), Cfg: Config{DefaultDeny: true}}, mock
}

// reqWithParam builds a request carrying a chi URL param, for handlers that
// call chi.URLParam(r, key). Pass body=nil for GET/DELETE-style requests.
func reqWithParam(method, target, key, val string, body io.Reader) *http.Request {
	r := httptest.NewRequest(method, target, body)
	rc := chi.NewRouteContext()
	rc.URLParams.Add(key, val)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rc))
}

// addParam attaches an additional chi URL param to a request already built
// via reqWithParam (for handlers with two path params).
func addParam(r *http.Request, key, val string) {
	chi.RouteContext(r.Context()).URLParams.Add(key, val)
}

func strPtr(s string) *string { return &s }

// anyArgs returns n pgxmock.AnyArg() matchers, for queries whose exact
// parameter values aren't under test (only the SQL shape + return rows are).
func anyArgs(n int) []interface{} {
	args := make([]interface{}, n)
	for i := range args {
		args[i] = pgxmock.AnyArg()
	}
	return args
}

// =============================================================================
// Health / Ready / NewHandlers
// =============================================================================

func TestNewHandlers(t *testing.T) {
	db := &DB{}
	cfg := Config{CacheTTLSeconds: 30}
	h := NewHandlers(db, cfg)
	if h.DB != db {
		t.Error("DB not wired")
	}
	if h.Cache == nil {
		t.Error("Cache not initialized")
	}
	if h.Cfg.CacheTTLSeconds != 30 {
		t.Errorf("Cfg = %+v", h.Cfg)
	}
}

func TestHealth(t *testing.T) {
	h := &Handlers{}
	r := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	h.Health(w, r)
	if w.Code != 200 {
		t.Errorf("status = %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"status":"ok"`) {
		t.Errorf("body = %s", w.Body.String())
	}
}

func TestReady_DBUp(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectExec("SELECT 1").WillReturnResult(pgxmock.NewResult("SELECT", 0))
	r := httptest.NewRequest("GET", "/ready", nil)
	w := httptest.NewRecorder()
	h.Ready(w, r)
	if w.Code != 200 {
		t.Errorf("status = %d; body=%s", w.Code, w.Body.String())
	}
}

func TestReady_DBDown(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectExec("SELECT 1").WillReturnError(errors.New("connection refused"))
	r := httptest.NewRequest("GET", "/ready", nil)
	w := httptest.NewRecorder()
	h.Ready(w, r)
	if w.Code != 503 {
		t.Errorf("status = %d; want 503", w.Code)
	}
}

// =============================================================================
// Roles
// =============================================================================

func roleCols() []string {
	return []string{"id", "source_account_id", "name", "display_name", "description",
		"parent_role_id", "level", "is_system", "metadata", "created_at", "updated_at"}
}

func TestListRoles_Success(t *testing.T) {
	h, mock := newMockHandlers(t)
	now := time.Now()
	rows := mock.NewRows(roleCols()).
		AddRow("r1", "primary", "admin", (*string)(nil), (*string)(nil), (*string)(nil), 0, true, []byte("{}"), now, now)
	mock.ExpectQuery("SELECT id, source_account_id, name").WithArgs(anyArgs(1)...).WillReturnRows(rows)

	r := httptest.NewRequest("GET", "/roles", nil)
	w := httptest.NewRecorder()
	h.ListRoles(w, r)
	if w.Code != 200 {
		t.Fatalf("status = %d; body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"admin"`) {
		t.Errorf("body = %s", w.Body.String())
	}
}

func TestListRoles_DBError(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("SELECT id, source_account_id, name").WithArgs(anyArgs(1)...).WillReturnError(errors.New("db down"))
	r := httptest.NewRequest("GET", "/roles", nil)
	w := httptest.NewRecorder()
	h.ListRoles(w, r)
	if w.Code != 500 {
		t.Errorf("status = %d; want 500", w.Code)
	}
}

func TestGetRole_Found(t *testing.T) {
	h, mock := newMockHandlers(t)
	now := time.Now()
	rows := mock.NewRows(roleCols()).
		AddRow("r1", "primary", "admin", (*string)(nil), (*string)(nil), (*string)(nil), 0, true, []byte("{}"), now, now)
	mock.ExpectQuery("SELECT id, source_account_id, name").WithArgs(anyArgs(2)...).WillReturnRows(rows)

	r := reqWithParam("GET", "/roles/r1", "id", "r1", nil)
	w := httptest.NewRecorder()
	h.GetRole(w, r)
	if w.Code != 200 {
		t.Fatalf("status = %d; body=%s", w.Code, w.Body.String())
	}
}

func TestGetRole_NotFound(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("SELECT id, source_account_id, name").WithArgs(anyArgs(2)...).WillReturnRows(mock.NewRows(roleCols()))

	r := reqWithParam("GET", "/roles/missing", "id", "missing", nil)
	w := httptest.NewRecorder()
	h.GetRole(w, r)
	if w.Code != 404 {
		t.Errorf("status = %d; want 404; body=%s", w.Code, w.Body.String())
	}
}

func TestCreateRole_Success(t *testing.T) {
	h, mock := newMockHandlers(t)
	now := time.Now()
	rows := mock.NewRows(roleCols()).
		AddRow("r1", "primary", "admin", (*string)(nil), (*string)(nil), (*string)(nil), 0, false, []byte("{}"), now, now)
	mock.ExpectQuery("INSERT INTO acl_roles").WithArgs(anyArgs(8)...).WillReturnRows(rows)

	r := httptest.NewRequest("POST", "/roles", strings.NewReader(`{"name":"admin"}`))
	w := httptest.NewRecorder()
	h.CreateRole(w, r)
	if w.Code != 201 {
		t.Fatalf("status = %d; body=%s", w.Code, w.Body.String())
	}
}

func TestCreateRole_DBError(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("INSERT INTO acl_roles").WithArgs(anyArgs(8)...).WillReturnError(errors.New("unique violation"))
	r := httptest.NewRequest("POST", "/roles", strings.NewReader(`{"name":"admin"}`))
	w := httptest.NewRecorder()
	h.CreateRole(w, r)
	if w.Code != 400 {
		t.Errorf("status = %d; want 400", w.Code)
	}
}

func TestCreateRole_WithParentRole(t *testing.T) {
	h, mock := newMockHandlers(t)
	// Parent-level lookup succeeds.
	mock.ExpectQuery("SELECT level FROM acl_roles").WithArgs(anyArgs(2)...).
		WillReturnRows(mock.NewRows([]string{"level"}).AddRow(2))
	now := time.Now()
	rows := mock.NewRows(roleCols()).
		AddRow("r2", "primary", "child", (*string)(nil), (*string)(nil), strPtr("r1"), 3, false, []byte("{}"), now, now)
	mock.ExpectQuery("INSERT INTO acl_roles").WithArgs(anyArgs(8)...).WillReturnRows(rows)

	r := httptest.NewRequest("POST", "/roles", strings.NewReader(`{"name":"child","parent_role_id":"r1"}`))
	w := httptest.NewRecorder()
	h.CreateRole(w, r)
	if w.Code != 201 {
		t.Fatalf("status = %d; body=%s", w.Code, w.Body.String())
	}
}

func TestUpdateRole_Success(t *testing.T) {
	h, mock := newMockHandlers(t)
	now := time.Now()
	rows := mock.NewRows(roleCols()).
		AddRow("r1", "primary", "admin", strPtr("Admin"), (*string)(nil), (*string)(nil), 0, false, []byte("{}"), now, now)
	mock.ExpectQuery("UPDATE acl_roles SET").WithArgs(anyArgs(3)...).WillReturnRows(rows)

	r := reqWithParam("PUT", "/roles/r1", "id", "r1", strings.NewReader(`{"display_name":"Admin"}`))
	w := httptest.NewRecorder()
	h.UpdateRole(w, r)
	if w.Code != 200 {
		t.Fatalf("status = %d; body=%s", w.Code, w.Body.String())
	}
}

func TestUpdateRole_NotFound(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("UPDATE acl_roles SET").WithArgs(anyArgs(3)...).WillReturnRows(mock.NewRows(roleCols()))

	r := reqWithParam("PUT", "/roles/x", "id", "x", strings.NewReader(`{"display_name":"X"}`))
	w := httptest.NewRecorder()
	h.UpdateRole(w, r)
	if w.Code != 404 {
		t.Errorf("status = %d; want 404", w.Code)
	}
}

func TestDeleteRole_Success(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectExec("DELETE FROM acl_roles").WithArgs(anyArgs(2)...).WillReturnResult(pgxmock.NewResult("DELETE", 1))
	r := reqWithParam("DELETE", "/roles/r1", "id", "r1", nil)
	w := httptest.NewRecorder()
	h.DeleteRole(w, r)
	if w.Code != 200 {
		t.Fatalf("status = %d; body=%s", w.Code, w.Body.String())
	}
}

func TestDeleteRole_NotFound(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectExec("DELETE FROM acl_roles").WithArgs(anyArgs(2)...).WillReturnResult(pgxmock.NewResult("DELETE", 0))
	r := reqWithParam("DELETE", "/roles/missing", "id", "missing", nil)
	w := httptest.NewRecorder()
	h.DeleteRole(w, r)
	if w.Code != 404 {
		t.Errorf("status = %d; want 404", w.Code)
	}
}

func TestDeleteRole_DBError(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectExec("DELETE FROM acl_roles").WithArgs(anyArgs(2)...).WillReturnError(errors.New("fk violation"))
	r := reqWithParam("DELETE", "/roles/r1", "id", "r1", nil)
	w := httptest.NewRecorder()
	h.DeleteRole(w, r)
	if w.Code != 500 {
		t.Errorf("status = %d; want 500", w.Code)
	}
}

// =============================================================================
// Permissions
// =============================================================================

func permCols() []string {
	return []string{"id", "source_account_id", "resource", "action", "description", "conditions", "created_at"}
}

func TestListPermissions_Success(t *testing.T) {
	h, mock := newMockHandlers(t)
	now := time.Now()
	rows := mock.NewRows(permCols()).AddRow("p1", "primary", "users", "read", (*string)(nil), []byte("{}"), now)
	mock.ExpectQuery("SELECT id, source_account_id, resource, action").WithArgs(anyArgs(1)...).WillReturnRows(rows)

	r := httptest.NewRequest("GET", "/permissions", nil)
	w := httptest.NewRecorder()
	h.ListPermissions(w, r)
	if w.Code != 200 {
		t.Fatalf("status = %d; body=%s", w.Code, w.Body.String())
	}
}

func TestGetPermission_NotFound(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("SELECT id, source_account_id, resource, action").WithArgs(anyArgs(2)...).WillReturnRows(mock.NewRows(permCols()))
	r := reqWithParam("GET", "/permissions/missing", "id", "missing", nil)
	w := httptest.NewRecorder()
	h.GetPermission(w, r)
	if w.Code != 404 {
		t.Errorf("status = %d; want 404", w.Code)
	}
}

func TestCreatePermission_Success(t *testing.T) {
	h, mock := newMockHandlers(t)
	now := time.Now()
	rows := mock.NewRows(permCols()).AddRow("p1", "primary", "users", "read", (*string)(nil), []byte("{}"), now)
	mock.ExpectQuery("INSERT INTO acl_permissions").WithArgs(anyArgs(5)...).WillReturnRows(rows)

	r := httptest.NewRequest("POST", "/permissions", strings.NewReader(`{"resource":"users","action":"read"}`))
	w := httptest.NewRecorder()
	h.CreatePermission(w, r)
	if w.Code != 201 {
		t.Fatalf("status = %d; body=%s", w.Code, w.Body.String())
	}
}

func TestUpdatePermission_NoFieldsDelegatesToGet(t *testing.T) {
	h, mock := newMockHandlers(t)
	now := time.Now()
	rows := mock.NewRows(permCols()).AddRow("p1", "primary", "users", "read", (*string)(nil), []byte("{}"), now)
	mock.ExpectQuery("SELECT id, source_account_id, resource, action").WithArgs(anyArgs(2)...).WillReturnRows(rows)

	r := reqWithParam("PUT", "/permissions/p1", "id", "p1", strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	h.UpdatePermission(w, r)
	if w.Code != 200 {
		t.Fatalf("status = %d; body=%s", w.Code, w.Body.String())
	}
}

func TestUpdatePermission_WithFields(t *testing.T) {
	h, mock := newMockHandlers(t)
	now := time.Now()
	rows := mock.NewRows(permCols()).AddRow("p1", "primary", "users", "read", strPtr("desc"), []byte("{}"), now)
	mock.ExpectQuery("UPDATE acl_permissions SET").WithArgs(anyArgs(3)...).WillReturnRows(rows)

	r := reqWithParam("PUT", "/permissions/p1", "id", "p1", strings.NewReader(`{"description":"desc"}`))
	w := httptest.NewRecorder()
	h.UpdatePermission(w, r)
	if w.Code != 200 {
		t.Fatalf("status = %d; body=%s", w.Code, w.Body.String())
	}
}

func TestDeletePermission_NotFound(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectExec("DELETE FROM acl_permissions").WithArgs(anyArgs(2)...).WillReturnResult(pgxmock.NewResult("DELETE", 0))
	r := reqWithParam("DELETE", "/permissions/missing", "id", "missing", nil)
	w := httptest.NewRecorder()
	h.DeletePermission(w, r)
	if w.Code != 404 {
		t.Errorf("status = %d; want 404", w.Code)
	}
}

// =============================================================================
// Role Permissions
// =============================================================================

func TestListRolePermissions_Success(t *testing.T) {
	h, mock := newMockHandlers(t)
	now := time.Now()
	rows := mock.NewRows(permCols()).AddRow("p1", "primary", "users", "read", (*string)(nil), []byte("{}"), now)
	mock.ExpectQuery("FROM acl_permissions p").WithArgs(anyArgs(1)...).WillReturnRows(rows)
	r := reqWithParam("GET", "/roles/r1/permissions", "id", "r1", nil)
	w := httptest.NewRecorder()
	h.ListRolePermissions(w, r)
	if w.Code != 200 {
		t.Fatalf("status = %d; body=%s", w.Code, w.Body.String())
	}
}

func TestAssignPermissionToRole_Success(t *testing.T) {
	h, mock := newMockHandlers(t)
	now := time.Now()
	rows := mock.NewRows([]string{"id", "source_account_id", "role_id", "permission_id", "granted", "conditions", "created_at"}).
		AddRow("rp1", "primary", "r1", "p1", true, []byte("{}"), now)
	mock.ExpectQuery("INSERT INTO acl_role_permissions").WithArgs(anyArgs(5)...).WillReturnRows(rows)

	r := reqWithParam("POST", "/roles/r1/permissions", "id", "r1", strings.NewReader(`{"permission_id":"p1"}`))
	w := httptest.NewRecorder()
	h.AssignPermissionToRole(w, r)
	if w.Code != 201 {
		t.Fatalf("status = %d; body=%s", w.Code, w.Body.String())
	}
}

func TestRemovePermissionFromRole_NotFound(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectExec("DELETE FROM acl_role_permissions").WithArgs(anyArgs(2)...).WillReturnResult(pgxmock.NewResult("DELETE", 0))
	r := reqWithParam("DELETE", "/roles/r1/permissions/p1", "id", "r1", nil)
	addParam(r, "permission_id", "p1")
	w := httptest.NewRecorder()
	h.RemovePermissionFromRole(w, r)
	if w.Code != 404 {
		t.Errorf("status = %d; want 404", w.Code)
	}
}

// =============================================================================
// User roles
// =============================================================================

func TestGetUserRoles_Success(t *testing.T) {
	h, mock := newMockHandlers(t)
	now := time.Now()
	rows := mock.NewRows(roleCols()).
		AddRow("r1", "primary", "admin", (*string)(nil), (*string)(nil), (*string)(nil), 0, true, []byte("{}"), now, now)
	mock.ExpectQuery("FROM acl_roles r").WithArgs(anyArgs(2)...).WillReturnRows(rows)
	r := reqWithParam("GET", "/users/u1/roles", "user_id", "u1", nil)
	w := httptest.NewRecorder()
	h.GetUserRoles(w, r)
	if w.Code != 200 {
		t.Fatalf("status = %d; body=%s", w.Code, w.Body.String())
	}
}

func TestAssignRoleToUser_Success(t *testing.T) {
	h, mock := newMockHandlers(t)
	now := time.Now()
	rows := mock.NewRows([]string{"id", "source_account_id", "user_id", "role_id", "granted_by", "expires_at", "scope", "scope_id", "created_at"}).
		AddRow("ur1", "primary", "u1", "r1", (*string)(nil), (*time.Time)(nil), (*string)(nil), (*string)(nil), now)
	mock.ExpectQuery("INSERT INTO acl_user_roles").WithArgs(anyArgs(7)...).WillReturnRows(rows)

	r := reqWithParam("POST", "/users/u1/roles", "user_id", "u1", strings.NewReader(`{"role_id":"r1"}`))
	w := httptest.NewRecorder()
	h.AssignRoleToUser(w, r)
	if w.Code != 201 {
		t.Fatalf("status = %d; body=%s", w.Code, w.Body.String())
	}
}

func TestRemoveRoleFromUser_Success(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectExec("DELETE FROM acl_user_roles").WithArgs(anyArgs(3)...).WillReturnResult(pgxmock.NewResult("DELETE", 1))
	r := reqWithParam("DELETE", "/users/u1/roles/r1", "user_id", "u1", nil)
	addParam(r, "role_id", "r1")
	w := httptest.NewRecorder()
	h.RemoveRoleFromUser(w, r)
	if w.Code != 200 {
		t.Fatalf("status = %d; body=%s", w.Code, w.Body.String())
	}
}

// =============================================================================
// Policies
// =============================================================================

func policyCols() []string {
	return []string{"id", "source_account_id", "name", "description", "effect", "principal_type",
		"principal_value", "resource_pattern", "action_pattern", "conditions", "priority", "enabled",
		"created_at", "updated_at"}
}

func TestListPolicies_Success(t *testing.T) {
	h, mock := newMockHandlers(t)
	now := time.Now()
	rows := mock.NewRows(policyCols()).
		AddRow("pol1", "primary", "allow-admin", (*string)(nil), "allow", "role", "admin", "*", "*", []byte("{}"), 0, true, now, now)
	mock.ExpectQuery("SELECT id, source_account_id, name, description, effect").WithArgs(anyArgs(1)...).WillReturnRows(rows)
	r := httptest.NewRequest("GET", "/policies", nil)
	w := httptest.NewRecorder()
	h.ListPolicies(w, r)
	if w.Code != 200 {
		t.Fatalf("status = %d; body=%s", w.Code, w.Body.String())
	}
}

func TestCreatePolicy_Success(t *testing.T) {
	h, mock := newMockHandlers(t)
	now := time.Now()
	rows := mock.NewRows(policyCols()).
		AddRow("pol1", "primary", "allow-admin", (*string)(nil), "allow", "role", "admin", "*", "*", []byte("{}"), 0, true, now, now)
	mock.ExpectQuery("INSERT INTO acl_policies").WithArgs(anyArgs(11)...).WillReturnRows(rows)

	r := httptest.NewRequest("POST", "/policies", strings.NewReader(`{"name":"allow-admin","effect":"allow","principal_type":"role"}`))
	w := httptest.NewRecorder()
	h.CreatePolicy(w, r)
	if w.Code != 201 {
		t.Fatalf("status = %d; body=%s", w.Code, w.Body.String())
	}
}

func TestGetPolicy_NotFound(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("SELECT id, source_account_id, name, description, effect").WithArgs(anyArgs(2)...).WillReturnRows(mock.NewRows(policyCols()))
	r := reqWithParam("GET", "/policies/missing", "id", "missing", nil)
	w := httptest.NewRecorder()
	h.GetPolicy(w, r)
	if w.Code != 404 {
		t.Errorf("status = %d; want 404", w.Code)
	}
}

func TestUpdatePolicy_Success(t *testing.T) {
	h, mock := newMockHandlers(t)
	now := time.Now()
	rows := mock.NewRows(policyCols()).
		AddRow("pol1", "primary", "renamed", (*string)(nil), "allow", "role", "admin", "*", "*", []byte("{}"), 0, true, now, now)
	mock.ExpectQuery("UPDATE acl_policies SET").WithArgs(anyArgs(3)...).WillReturnRows(rows)

	r := reqWithParam("PUT", "/policies/pol1", "id", "pol1", strings.NewReader(`{"name":"renamed"}`))
	w := httptest.NewRecorder()
	h.UpdatePolicy(w, r)
	if w.Code != 200 {
		t.Fatalf("status = %d; body=%s", w.Code, w.Body.String())
	}
}

func TestDeletePolicy_Success(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectExec("DELETE FROM acl_policies").WithArgs(anyArgs(2)...).WillReturnResult(pgxmock.NewResult("DELETE", 1))
	r := reqWithParam("DELETE", "/policies/pol1", "id", "pol1", nil)
	w := httptest.NewRecorder()
	h.DeletePolicy(w, r)
	if w.Code != 200 {
		t.Fatalf("status = %d; body=%s", w.Code, w.Body.String())
	}
}

// =============================================================================
// CheckAccess — cache-miss path exercising getUserPermissions/getUserRoleIDs/
// getApplicablePolicies against the mock DB.
// =============================================================================

func TestCheckAccess_CacheMiss_FullDBPath_Allow(t *testing.T) {
	h, mock := newMockHandlers(t)

	permRows := mock.NewRows(permCols()).AddRow("p1", "primary", "users", "read", (*string)(nil), []byte("{}"), time.Now())
	mock.ExpectQuery("WITH RECURSIVE role_tree").WithArgs(anyArgs(2)...).WillReturnRows(permRows)

	roleIDRows := mock.NewRows([]string{"role_id"}).AddRow("r1")
	mock.ExpectQuery("SELECT role_id FROM acl_user_roles").WithArgs(anyArgs(2)...).WillReturnRows(roleIDRows)

	r := httptest.NewRequest("POST", "/check", strings.NewReader(`{"user_id":"u1","resource":"users","action":"read"}`))
	w := httptest.NewRecorder()
	h.CheckAccess(w, r)
	if w.Code != 200 {
		t.Fatalf("status = %d; body=%s", w.Code, w.Body.String())
	}
	var out CheckAccessResult
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if !out.Allowed {
		t.Errorf("expected allowed=true, got %+v", out)
	}
}

func TestCheckAccess_CacheMiss_FullDBPath_PolicyDeny(t *testing.T) {
	h, mock := newMockHandlers(t)

	mock.ExpectQuery("WITH RECURSIVE role_tree").WithArgs(anyArgs(2)...).WillReturnRows(mock.NewRows(permCols()))
	mock.ExpectQuery("SELECT role_id FROM acl_user_roles").WithArgs(anyArgs(2)...).WillReturnRows(mock.NewRows([]string{"role_id"}))

	policyRows := mock.NewRows(policyCols()).
		AddRow("pol1", "primary", "deny-all", (*string)(nil), "deny", "user", "u1", "*", "*", []byte("{}"), 10, true, time.Now(), time.Now())
	mock.ExpectQuery("FROM acl_policies").WithArgs(anyArgs(3)...).WillReturnRows(policyRows)

	r := httptest.NewRequest("POST", "/check", strings.NewReader(`{"user_id":"u1","resource":"users","action":"read"}`))
	w := httptest.NewRecorder()
	h.CheckAccess(w, r)
	if w.Code != 200 {
		t.Fatalf("status = %d; body=%s", w.Code, w.Body.String())
	}
	var out CheckAccessResult
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Allowed {
		t.Errorf("expected allowed=false for deny policy, got %+v", out)
	}
}

func TestCheckAccess_CacheMiss_DBError(t *testing.T) {
	h, mock := newMockHandlers(t)
	mock.ExpectQuery("WITH RECURSIVE role_tree").WithArgs(anyArgs(2)...).WillReturnError(errors.New("db down"))

	r := httptest.NewRequest("POST", "/check", strings.NewReader(`{"user_id":"u1","resource":"users","action":"read"}`))
	w := httptest.NewRecorder()
	h.CheckAccess(w, r)
	if w.Code != 500 {
		t.Errorf("status = %d; want 500", w.Code)
	}
}
