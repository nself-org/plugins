package internal

import (
	"encoding/json"
	"time"
)

// Role represents a row in acl_roles.
type Role struct {
	ID              string          `json:"id"`
	SourceAccountID string          `json:"source_account_id"`
	Name            string          `json:"name"`
	DisplayName     *string         `json:"display_name"`
	Description     *string         `json:"description"`
	ParentRoleID    *string         `json:"parent_role_id"`
	Level           int             `json:"level"`
	IsSystem        bool            `json:"is_system"`
	Metadata        json.RawMessage `json:"metadata"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

// Permission represents a row in acl_permissions.
type Permission struct {
	ID              string          `json:"id"`
	SourceAccountID string          `json:"source_account_id"`
	Resource        string          `json:"resource"`
	Action          string          `json:"action"`
	Description     *string         `json:"description"`
	Conditions      json.RawMessage `json:"conditions"`
	CreatedAt       time.Time       `json:"created_at"`
}

// RolePermission represents a row in acl_role_permissions.
type RolePermission struct {
	ID              string          `json:"id"`
	SourceAccountID string          `json:"source_account_id"`
	RoleID          string          `json:"role_id"`
	PermissionID    string          `json:"permission_id"`
	Granted         bool            `json:"granted"`
	Conditions      json.RawMessage `json:"conditions"`
	CreatedAt       time.Time       `json:"created_at"`
}

// UserRole represents a row in acl_user_roles.
type UserRole struct {
	ID              string     `json:"id"`
	SourceAccountID string     `json:"source_account_id"`
	UserID          string     `json:"user_id"`
	RoleID          string     `json:"role_id"`
	GrantedBy       *string    `json:"granted_by"`
	ExpiresAt       *time.Time `json:"expires_at"`
	Scope           *string    `json:"scope"`
	ScopeID         *string    `json:"scope_id"`
	CreatedAt       time.Time  `json:"created_at"`
}

// Policy represents a row in acl_policies.
type Policy struct {
	ID              string          `json:"id"`
	SourceAccountID string          `json:"source_account_id"`
	Name            string          `json:"name"`
	Description     *string         `json:"description"`
	Effect          string          `json:"effect"`
	PrincipalType   string          `json:"principal_type"`
	PrincipalValue  string          `json:"principal_value"`
	ResourcePattern string          `json:"resource_pattern"`
	ActionPattern   string          `json:"action_pattern"`
	Conditions      json.RawMessage `json:"conditions"`
	Priority        int             `json:"priority"`
	Enabled         bool            `json:"enabled"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

// --- Request / response payloads ---

type CreateRoleInput struct {
	Name         string          `json:"name"`
	DisplayName  *string         `json:"display_name"`
	Description  *string         `json:"description"`
	ParentRoleID *string         `json:"parent_role_id"`
	IsSystem     bool            `json:"is_system"`
	Metadata     json.RawMessage `json:"metadata"`
}

type UpdateRoleInput struct {
	DisplayName  *string         `json:"display_name"`
	Description  *string         `json:"description"`
	ParentRoleID *string         `json:"parent_role_id"`
	Metadata     json.RawMessage `json:"metadata"`
}

type CreatePermissionInput struct {
	Resource    string          `json:"resource"`
	Action      string          `json:"action"`
	Description *string         `json:"description"`
	Conditions  json.RawMessage `json:"conditions"`
}

type UpdatePermissionInput struct {
	Description *string         `json:"description"`
	Conditions  json.RawMessage `json:"conditions"`
}

type AssignPermissionInput struct {
	PermissionID string          `json:"permission_id"`
	Granted      *bool           `json:"granted"`
	Conditions   json.RawMessage `json:"conditions"`
}

type AssignUserRoleInput struct {
	RoleID    string     `json:"role_id"`
	GrantedBy *string    `json:"granted_by"`
	ExpiresAt *time.Time `json:"expires_at"`
	Scope     *string    `json:"scope"`
	ScopeID   *string    `json:"scope_id"`
}

type CreatePolicyInput struct {
	Name            string          `json:"name"`
	Description     *string         `json:"description"`
	Effect          string          `json:"effect"`
	PrincipalType   string          `json:"principal_type"`
	PrincipalValue  string          `json:"principal_value"`
	ResourcePattern string          `json:"resource_pattern"`
	ActionPattern   string          `json:"action_pattern"`
	Conditions      json.RawMessage `json:"conditions"`
	Priority        *int            `json:"priority"`
	Enabled         *bool           `json:"enabled"`
}

type UpdatePolicyInput struct {
	Name            *string         `json:"name"`
	Description     *string         `json:"description"`
	Effect          *string         `json:"effect"`
	PrincipalValue  *string         `json:"principal_value"`
	ResourcePattern *string         `json:"resource_pattern"`
	ActionPattern   *string         `json:"action_pattern"`
	Conditions      json.RawMessage `json:"conditions"`
	Priority        *int            `json:"priority"`
	Enabled         *bool           `json:"enabled"`
}

type CheckAccessInput struct {
	UserID   string `json:"user_id"`
	Resource string `json:"resource"`
	Action   string `json:"action"`
}

type CheckAccessResult struct {
	Allowed           bool    `json:"allowed"`
	Reason            string  `json:"reason"`
	MatchedRole       *string `json:"matched_role"`
	MatchedPermission *string `json:"matched_permission"`
}
