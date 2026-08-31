// Purpose: Cover handler validation paths, permCache, matchPattern, and the
// cache-hit branch of checkAccess without requiring a live database.
// Inputs: httptest requests, in-memory permCache.
// Outputs: asserted HTTP status codes / bodies / pure-function results.
// Constraints: No DB access — only paths reachable before a Pool call.
package internal

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// =============================================================================
// matchPattern — pure function, exhaustive table
// =============================================================================

func TestMatchPattern(t *testing.T) {
	cases := []struct {
		name    string
		pattern string
		value   string
		want    bool
	}{
		{"wildcard matches anything", "*", "users.read", true},
		{"exact match", "users.read", "users.read", true},
		{"exact mismatch", "users.read", "users.write", false},
		{"prefix glob matches", "users.*", "users.read", true},
		{"prefix glob mismatch", "users.*", "posts.read", false},
		{"suffix glob matches", "*.read", "users.read", true},
		{"mid glob matches", "users.*.read", "users.posts.read", true},
		{"empty pattern empty value", "", "", true},
		{"empty pattern nonempty value", "", "x", false},
		{"glob with regex-special chars", "users.[read]", "users.[read]", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := matchPattern(tc.pattern, tc.value); got != tc.want {
				t.Errorf("matchPattern(%q, %q) = %v; want %v", tc.pattern, tc.value, got, tc.want)
			}
		})
	}
}

// =============================================================================
// permCache — get/set/invalidate/clear/expiry
// =============================================================================

func TestPermCache_SetGet(t *testing.T) {
	c := newPermCache(time.Minute)
	perms := []Permission{{ID: "p1", Resource: "users", Action: "read"}}
	roleIDs := []string{"r1"}
	c.set("k1", perms, roleIDs)

	got, ok := c.get("k1")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if len(got.permissions) != 1 || got.permissions[0].ID != "p1" {
		t.Errorf("permissions mismatch: %+v", got.permissions)
	}
	if len(got.roleIDs) != 1 || got.roleIDs[0] != "r1" {
		t.Errorf("roleIDs mismatch: %+v", got.roleIDs)
	}
}

func TestPermCache_MissForUnknownKey(t *testing.T) {
	c := newPermCache(time.Minute)
	if _, ok := c.get("missing"); ok {
		t.Error("expected cache miss for unknown key")
	}
}

func TestPermCache_ExpiredEntryIsMiss(t *testing.T) {
	c := newPermCache(-time.Second) // already expired
	c.set("k1", nil, nil)
	if _, ok := c.get("k1"); ok {
		t.Error("expected expired entry to miss")
	}
}

func TestPermCache_Invalidate(t *testing.T) {
	c := newPermCache(time.Minute)
	c.set("k1", nil, nil)
	c.invalidate("k1")
	if _, ok := c.get("k1"); ok {
		t.Error("expected invalidated key to miss")
	}
}

func TestPermCache_Clear(t *testing.T) {
	c := newPermCache(time.Minute)
	c.set("k1", nil, nil)
	c.set("k2", nil, nil)
	c.clear()
	if _, ok := c.get("k1"); ok {
		t.Error("expected k1 gone after clear")
	}
	if _, ok := c.get("k2"); ok {
		t.Error("expected k2 gone after clear")
	}
}

// =============================================================================
// decodeBody
// =============================================================================

func TestDecodeBody_Valid(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{"name":"x"}`))
	var dst CreateRoleInput
	if err := decodeBody(r, &dst); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dst.Name != "x" {
		t.Errorf("Name = %q", dst.Name)
	}
}

func TestDecodeBody_Invalid(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`not-json`))
	var dst CreateRoleInput
	if err := decodeBody(r, &dst); err == nil {
		t.Error("expected decode error")
	}
}

// =============================================================================
// Handlers — validation paths (no DB access)
// =============================================================================

func TestCreateRole_InvalidJSON(t *testing.T) {
	h := &Handlers{}
	r := httptest.NewRequest(http.MethodPost, "/roles", strings.NewReader("{"))
	w := httptest.NewRecorder()
	h.CreateRole(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d; want 400", w.Code)
	}
}

func TestCreateRole_MissingName(t *testing.T) {
	h := &Handlers{}
	body, _ := json.Marshal(map[string]any{"display_name": "x"})
	r := httptest.NewRequest(http.MethodPost, "/roles", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.CreateRole(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d; want 400", w.Code)
	}
	if !strings.Contains(w.Body.String(), "name is required") {
		t.Errorf("body = %s", w.Body.String())
	}
}

func TestUpdateRole_InvalidJSON(t *testing.T) {
	h := &Handlers{}
	r := httptest.NewRequest(http.MethodPut, "/roles/1", strings.NewReader("{"))
	w := httptest.NewRecorder()
	h.UpdateRole(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d; want 400", w.Code)
	}
}

func TestCreatePermission_InvalidJSON(t *testing.T) {
	h := &Handlers{}
	r := httptest.NewRequest(http.MethodPost, "/permissions", strings.NewReader("{"))
	w := httptest.NewRecorder()
	h.CreatePermission(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d; want 400", w.Code)
	}
}

func TestCreatePermission_MissingFields(t *testing.T) {
	h := &Handlers{}
	cases := []map[string]any{
		{"action": "read"},
		{"resource": "users"},
		{},
	}
	for i, body := range cases {
		buf, _ := json.Marshal(body)
		r := httptest.NewRequest(http.MethodPost, "/permissions", bytes.NewReader(buf))
		w := httptest.NewRecorder()
		h.CreatePermission(w, r)
		if w.Code != http.StatusBadRequest {
			t.Errorf("case %d: status = %d; want 400", i, w.Code)
		}
	}
}

func TestUpdatePermission_InvalidJSON(t *testing.T) {
	h := &Handlers{}
	r := httptest.NewRequest(http.MethodPut, "/permissions/1", strings.NewReader("{"))
	w := httptest.NewRecorder()
	h.UpdatePermission(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d; want 400", w.Code)
	}
}

func TestAssignPermissionToRole_InvalidJSON(t *testing.T) {
	h := &Handlers{}
	r := httptest.NewRequest(http.MethodPost, "/roles/1/permissions", strings.NewReader("{"))
	w := httptest.NewRecorder()
	h.AssignPermissionToRole(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d; want 400", w.Code)
	}
}

func TestAssignRoleToUser_InvalidJSON(t *testing.T) {
	h := &Handlers{}
	r := httptest.NewRequest(http.MethodPost, "/users/u1/roles", strings.NewReader("{"))
	w := httptest.NewRecorder()
	h.AssignRoleToUser(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d; want 400", w.Code)
	}
}

func TestAssignRoleToUser_MissingRoleID(t *testing.T) {
	h := &Handlers{}
	body, _ := json.Marshal(map[string]any{})
	r := httptest.NewRequest(http.MethodPost, "/users/u1/roles", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.AssignRoleToUser(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d; want 400", w.Code)
	}
	if !strings.Contains(w.Body.String(), "role_id is required") {
		t.Errorf("body = %s", w.Body.String())
	}
}

func TestCreatePolicy_InvalidJSON(t *testing.T) {
	h := &Handlers{}
	r := httptest.NewRequest(http.MethodPost, "/policies", strings.NewReader("{"))
	w := httptest.NewRecorder()
	h.CreatePolicy(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d; want 400", w.Code)
	}
}

func TestCreatePolicy_MissingFields(t *testing.T) {
	h := &Handlers{}
	cases := []map[string]any{
		{"effect": "allow", "principal_type": "user"},
		{"name": "p1", "principal_type": "user"},
		{"name": "p1", "effect": "allow"},
	}
	for i, body := range cases {
		buf, _ := json.Marshal(body)
		r := httptest.NewRequest(http.MethodPost, "/policies", bytes.NewReader(buf))
		w := httptest.NewRecorder()
		h.CreatePolicy(w, r)
		if w.Code != http.StatusBadRequest {
			t.Errorf("case %d: status = %d; want 400", i, w.Code)
		}
	}
}

func TestUpdatePolicy_InvalidJSON(t *testing.T) {
	h := &Handlers{}
	r := httptest.NewRequest(http.MethodPut, "/policies/1", strings.NewReader("{"))
	w := httptest.NewRecorder()
	h.UpdatePolicy(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d; want 400", w.Code)
	}
}

func TestCheckAccess_InvalidJSON(t *testing.T) {
	h := &Handlers{}
	r := httptest.NewRequest(http.MethodPost, "/check", strings.NewReader("{"))
	w := httptest.NewRecorder()
	h.CheckAccess(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d; want 400", w.Code)
	}
}

func TestCheckAccess_MissingFields(t *testing.T) {
	h := &Handlers{}
	cases := []map[string]any{
		{"resource": "r", "action": "a"},
		{"user_id": "u", "action": "a"},
		{"user_id": "u", "resource": "r"},
	}
	for i, body := range cases {
		buf, _ := json.Marshal(body)
		r := httptest.NewRequest(http.MethodPost, "/check", bytes.NewReader(buf))
		w := httptest.NewRecorder()
		h.CheckAccess(w, r)
		if w.Code != http.StatusBadRequest {
			t.Errorf("case %d: status = %d; want 400", i, w.Code)
		}
	}
}

// =============================================================================
// checkAccess — cache-hit branches exercise real RBAC/ABAC/default logic
// without touching the DB.
// =============================================================================

func newHandlersWithCache(ttl time.Duration, cfg Config) *Handlers {
	return &Handlers{Cache: newPermCache(ttl), Cfg: cfg}
}

func TestCheckAccess_CacheHit_RBACAllows(t *testing.T) {
	h := newHandlersWithCache(time.Minute, Config{DefaultDeny: true})
	h.Cache.set("acct:u1", []Permission{{ID: "p1", Resource: "users", Action: "read"}}, nil)

	// RBAC matches on the first permission, so checkAccess returns before
	// ever reaching getApplicablePolicies (which would need a live DB).
	res, err := h.checkAccess(context.Background(), "acct", CheckAccessInput{UserID: "u1", Resource: "users", Action: "read"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Allowed {
		t.Error("expected RBAC permission to allow access")
	}
	if res.MatchedPermission == nil || *res.MatchedPermission != "p1" {
		t.Errorf("MatchedPermission = %v", res.MatchedPermission)
	}
	if res.Reason != "RBAC permission granted" {
		t.Errorf("Reason = %q", res.Reason)
	}
}

func TestCheckAccess_CacheHit_WildcardRBAC(t *testing.T) {
	h := newHandlersWithCache(time.Minute, Config{DefaultDeny: true})
	h.Cache.set("acct:u1", []Permission{{ID: "p1", Resource: "*", Action: "*"}}, nil)

	res, err := h.checkAccess(context.Background(), "acct", CheckAccessInput{UserID: "u1", Resource: "anything", Action: "read"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Allowed {
		t.Error("expected wildcard permission to allow access")
	}
}
