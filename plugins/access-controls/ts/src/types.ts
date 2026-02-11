/**
 * Access Controls Plugin Types
 * Complete type definitions for RBAC + ABAC system
 */

export interface ACLPluginConfig {
  port: number;
  host: string;
  cacheTtlSeconds: number;
  maxRoleDepth: number;
  defaultDeny: boolean;
  database: {
    host: string;
    port: number;
    database: string;
    user: string;
    password: string;
    ssl?: boolean;
  };
}

// =============================================================================
// Role Types
// =============================================================================

export interface RoleRecord extends Record<string, unknown> {
  id: string;
  source_account_id: string;
  name: string;
  display_name: string | null;
  description: string | null;
  parent_role_id: string | null;
  level: number;
  is_system: boolean;
  metadata: Record<string, unknown>;
  created_at: Date;
  updated_at: Date;
}

export interface CreateRoleInput {
  name: string;
  display_name?: string;
  description?: string;
  parent_role_id?: string;
  is_system?: boolean;
  metadata?: Record<string, unknown>;
}

export interface UpdateRoleInput {
  display_name?: string;
  description?: string;
  parent_role_id?: string;
  metadata?: Record<string, unknown>;
}

// =============================================================================
// Permission Types
// =============================================================================

export interface PermissionRecord extends Record<string, unknown> {
  id: string;
  source_account_id: string;
  resource: string;
  action: string;
  description: string | null;
  conditions: Record<string, unknown>;
  created_at: Date;
}

export interface CreatePermissionInput {
  resource: string;
  action: string;
  description?: string;
  conditions?: Record<string, unknown>;
}

// =============================================================================
// Role Permission Mapping Types
// =============================================================================

export interface RolePermissionRecord extends Record<string, unknown> {
  id: string;
  source_account_id: string;
  role_id: string;
  permission_id: string;
  granted: boolean;
  conditions: Record<string, unknown>;
  created_at: Date;
}

export interface AssignPermissionInput {
  permission_id: string;
  granted?: boolean;
  conditions?: Record<string, unknown>;
}

// =============================================================================
// User Role Types
// =============================================================================

export interface UserRoleRecord extends Record<string, unknown> {
  id: string;
  source_account_id: string;
  user_id: string;
  role_id: string;
  granted_by: string | null;
  expires_at: Date | null;
  scope: string | null;
  scope_id: string | null;
  created_at: Date;
}

export interface AssignUserRoleInput {
  role_id: string;
  granted_by?: string;
  expires_at?: Date;
  scope?: string;
  scope_id?: string;
}

// =============================================================================
// Policy Types (ABAC)
// =============================================================================

export type PolicyEffect = 'allow' | 'deny';
export type PrincipalType = 'role' | 'user' | 'group';

export interface PolicyRecord extends Record<string, unknown> {
  id: string;
  source_account_id: string;
  name: string;
  description: string | null;
  effect: PolicyEffect;
  principal_type: PrincipalType;
  principal_value: string;
  resource_pattern: string;
  action_pattern: string;
  conditions: Record<string, unknown>;
  priority: number;
  enabled: boolean;
  created_at: Date;
  updated_at: Date;
}

export interface CreatePolicyInput {
  name: string;
  description?: string;
  effect: PolicyEffect;
  principal_type: PrincipalType;
  principal_value: string;
  resource_pattern: string;
  action_pattern: string;
  conditions?: Record<string, unknown>;
  priority?: number;
  enabled?: boolean;
}

export interface UpdatePolicyInput {
  name?: string;
  description?: string;
  effect?: PolicyEffect;
  principal_value?: string;
  resource_pattern?: string;
  action_pattern?: string;
  conditions?: Record<string, unknown>;
  priority?: number;
  enabled?: boolean;
}

// =============================================================================
// Authorization Types
// =============================================================================

export interface AuthorizationRequest {
  user_id: string;
  resource: string;
  action: string;
  context?: Record<string, unknown>;
}

export interface AuthorizationResult {
  allowed: boolean;
  reason: string;
  matched_permissions?: string[];
  matched_policies?: string[];
  cached?: boolean;
}

export interface BatchAuthorizationRequest {
  requests: AuthorizationRequest[];
}

export interface BatchAuthorizationResult {
  results: (AuthorizationResult & { request: AuthorizationRequest })[];
}

// =============================================================================
// Effective Permissions (Cache)
// =============================================================================

export interface EffectivePermission {
  resource: string;
  action: string;
  granted: boolean;
  source: 'role' | 'policy';
  source_id: string;
  conditions: Record<string, unknown>;
}

export interface UserPermissionsCache {
  user_id: string;
  source_account_id: string;
  permissions: EffectivePermission[];
  roles: string[];
  cached_at: Date;
  expires_at: Date;
}

// =============================================================================
// Webhook Event Types
// =============================================================================

export interface WebhookEventRecord extends Record<string, unknown> {
  id: string;
  source_account_id: string;
  event_type: string;
  payload: Record<string, unknown>;
  processed: boolean;
  processed_at: Date | null;
  error: string | null;
  created_at: Date;
}

// =============================================================================
// Stats Types
// =============================================================================

export interface ACLStats {
  roles: number;
  permissions: number;
  role_permissions: number;
  user_roles: number;
  policies: number;
  webhook_events?: number;
}

// =============================================================================
// Role Hierarchy Types
// =============================================================================

export interface RoleHierarchyNode {
  role: RoleRecord;
  children: RoleHierarchyNode[];
  permissions: PermissionRecord[];
}

// =============================================================================
// Pattern Matching Types
// =============================================================================

export interface PatternMatch {
  pattern: string;
  value: string;
  matched: boolean;
}
