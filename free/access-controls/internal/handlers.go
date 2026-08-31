package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

// --- helpers ---

func sa(r *http.Request) string {
	if v := r.Header.Get("X-Source-Account-Id"); v != "" {
		return v
	}
	return "primary"
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func errJSON(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func decodeBody(r *http.Request, dst any) error {
	return json.NewDecoder(r.Body).Decode(dst)
}

// rawJSON normalises an incoming json.RawMessage: returns '{}' when nil/empty.
func rawJSON(b json.RawMessage) json.RawMessage {
	if len(b) == 0 {
		return json.RawMessage("{}")
	}
	return b
}

// boolPtr returns a pointer to a bool.
func boolPtr(b bool) *bool { return &b }

// --- in-memory permission cache ---

type cachedPerms struct {
	permissions []Permission
	roleIDs     []string
	expiresAt   time.Time
}

type permCache struct {
	mu    sync.Mutex
	items map[string]*cachedPerms
	ttl   time.Duration
}

func newPermCache(ttl time.Duration) *permCache {
	return &permCache{items: make(map[string]*cachedPerms), ttl: ttl}
}

func (c *permCache) get(key string) (*cachedPerms, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	item, ok := c.items[key]
	if !ok || time.Now().After(item.expiresAt) {
		return nil, false
	}
	return item, true
}

func (c *permCache) set(key string, perms []Permission, roleIDs []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items[key] = &cachedPerms{
		permissions: perms,
		roleIDs:     roleIDs,
		expiresAt:   time.Now().Add(c.ttl),
	}
}

func (c *permCache) invalidate(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.items, key)
}

func (c *permCache) clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items = make(map[string]*cachedPerms)
}

// --- Handlers struct ---

// Handlers holds the database and cache needed by all HTTP handlers.
type Handlers struct {
	DB    *DB
	Cache *permCache
	Cfg   Config
}

// NewHandlers constructs a Handlers instance.
func NewHandlers(db *DB, cfg Config) *Handlers {
	return &Handlers{
		DB:    db,
		Cache: newPermCache(time.Duration(cfg.CacheTTLSeconds) * time.Second),
		Cfg:   cfg,
	}
}

// =============================================================================
// Health
// =============================================================================

func (h *Handlers) Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status":    "ok",
		"plugin":    "access-controls",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

func (h *Handlers) Ready(w http.ResponseWriter, r *http.Request) {
	if _, err := h.DB.Pool.Exec(r.Context(), "SELECT 1"); err != nil {
		errJSON(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"status":    "ready",
		"plugin":    "access-controls",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

// =============================================================================
// Roles
// =============================================================================

func (h *Handlers) ListRoles(w http.ResponseWriter, r *http.Request) {
	account := sa(r)
	rows, err := h.DB.Pool.Query(r.Context(),
		`SELECT id, source_account_id, name, display_name, description,
		        parent_role_id, level, is_system, metadata, created_at, updated_at
		 FROM acl_roles
		 WHERE source_account_id = $1
		 ORDER BY level ASC, name ASC`, account)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	roles := []Role{}
	for rows.Next() {
		var ro Role
		if err := rows.Scan(&ro.ID, &ro.SourceAccountID, &ro.Name, &ro.DisplayName,
			&ro.Description, &ro.ParentRoleID, &ro.Level, &ro.IsSystem,
			&ro.Metadata, &ro.CreatedAt, &ro.UpdatedAt); err != nil {
			errJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		roles = append(roles, ro)
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": roles, "total": len(roles)})
}

func (h *Handlers) CreateRole(w http.ResponseWriter, r *http.Request) {
	var inp CreateRoleInput
	if err := decodeBody(r, &inp); err != nil {
		errJSON(w, http.StatusBadRequest, err.Error())
		return
	}
	if inp.Name == "" {
		errJSON(w, http.StatusBadRequest, "name is required")
		return
	}

	account := sa(r)
	level := 0
	if inp.ParentRoleID != nil {
		var parentLevel int
		err := h.DB.Pool.QueryRow(r.Context(),
			`SELECT level FROM acl_roles WHERE id = $1 AND source_account_id = $2`,
			*inp.ParentRoleID, account).Scan(&parentLevel)
		if err == nil {
			level = parentLevel + 1
		}
	}

	var ro Role
	err := h.DB.Pool.QueryRow(r.Context(),
		`INSERT INTO acl_roles
		 (source_account_id, name, display_name, description, parent_role_id, level, is_system, metadata)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		 RETURNING id, source_account_id, name, display_name, description,
		           parent_role_id, level, is_system, metadata, created_at, updated_at`,
		account, inp.Name, inp.DisplayName, inp.Description,
		inp.ParentRoleID, level, inp.IsSystem, rawJSON(inp.Metadata),
	).Scan(&ro.ID, &ro.SourceAccountID, &ro.Name, &ro.DisplayName, &ro.Description,
		&ro.ParentRoleID, &ro.Level, &ro.IsSystem, &ro.Metadata, &ro.CreatedAt, &ro.UpdatedAt)
	if err != nil {
		errJSON(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, ro)
}

func (h *Handlers) GetRole(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	account := sa(r)
	var ro Role
	err := h.DB.Pool.QueryRow(r.Context(),
		`SELECT id, source_account_id, name, display_name, description,
		        parent_role_id, level, is_system, metadata, created_at, updated_at
		 FROM acl_roles WHERE id = $1 AND source_account_id = $2`, id, account,
	).Scan(&ro.ID, &ro.SourceAccountID, &ro.Name, &ro.DisplayName, &ro.Description,
		&ro.ParentRoleID, &ro.Level, &ro.IsSystem, &ro.Metadata, &ro.CreatedAt, &ro.UpdatedAt)
	if err == pgx.ErrNoRows {
		errJSON(w, http.StatusNotFound, "role not found")
		return
	}
	if err != nil {
		errJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, ro)
}

func (h *Handlers) UpdateRole(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	account := sa(r)
	var inp UpdateRoleInput
	if err := decodeBody(r, &inp); err != nil {
		errJSON(w, http.StatusBadRequest, err.Error())
		return
	}

	sets := []string{"updated_at = NOW()"}
	args := []any{id, account}
	idx := 3

	if inp.DisplayName != nil {
		sets = append(sets, fmt.Sprintf("display_name = $%d", idx))
		args = append(args, *inp.DisplayName)
		idx++
	}
	if inp.Description != nil {
		sets = append(sets, fmt.Sprintf("description = $%d", idx))
		args = append(args, *inp.Description)
		idx++
	}
	if inp.ParentRoleID != nil {
		sets = append(sets, fmt.Sprintf("parent_role_id = $%d", idx))
		args = append(args, *inp.ParentRoleID)
		idx++
	}
	if len(inp.Metadata) > 0 {
		sets = append(sets, fmt.Sprintf("metadata = $%d", idx))
		args = append(args, rawJSON(inp.Metadata))
		idx++
	}

	q := fmt.Sprintf(`UPDATE acl_roles SET %s WHERE id = $1 AND source_account_id = $2
		RETURNING id, source_account_id, name, display_name, description,
		          parent_role_id, level, is_system, metadata, created_at, updated_at`,
		strings.Join(sets, ", "))

	var ro Role
	err := h.DB.Pool.QueryRow(r.Context(), q, args...).Scan(
		&ro.ID, &ro.SourceAccountID, &ro.Name, &ro.DisplayName, &ro.Description,
		&ro.ParentRoleID, &ro.Level, &ro.IsSystem, &ro.Metadata, &ro.CreatedAt, &ro.UpdatedAt)
	if err == pgx.ErrNoRows {
		errJSON(w, http.StatusNotFound, "role not found")
		return
	}
	if err != nil {
		errJSON(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, ro)
}

func (h *Handlers) DeleteRole(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	account := sa(r)
	tag, err := h.DB.Pool.Exec(r.Context(),
		`DELETE FROM acl_roles WHERE id = $1 AND source_account_id = $2`, id, account)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	if tag.RowsAffected() == 0 {
		errJSON(w, http.StatusNotFound, "role not found")
		return
	}
	h.Cache.clear()
	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

// =============================================================================
// Role Permissions
// =============================================================================

func (h *Handlers) ListRolePermissions(w http.ResponseWriter, r *http.Request) {
	roleID := chi.URLParam(r, "id")
	rows, err := h.DB.Pool.Query(r.Context(),
		`SELECT p.id, p.source_account_id, p.resource, p.action, p.description,
		        p.conditions, p.created_at
		 FROM acl_permissions p
		 JOIN acl_role_permissions rp ON rp.permission_id = p.id
		 WHERE rp.role_id = $1 AND rp.granted = true
		 ORDER BY p.resource, p.action`, roleID)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	perms := []Permission{}
	for rows.Next() {
		var p Permission
		if err := rows.Scan(&p.ID, &p.SourceAccountID, &p.Resource, &p.Action,
			&p.Description, &p.Conditions, &p.CreatedAt); err != nil {
			errJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		perms = append(perms, p)
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": perms})
}

func (h *Handlers) AssignPermissionToRole(w http.ResponseWriter, r *http.Request) {
	roleID := chi.URLParam(r, "id")
	account := sa(r)
	var inp AssignPermissionInput
	if err := decodeBody(r, &inp); err != nil {
		errJSON(w, http.StatusBadRequest, err.Error())
		return
	}
	granted := true
	if inp.Granted != nil {
		granted = *inp.Granted
	}

	var rp RolePermission
	err := h.DB.Pool.QueryRow(r.Context(),
		`INSERT INTO acl_role_permissions
		 (source_account_id, role_id, permission_id, granted, conditions)
		 VALUES ($1,$2,$3,$4,$5)
		 ON CONFLICT (role_id, permission_id) DO UPDATE SET granted = EXCLUDED.granted, conditions = EXCLUDED.conditions
		 RETURNING id, source_account_id, role_id, permission_id, granted, conditions, created_at`,
		account, roleID, inp.PermissionID, granted, rawJSON(inp.Conditions),
	).Scan(&rp.ID, &rp.SourceAccountID, &rp.RoleID, &rp.PermissionID,
		&rp.Granted, &rp.Conditions, &rp.CreatedAt)
	if err != nil {
		errJSON(w, http.StatusBadRequest, err.Error())
		return
	}
	h.Cache.clear()
	writeJSON(w, http.StatusCreated, rp)
}

func (h *Handlers) RemovePermissionFromRole(w http.ResponseWriter, r *http.Request) {
	roleID := chi.URLParam(r, "id")
	permID := chi.URLParam(r, "permission_id")
	tag, err := h.DB.Pool.Exec(r.Context(),
		`DELETE FROM acl_role_permissions WHERE role_id = $1 AND permission_id = $2`, roleID, permID)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	if tag.RowsAffected() == 0 {
		errJSON(w, http.StatusNotFound, "mapping not found")
		return
	}
	h.Cache.clear()
	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

// =============================================================================
// Permissions
// =============================================================================

func (h *Handlers) ListPermissions(w http.ResponseWriter, r *http.Request) {
	account := sa(r)
	rows, err := h.DB.Pool.Query(r.Context(),
		`SELECT id, source_account_id, resource, action, description, conditions, created_at
		 FROM acl_permissions
		 WHERE source_account_id = $1
		 ORDER BY resource, action`, account)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	perms := []Permission{}
	for rows.Next() {
		var p Permission
		if err := rows.Scan(&p.ID, &p.SourceAccountID, &p.Resource, &p.Action,
			&p.Description, &p.Conditions, &p.CreatedAt); err != nil {
			errJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		perms = append(perms, p)
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": perms, "total": len(perms)})
}

func (h *Handlers) CreatePermission(w http.ResponseWriter, r *http.Request) {
	account := sa(r)
	var inp CreatePermissionInput
	if err := decodeBody(r, &inp); err != nil {
		errJSON(w, http.StatusBadRequest, err.Error())
		return
	}
	if inp.Resource == "" || inp.Action == "" {
		errJSON(w, http.StatusBadRequest, "resource and action are required")
		return
	}

	var p Permission
	err := h.DB.Pool.QueryRow(r.Context(),
		`INSERT INTO acl_permissions (source_account_id, resource, action, description, conditions)
		 VALUES ($1,$2,$3,$4,$5)
		 ON CONFLICT (source_account_id, resource, action) DO UPDATE
		   SET description = EXCLUDED.description, conditions = EXCLUDED.conditions
		 RETURNING id, source_account_id, resource, action, description, conditions, created_at`,
		account, inp.Resource, inp.Action, inp.Description, rawJSON(inp.Conditions),
	).Scan(&p.ID, &p.SourceAccountID, &p.Resource, &p.Action,
		&p.Description, &p.Conditions, &p.CreatedAt)
	if err != nil {
		errJSON(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

func (h *Handlers) GetPermission(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	account := sa(r)
	var p Permission
	err := h.DB.Pool.QueryRow(r.Context(),
		`SELECT id, source_account_id, resource, action, description, conditions, created_at
		 FROM acl_permissions WHERE id = $1 AND source_account_id = $2`, id, account,
	).Scan(&p.ID, &p.SourceAccountID, &p.Resource, &p.Action,
		&p.Description, &p.Conditions, &p.CreatedAt)
	if err == pgx.ErrNoRows {
		errJSON(w, http.StatusNotFound, "permission not found")
		return
	}
	if err != nil {
		errJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (h *Handlers) UpdatePermission(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	account := sa(r)
	var inp UpdatePermissionInput
	if err := decodeBody(r, &inp); err != nil {
		errJSON(w, http.StatusBadRequest, err.Error())
		return
	}

	sets := []string{}
	args := []any{id, account}
	idx := 3

	if inp.Description != nil {
		sets = append(sets, fmt.Sprintf("description = $%d", idx))
		args = append(args, *inp.Description)
		idx++
	}
	if len(inp.Conditions) > 0 {
		sets = append(sets, fmt.Sprintf("conditions = $%d", idx))
		args = append(args, rawJSON(inp.Conditions))
		idx++
	}
	if len(sets) == 0 {
		h.GetPermission(w, r)
		return
	}

	q := fmt.Sprintf(`UPDATE acl_permissions SET %s WHERE id = $1 AND source_account_id = $2
		RETURNING id, source_account_id, resource, action, description, conditions, created_at`,
		strings.Join(sets, ", "))

	var p Permission
	err := h.DB.Pool.QueryRow(r.Context(), q, args...).Scan(
		&p.ID, &p.SourceAccountID, &p.Resource, &p.Action,
		&p.Description, &p.Conditions, &p.CreatedAt)
	if err == pgx.ErrNoRows {
		errJSON(w, http.StatusNotFound, "permission not found")
		return
	}
	if err != nil {
		errJSON(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (h *Handlers) DeletePermission(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	account := sa(r)
	tag, err := h.DB.Pool.Exec(r.Context(),
		`DELETE FROM acl_permissions WHERE id = $1 AND source_account_id = $2`, id, account)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	if tag.RowsAffected() == 0 {
		errJSON(w, http.StatusNotFound, "permission not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

// =============================================================================
// User Roles
// =============================================================================

func (h *Handlers) GetUserRoles(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "user_id")
	account := sa(r)
	rows, err := h.DB.Pool.Query(r.Context(),
		`SELECT r.id, r.source_account_id, r.name, r.display_name, r.description,
		        r.parent_role_id, r.level, r.is_system, r.metadata, r.created_at, r.updated_at
		 FROM acl_roles r
		 JOIN acl_user_roles ur ON ur.role_id = r.id
		 WHERE ur.user_id = $1 AND ur.source_account_id = $2
		   AND (ur.expires_at IS NULL OR ur.expires_at > NOW())
		 ORDER BY r.level, r.name`, userID, account)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	roles := []Role{}
	for rows.Next() {
		var ro Role
		if err := rows.Scan(&ro.ID, &ro.SourceAccountID, &ro.Name, &ro.DisplayName,
			&ro.Description, &ro.ParentRoleID, &ro.Level, &ro.IsSystem,
			&ro.Metadata, &ro.CreatedAt, &ro.UpdatedAt); err != nil {
			errJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		roles = append(roles, ro)
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": roles})
}

func (h *Handlers) AssignRoleToUser(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "user_id")
	account := sa(r)
	var inp AssignUserRoleInput
	if err := decodeBody(r, &inp); err != nil {
		errJSON(w, http.StatusBadRequest, err.Error())
		return
	}
	if inp.RoleID == "" {
		errJSON(w, http.StatusBadRequest, "role_id is required")
		return
	}

	var ur UserRole
	err := h.DB.Pool.QueryRow(r.Context(),
		`INSERT INTO acl_user_roles
		 (source_account_id, user_id, role_id, granted_by, expires_at, scope, scope_id)
		 VALUES ($1,$2,$3,$4,$5,$6,$7)
		 ON CONFLICT (source_account_id, user_id, role_id, scope, scope_id) DO UPDATE
		   SET granted_by = EXCLUDED.granted_by, expires_at = EXCLUDED.expires_at
		 RETURNING id, source_account_id, user_id, role_id, granted_by, expires_at, scope, scope_id, created_at`,
		account, userID, inp.RoleID, inp.GrantedBy, inp.ExpiresAt, inp.Scope, inp.ScopeID,
	).Scan(&ur.ID, &ur.SourceAccountID, &ur.UserID, &ur.RoleID,
		&ur.GrantedBy, &ur.ExpiresAt, &ur.Scope, &ur.ScopeID, &ur.CreatedAt)
	if err != nil {
		errJSON(w, http.StatusBadRequest, err.Error())
		return
	}
	cacheKey := fmt.Sprintf("%s:%s", account, userID)
	h.Cache.invalidate(cacheKey)
	writeJSON(w, http.StatusCreated, ur)
}

func (h *Handlers) RemoveRoleFromUser(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "user_id")
	roleID := chi.URLParam(r, "role_id")
	account := sa(r)
	tag, err := h.DB.Pool.Exec(r.Context(),
		`DELETE FROM acl_user_roles
		 WHERE source_account_id = $1 AND user_id = $2 AND role_id = $3`,
		account, userID, roleID)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	if tag.RowsAffected() == 0 {
		errJSON(w, http.StatusNotFound, "user role assignment not found")
		return
	}
	cacheKey := fmt.Sprintf("%s:%s", account, userID)
	h.Cache.invalidate(cacheKey)
	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

// =============================================================================
// Policies
// =============================================================================

func (h *Handlers) ListPolicies(w http.ResponseWriter, r *http.Request) {
	account := sa(r)
	rows, err := h.DB.Pool.Query(r.Context(),
		`SELECT id, source_account_id, name, description, effect, principal_type,
		        principal_value, resource_pattern, action_pattern, conditions,
		        priority, enabled, created_at, updated_at
		 FROM acl_policies
		 WHERE source_account_id = $1
		 ORDER BY priority DESC, name`, account)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	policies := []Policy{}
	for rows.Next() {
		var p Policy
		if err := rows.Scan(&p.ID, &p.SourceAccountID, &p.Name, &p.Description,
			&p.Effect, &p.PrincipalType, &p.PrincipalValue,
			&p.ResourcePattern, &p.ActionPattern, &p.Conditions,
			&p.Priority, &p.Enabled, &p.CreatedAt, &p.UpdatedAt); err != nil {
			errJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		policies = append(policies, p)
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": policies, "total": len(policies)})
}

func (h *Handlers) CreatePolicy(w http.ResponseWriter, r *http.Request) {
	account := sa(r)
	var inp CreatePolicyInput
	if err := decodeBody(r, &inp); err != nil {
		errJSON(w, http.StatusBadRequest, err.Error())
		return
	}
	if inp.Name == "" || inp.Effect == "" || inp.PrincipalType == "" {
		errJSON(w, http.StatusBadRequest, "name, effect and principal_type are required")
		return
	}
	priority := 0
	if inp.Priority != nil {
		priority = *inp.Priority
	}
	enabled := true
	if inp.Enabled != nil {
		enabled = *inp.Enabled
	}

	var p Policy
	err := h.DB.Pool.QueryRow(r.Context(),
		`INSERT INTO acl_policies
		 (source_account_id, name, description, effect, principal_type, principal_value,
		  resource_pattern, action_pattern, conditions, priority, enabled)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		 RETURNING id, source_account_id, name, description, effect, principal_type,
		           principal_value, resource_pattern, action_pattern, conditions,
		           priority, enabled, created_at, updated_at`,
		account, inp.Name, inp.Description, inp.Effect, inp.PrincipalType,
		inp.PrincipalValue, inp.ResourcePattern, inp.ActionPattern,
		rawJSON(inp.Conditions), priority, enabled,
	).Scan(&p.ID, &p.SourceAccountID, &p.Name, &p.Description,
		&p.Effect, &p.PrincipalType, &p.PrincipalValue,
		&p.ResourcePattern, &p.ActionPattern, &p.Conditions,
		&p.Priority, &p.Enabled, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		errJSON(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

func (h *Handlers) GetPolicy(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	account := sa(r)
	var p Policy
	err := h.DB.Pool.QueryRow(r.Context(),
		`SELECT id, source_account_id, name, description, effect, principal_type,
		        principal_value, resource_pattern, action_pattern, conditions,
		        priority, enabled, created_at, updated_at
		 FROM acl_policies WHERE id = $1 AND source_account_id = $2`, id, account,
	).Scan(&p.ID, &p.SourceAccountID, &p.Name, &p.Description,
		&p.Effect, &p.PrincipalType, &p.PrincipalValue,
		&p.ResourcePattern, &p.ActionPattern, &p.Conditions,
		&p.Priority, &p.Enabled, &p.CreatedAt, &p.UpdatedAt)
	if err == pgx.ErrNoRows {
		errJSON(w, http.StatusNotFound, "policy not found")
		return
	}
	if err != nil {
		errJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (h *Handlers) UpdatePolicy(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	account := sa(r)
	var inp UpdatePolicyInput
	if err := decodeBody(r, &inp); err != nil {
		errJSON(w, http.StatusBadRequest, err.Error())
		return
	}

	sets := []string{"updated_at = NOW()"}
	args := []any{id, account}
	idx := 3

	if inp.Name != nil {
		sets = append(sets, fmt.Sprintf("name = $%d", idx))
		args = append(args, *inp.Name)
		idx++
	}
	if inp.Description != nil {
		sets = append(sets, fmt.Sprintf("description = $%d", idx))
		args = append(args, *inp.Description)
		idx++
	}
	if inp.Effect != nil {
		sets = append(sets, fmt.Sprintf("effect = $%d", idx))
		args = append(args, *inp.Effect)
		idx++
	}
	if inp.PrincipalValue != nil {
		sets = append(sets, fmt.Sprintf("principal_value = $%d", idx))
		args = append(args, *inp.PrincipalValue)
		idx++
	}
	if inp.ResourcePattern != nil {
		sets = append(sets, fmt.Sprintf("resource_pattern = $%d", idx))
		args = append(args, *inp.ResourcePattern)
		idx++
	}
	if inp.ActionPattern != nil {
		sets = append(sets, fmt.Sprintf("action_pattern = $%d", idx))
		args = append(args, *inp.ActionPattern)
		idx++
	}
	if len(inp.Conditions) > 0 {
		sets = append(sets, fmt.Sprintf("conditions = $%d", idx))
		args = append(args, rawJSON(inp.Conditions))
		idx++
	}
	if inp.Priority != nil {
		sets = append(sets, fmt.Sprintf("priority = $%d", idx))
		args = append(args, *inp.Priority)
		idx++
	}
	if inp.Enabled != nil {
		sets = append(sets, fmt.Sprintf("enabled = $%d", idx))
		args = append(args, *inp.Enabled)
		idx++
	}
	_ = idx

	q := fmt.Sprintf(`UPDATE acl_policies SET %s WHERE id = $1 AND source_account_id = $2
		RETURNING id, source_account_id, name, description, effect, principal_type,
		          principal_value, resource_pattern, action_pattern, conditions,
		          priority, enabled, created_at, updated_at`,
		strings.Join(sets, ", "))

	var p Policy
	err := h.DB.Pool.QueryRow(r.Context(), q, args...).Scan(
		&p.ID, &p.SourceAccountID, &p.Name, &p.Description,
		&p.Effect, &p.PrincipalType, &p.PrincipalValue,
		&p.ResourcePattern, &p.ActionPattern, &p.Conditions,
		&p.Priority, &p.Enabled, &p.CreatedAt, &p.UpdatedAt)
	if err == pgx.ErrNoRows {
		errJSON(w, http.StatusNotFound, "policy not found")
		return
	}
	if err != nil {
		errJSON(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (h *Handlers) DeletePolicy(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	account := sa(r)
	tag, err := h.DB.Pool.Exec(r.Context(),
		`DELETE FROM acl_policies WHERE id = $1 AND source_account_id = $2`, id, account)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	if tag.RowsAffected() == 0 {
		errJSON(w, http.StatusNotFound, "policy not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

// =============================================================================
// Access check
// =============================================================================

func (h *Handlers) CheckAccess(w http.ResponseWriter, r *http.Request) {
	account := sa(r)
	var inp CheckAccessInput
	if err := decodeBody(r, &inp); err != nil {
		errJSON(w, http.StatusBadRequest, err.Error())
		return
	}
	if inp.UserID == "" || inp.Resource == "" || inp.Action == "" {
		errJSON(w, http.StatusBadRequest, "user_id, resource and action are required")
		return
	}

	result, err := h.checkAccess(r.Context(), account, inp)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handlers) checkAccess(ctx context.Context, account string, inp CheckAccessInput) (*CheckAccessResult, error) {
	cacheKey := fmt.Sprintf("%s:%s", account, inp.UserID)

	cached, ok := h.Cache.get(cacheKey)
	if !ok {
		// Fetch user permissions via role hierarchy.
		perms, err := h.getUserPermissions(ctx, account, inp.UserID)
		if err != nil {
			return nil, err
		}
		roleIDs, err := h.getUserRoleIDs(ctx, account, inp.UserID)
		if err != nil {
			return nil, err
		}
		h.Cache.set(cacheKey, perms, roleIDs)
		cached = &cachedPerms{permissions: perms, roleIDs: roleIDs}
	}

	// RBAC check.
	for _, p := range cached.permissions {
		if matchPattern(p.Resource, inp.Resource) && matchPattern(p.Action, inp.Action) {
			return &CheckAccessResult{
				Allowed:           true,
				Reason:            "RBAC permission granted",
				MatchedPermission: &p.ID,
			}, nil
		}
	}

	// ABAC policy check.
	policies, err := h.getApplicablePolicies(ctx, account, inp.UserID, cached.roleIDs)
	if err != nil {
		return nil, err
	}
	for _, pol := range policies {
		if !matchPattern(pol.ResourcePattern, inp.Resource) {
			continue
		}
		if !matchPattern(pol.ActionPattern, inp.Action) {
			continue
		}
		allowed := pol.Effect == "allow"
		reason := fmt.Sprintf(`policy "%s" (%s)`, pol.Name, pol.Effect)
		return &CheckAccessResult{
			Allowed:     allowed,
			Reason:      reason,
			MatchedRole: &pol.ID,
		}, nil
	}

	// Default decision.
	if h.Cfg.DefaultDeny {
		return &CheckAccessResult{
			Allowed: false,
			Reason:  "no matching permissions or policies (default deny)",
		}, nil
	}
	return &CheckAccessResult{Allowed: true, Reason: "default allow"}, nil
}

func (h *Handlers) getUserPermissions(ctx context.Context, account, userID string) ([]Permission, error) {
	rows, err := h.DB.Pool.Query(ctx,
		`WITH RECURSIVE role_tree AS (
		   SELECT r.id, r.parent_role_id FROM acl_roles r
		   JOIN acl_user_roles ur ON ur.role_id = r.id
		   WHERE ur.user_id = $1 AND ur.source_account_id = $2
		     AND (ur.expires_at IS NULL OR ur.expires_at > NOW())
		   UNION
		   SELECT r.id, r.parent_role_id FROM acl_roles r
		   JOIN role_tree rt ON rt.parent_role_id = r.id
		 )
		 SELECT DISTINCT p.id, p.source_account_id, p.resource, p.action,
		        p.description, p.conditions, p.created_at
		 FROM acl_permissions p
		 JOIN acl_role_permissions rp ON rp.permission_id = p.id
		 JOIN role_tree rt ON rt.id = rp.role_id
		 WHERE rp.granted = true`, userID, account)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var perms []Permission
	for rows.Next() {
		var p Permission
		if err := rows.Scan(&p.ID, &p.SourceAccountID, &p.Resource, &p.Action,
			&p.Description, &p.Conditions, &p.CreatedAt); err != nil {
			return nil, err
		}
		perms = append(perms, p)
	}
	return perms, nil
}

func (h *Handlers) getUserRoleIDs(ctx context.Context, account, userID string) ([]string, error) {
	rows, err := h.DB.Pool.Query(ctx,
		`SELECT role_id FROM acl_user_roles
		 WHERE user_id = $1 AND source_account_id = $2
		   AND (expires_at IS NULL OR expires_at > NOW())`, userID, account)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func (h *Handlers) getApplicablePolicies(ctx context.Context, account, userID string, roleIDs []string) ([]Policy, error) {
	rows, err := h.DB.Pool.Query(ctx,
		`SELECT id, source_account_id, name, description, effect, principal_type,
		        principal_value, resource_pattern, action_pattern, conditions,
		        priority, enabled, created_at, updated_at
		 FROM acl_policies
		 WHERE source_account_id = $1
		   AND enabled = true
		   AND (
		     (principal_type = 'user' AND principal_value = $2)
		     OR (principal_type = 'role' AND principal_value = ANY($3))
		   )
		 ORDER BY priority DESC`, account, userID, roleIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var policies []Policy
	for rows.Next() {
		var p Policy
		if err := rows.Scan(&p.ID, &p.SourceAccountID, &p.Name, &p.Description,
			&p.Effect, &p.PrincipalType, &p.PrincipalValue,
			&p.ResourcePattern, &p.ActionPattern, &p.Conditions,
			&p.Priority, &p.Enabled, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		policies = append(policies, p)
	}
	return policies, nil
}

// matchPattern supports exact match and glob-style wildcards using path.Match semantics.
func matchPattern(pattern, value string) bool {
	if pattern == "*" || pattern == value {
		return true
	}
	if strings.Contains(pattern, "*") {
		// Convert glob * to regex .*
		escaped := regexp.QuoteMeta(pattern)
		escaped = strings.ReplaceAll(escaped, `\*`, `.*`)
		if ok, _ := regexp.MatchString("^"+escaped+"$", value); ok {
			return true
		}
		if ok, _ := path.Match(pattern, value); ok {
			return true
		}
	}
	return false
}
