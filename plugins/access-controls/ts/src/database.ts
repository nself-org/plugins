/**
 * Access Controls Database Operations
 * Complete CRUD operations for ACL system in PostgreSQL
 */

import { createDatabase, createLogger, type Database } from '@nself/plugin-utils';
import type {
  RoleRecord,
  PermissionRecord,
  RolePermissionRecord,
  UserRoleRecord,
  PolicyRecord,
  ACLStats,
  CreateRoleInput,
  UpdateRoleInput,
  CreatePermissionInput,
  AssignPermissionInput,
  AssignUserRoleInput,
  CreatePolicyInput,
  UpdatePolicyInput,
  RoleHierarchyNode,
} from './types.js';

const logger = createLogger('acl:db');

export class ACLDatabase {
  private db: Database;
  private readonly sourceAccountId: string;

  constructor(db?: Database, sourceAccountId = 'primary') {
    this.db = db ?? createDatabase();
    this.sourceAccountId = this.normalizeSourceAccountId(sourceAccountId);
  }

  forSourceAccount(sourceAccountId: string): ACLDatabase {
    return new ACLDatabase(this.db, sourceAccountId);
  }

  getCurrentSourceAccountId(): string {
    return this.sourceAccountId;
  }

  private normalizeSourceAccountId(value: string): string {
    const normalized = value
      .toLowerCase()
      .replace(/[^a-z0-9_-]+/g, '-')
      .replace(/^-+|-+$/g, '');

    return normalized.length > 0 ? normalized : 'primary';
  }

  async connect(): Promise<void> {
    await this.db.connect();
  }

  async disconnect(): Promise<void> {
    await this.db.disconnect();
  }

  async query<T extends Record<string, unknown>>(sql: string, params?: unknown[]): Promise<{ rows: T[]; rowCount: number | null }> {
    return this.db.query<T>(sql, params);
  }

  async execute(sql: string, params?: unknown[]): Promise<number> {
    return this.db.execute(sql, params);
  }

  // =========================================================================
  // Schema Management
  // =========================================================================

  async initializeSchema(): Promise<void> {
    logger.info('Initializing ACL schema...');

    const schema = `
      CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
      CREATE EXTENSION IF NOT EXISTS "pg_trgm";

      -- =====================================================================
      -- Roles Table
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS acl_roles (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) DEFAULT 'primary',
        name VARCHAR(128) NOT NULL,
        display_name VARCHAR(255),
        description TEXT,
        parent_role_id UUID REFERENCES acl_roles(id) ON DELETE SET NULL,
        level INTEGER DEFAULT 0,
        is_system BOOLEAN DEFAULT false,
        metadata JSONB DEFAULT '{}',
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        UNIQUE(source_account_id, name)
      );

      CREATE INDEX IF NOT EXISTS idx_acl_roles_source_account
        ON acl_roles(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_acl_roles_name
        ON acl_roles(name);
      CREATE INDEX IF NOT EXISTS idx_acl_roles_parent
        ON acl_roles(parent_role_id);
      CREATE INDEX IF NOT EXISTS idx_acl_roles_level
        ON acl_roles(level);
      CREATE INDEX IF NOT EXISTS idx_acl_roles_is_system
        ON acl_roles(is_system);

      -- =====================================================================
      -- Permissions Table
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS acl_permissions (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) DEFAULT 'primary',
        resource VARCHAR(128) NOT NULL,
        action VARCHAR(64) NOT NULL,
        description TEXT,
        conditions JSONB DEFAULT '{}',
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        UNIQUE(source_account_id, resource, action)
      );

      CREATE INDEX IF NOT EXISTS idx_acl_permissions_source_account
        ON acl_permissions(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_acl_permissions_resource
        ON acl_permissions(resource);
      CREATE INDEX IF NOT EXISTS idx_acl_permissions_action
        ON acl_permissions(action);
      CREATE INDEX IF NOT EXISTS idx_acl_permissions_resource_action
        ON acl_permissions(resource, action);

      -- =====================================================================
      -- Role Permissions Mapping Table
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS acl_role_permissions (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) DEFAULT 'primary',
        role_id UUID NOT NULL REFERENCES acl_roles(id) ON DELETE CASCADE,
        permission_id UUID NOT NULL REFERENCES acl_permissions(id) ON DELETE CASCADE,
        granted BOOLEAN DEFAULT true,
        conditions JSONB DEFAULT '{}',
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        UNIQUE(role_id, permission_id)
      );

      CREATE INDEX IF NOT EXISTS idx_acl_role_permissions_role
        ON acl_role_permissions(role_id);
      CREATE INDEX IF NOT EXISTS idx_acl_role_permissions_permission
        ON acl_role_permissions(permission_id);
      CREATE INDEX IF NOT EXISTS idx_acl_role_permissions_granted
        ON acl_role_permissions(granted);

      -- =====================================================================
      -- User Roles Table
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS acl_user_roles (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) DEFAULT 'primary',
        user_id VARCHAR(255) NOT NULL,
        role_id UUID NOT NULL REFERENCES acl_roles(id) ON DELETE CASCADE,
        granted_by VARCHAR(255),
        expires_at TIMESTAMP WITH TIME ZONE,
        scope VARCHAR(255),
        scope_id VARCHAR(255),
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        UNIQUE(source_account_id, user_id, role_id, scope, scope_id)
      );

      CREATE INDEX IF NOT EXISTS idx_acl_user_roles_source_account
        ON acl_user_roles(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_acl_user_roles_user_id
        ON acl_user_roles(user_id);
      CREATE INDEX IF NOT EXISTS idx_acl_user_roles_role_id
        ON acl_user_roles(role_id);
      CREATE INDEX IF NOT EXISTS idx_acl_user_roles_expires_at
        ON acl_user_roles(expires_at);
      CREATE INDEX IF NOT EXISTS idx_acl_user_roles_scope
        ON acl_user_roles(scope, scope_id);

      -- =====================================================================
      -- Policies Table (ABAC)
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS acl_policies (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        source_account_id VARCHAR(128) DEFAULT 'primary',
        name VARCHAR(255) NOT NULL,
        description TEXT,
        effect VARCHAR(8) NOT NULL DEFAULT 'allow' CHECK (effect IN ('allow', 'deny')),
        principal_type VARCHAR(32) NOT NULL CHECK (principal_type IN ('role', 'user', 'group')),
        principal_value VARCHAR(255) NOT NULL,
        resource_pattern VARCHAR(255) NOT NULL,
        action_pattern VARCHAR(255) NOT NULL,
        conditions JSONB DEFAULT '{}',
        priority INTEGER DEFAULT 0,
        enabled BOOLEAN DEFAULT true,
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_acl_policies_source_account
        ON acl_policies(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_acl_policies_name
        ON acl_policies(name);
      CREATE INDEX IF NOT EXISTS idx_acl_policies_principal
        ON acl_policies(principal_type, principal_value);
      CREATE INDEX IF NOT EXISTS idx_acl_policies_resource_pattern
        ON acl_policies(resource_pattern);
      CREATE INDEX IF NOT EXISTS idx_acl_policies_action_pattern
        ON acl_policies(action_pattern);
      CREATE INDEX IF NOT EXISTS idx_acl_policies_priority
        ON acl_policies(priority DESC);
      CREATE INDEX IF NOT EXISTS idx_acl_policies_enabled
        ON acl_policies(enabled);

      -- =====================================================================
      -- Webhook Events Table
      -- =====================================================================

      CREATE TABLE IF NOT EXISTS acl_webhook_events (
        id VARCHAR(255) PRIMARY KEY,
        source_account_id VARCHAR(128) DEFAULT 'primary',
        event_type VARCHAR(128),
        payload JSONB,
        processed BOOLEAN DEFAULT false,
        processed_at TIMESTAMP WITH TIME ZONE,
        error TEXT,
        created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
      );

      CREATE INDEX IF NOT EXISTS idx_acl_webhook_events_source_account
        ON acl_webhook_events(source_account_id);
      CREATE INDEX IF NOT EXISTS idx_acl_webhook_events_type
        ON acl_webhook_events(event_type);
      CREATE INDEX IF NOT EXISTS idx_acl_webhook_events_processed
        ON acl_webhook_events(processed);
      CREATE INDEX IF NOT EXISTS idx_acl_webhook_events_created
        ON acl_webhook_events(created_at);
    `;

    await this.execute(schema);
    logger.success('ACL schema initialized');
  }

  // =========================================================================
  // Roles Operations
  // =========================================================================

  async createRole(input: CreateRoleInput): Promise<RoleRecord> {
    // Calculate level based on parent
    let level = 0;
    if (input.parent_role_id) {
      const parent = await this.getRole(input.parent_role_id);
      if (parent) {
        level = parent.level + 1;
      }
    }

    const result = await this.query<RoleRecord>(
      `INSERT INTO acl_roles
       (source_account_id, name, display_name, description, parent_role_id, level, is_system, metadata)
       VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
       RETURNING *`,
      [
        this.sourceAccountId,
        input.name,
        input.display_name ?? null,
        input.description ?? null,
        input.parent_role_id ?? null,
        level,
        input.is_system ?? false,
        JSON.stringify(input.metadata ?? {}),
      ]
    );

    return result.rows[0];
  }

  async getRole(id: string): Promise<RoleRecord | null> {
    const result = await this.query<RoleRecord>(
      'SELECT * FROM acl_roles WHERE id = $1 AND source_account_id = $2',
      [id, this.sourceAccountId]
    );

    return result.rows[0] ?? null;
  }

  async getRoleByName(name: string): Promise<RoleRecord | null> {
    const result = await this.query<RoleRecord>(
      'SELECT * FROM acl_roles WHERE name = $1 AND source_account_id = $2',
      [name, this.sourceAccountId]
    );

    return result.rows[0] ?? null;
  }

  async listRoles(limit = 100, offset = 0): Promise<RoleRecord[]> {
    const result = await this.query<RoleRecord>(
      `SELECT * FROM acl_roles
       WHERE source_account_id = $1
       ORDER BY level ASC, name ASC
       LIMIT $2 OFFSET $3`,
      [this.sourceAccountId, limit, offset]
    );

    return result.rows;
  }

  async countRoles(): Promise<number> {
    const result = await this.query<{ count: string }>(
      'SELECT COUNT(*) as count FROM acl_roles WHERE source_account_id = $1',
      [this.sourceAccountId]
    );

    return parseInt(result.rows[0].count, 10);
  }

  async updateRole(id: string, input: UpdateRoleInput): Promise<RoleRecord | null> {
    const updates: string[] = [];
    const params: unknown[] = [id, this.sourceAccountId];
    let paramIndex = 3;

    if (input.display_name !== undefined) {
      updates.push(`display_name = $${paramIndex++}`);
      params.push(input.display_name);
    }

    if (input.description !== undefined) {
      updates.push(`description = $${paramIndex++}`);
      params.push(input.description);
    }

    if (input.parent_role_id !== undefined) {
      // Recalculate level
      let level = 0;
      if (input.parent_role_id) {
        const parent = await this.getRole(input.parent_role_id);
        if (parent) {
          level = parent.level + 1;
        }
      }
      updates.push(`parent_role_id = $${paramIndex++}`);
      params.push(input.parent_role_id);
      updates.push(`level = $${paramIndex++}`);
      params.push(level);
    }

    if (input.metadata !== undefined) {
      updates.push(`metadata = $${paramIndex++}`);
      params.push(JSON.stringify(input.metadata));
    }

    updates.push(`updated_at = NOW()`);

    if (updates.length === 1) {
      return this.getRole(id);
    }

    const result = await this.query<RoleRecord>(
      `UPDATE acl_roles SET ${updates.join(', ')}
       WHERE id = $1 AND source_account_id = $2
       RETURNING *`,
      params
    );

    return result.rows[0] ?? null;
  }

  async deleteRole(id: string): Promise<boolean> {
    const result = await this.execute(
      'DELETE FROM acl_roles WHERE id = $1 AND source_account_id = $2',
      [id, this.sourceAccountId]
    );

    return result > 0;
  }

  async getRoleHierarchy(roleId?: string): Promise<RoleHierarchyNode[]> {
    // Get all roles
    const roles = await this.listRoles(1000, 0);

    // Get all permissions for these roles
    const roleIds = roles.map(r => r.id);
    const permissionsMap = new Map<string, PermissionRecord[]>();

    if (roleIds.length > 0) {
      const result = await this.query<RolePermissionRecord & { resource: string; action: string; description: string }>(
        `SELECT rp.role_id, p.id, p.resource, p.action, p.description, p.conditions
         FROM acl_role_permissions rp
         JOIN acl_permissions p ON p.id = rp.permission_id
         WHERE rp.role_id = ANY($1) AND rp.granted = true`,
        [roleIds]
      );

      for (const row of result.rows) {
        if (!permissionsMap.has(row.role_id)) {
          permissionsMap.set(row.role_id, []);
        }
        permissionsMap.get(row.role_id)!.push({
          id: row.id,
          source_account_id: this.sourceAccountId,
          resource: row.resource,
          action: row.action,
          description: row.description,
          conditions: row.conditions,
          created_at: new Date(),
        });
      }
    }

    // Build hierarchy
    const buildTree = (parentId: string | null): RoleHierarchyNode[] => {
      return roles
        .filter(r => r.parent_role_id === parentId)
        .map(role => ({
          role,
          children: buildTree(role.id),
          permissions: permissionsMap.get(role.id) ?? [],
        }));
    };

    if (roleId) {
      const role = roles.find(r => r.id === roleId);
      if (!role) return [];
      return [{
        role,
        children: buildTree(roleId),
        permissions: permissionsMap.get(roleId) ?? [],
      }];
    }

    return buildTree(null);
  }

  // =========================================================================
  // Permissions Operations
  // =========================================================================

  async createPermission(input: CreatePermissionInput): Promise<PermissionRecord> {
    const result = await this.query<PermissionRecord>(
      `INSERT INTO acl_permissions
       (source_account_id, resource, action, description, conditions)
       VALUES ($1, $2, $3, $4, $5)
       ON CONFLICT (source_account_id, resource, action) DO UPDATE SET
         description = EXCLUDED.description,
         conditions = EXCLUDED.conditions
       RETURNING *`,
      [
        this.sourceAccountId,
        input.resource,
        input.action,
        input.description ?? null,
        JSON.stringify(input.conditions ?? {}),
      ]
    );

    return result.rows[0];
  }

  async getPermission(id: string): Promise<PermissionRecord | null> {
    const result = await this.query<PermissionRecord>(
      'SELECT * FROM acl_permissions WHERE id = $1 AND source_account_id = $2',
      [id, this.sourceAccountId]
    );

    return result.rows[0] ?? null;
  }

  async listPermissions(limit = 100, offset = 0): Promise<PermissionRecord[]> {
    const result = await this.query<PermissionRecord>(
      `SELECT * FROM acl_permissions
       WHERE source_account_id = $1
       ORDER BY resource ASC, action ASC
       LIMIT $2 OFFSET $3`,
      [this.sourceAccountId, limit, offset]
    );

    return result.rows;
  }

  async countPermissions(): Promise<number> {
    const result = await this.query<{ count: string }>(
      'SELECT COUNT(*) as count FROM acl_permissions WHERE source_account_id = $1',
      [this.sourceAccountId]
    );

    return parseInt(result.rows[0].count, 10);
  }

  async deletePermission(id: string): Promise<boolean> {
    const result = await this.execute(
      'DELETE FROM acl_permissions WHERE id = $1 AND source_account_id = $2',
      [id, this.sourceAccountId]
    );

    return result > 0;
  }

  // =========================================================================
  // Role Permissions Operations
  // =========================================================================

  async assignPermissionToRole(roleId: string, input: AssignPermissionInput): Promise<RolePermissionRecord> {
    const result = await this.query<RolePermissionRecord>(
      `INSERT INTO acl_role_permissions
       (source_account_id, role_id, permission_id, granted, conditions)
       VALUES ($1, $2, $3, $4, $5)
       ON CONFLICT (role_id, permission_id) DO UPDATE SET
         granted = EXCLUDED.granted,
         conditions = EXCLUDED.conditions
       RETURNING *`,
      [
        this.sourceAccountId,
        roleId,
        input.permission_id,
        input.granted ?? true,
        JSON.stringify(input.conditions ?? {}),
      ]
    );

    return result.rows[0];
  }

  async removePermissionFromRole(roleId: string, permissionId: string): Promise<boolean> {
    const result = await this.execute(
      'DELETE FROM acl_role_permissions WHERE role_id = $1 AND permission_id = $2',
      [roleId, permissionId]
    );

    return result > 0;
  }

  async getRolePermissions(roleId: string): Promise<PermissionRecord[]> {
    const result = await this.query<PermissionRecord>(
      `SELECT p.* FROM acl_permissions p
       JOIN acl_role_permissions rp ON rp.permission_id = p.id
       WHERE rp.role_id = $1 AND rp.granted = true
       ORDER BY p.resource ASC, p.action ASC`,
      [roleId]
    );

    return result.rows;
  }

  // =========================================================================
  // User Roles Operations
  // =========================================================================

  async assignRoleToUser(userId: string, input: AssignUserRoleInput): Promise<UserRoleRecord> {
    const result = await this.query<UserRoleRecord>(
      `INSERT INTO acl_user_roles
       (source_account_id, user_id, role_id, granted_by, expires_at, scope, scope_id)
       VALUES ($1, $2, $3, $4, $5, $6, $7)
       ON CONFLICT (source_account_id, user_id, role_id, scope, scope_id) DO UPDATE SET
         granted_by = EXCLUDED.granted_by,
         expires_at = EXCLUDED.expires_at
       RETURNING *`,
      [
        this.sourceAccountId,
        userId,
        input.role_id,
        input.granted_by ?? null,
        input.expires_at ?? null,
        input.scope ?? null,
        input.scope_id ?? null,
      ]
    );

    return result.rows[0];
  }

  async removeRoleFromUser(userId: string, roleId: string, scope?: string, scopeId?: string): Promise<boolean> {
    let sql = 'DELETE FROM acl_user_roles WHERE source_account_id = $1 AND user_id = $2 AND role_id = $3';
    const params: unknown[] = [this.sourceAccountId, userId, roleId];

    if (scope !== undefined) {
      sql += ' AND scope = $4';
      params.push(scope);
    }

    if (scopeId !== undefined) {
      sql += ` AND scope_id = $${params.length + 1}`;
      params.push(scopeId);
    }

    const result = await this.execute(sql, params);
    return result > 0;
  }

  async getUserRoles(userId: string): Promise<RoleRecord[]> {
    const result = await this.query<RoleRecord>(
      `SELECT r.* FROM acl_roles r
       JOIN acl_user_roles ur ON ur.role_id = r.id
       WHERE ur.user_id = $1
         AND ur.source_account_id = $2
         AND (ur.expires_at IS NULL OR ur.expires_at > NOW())
       ORDER BY r.level ASC, r.name ASC`,
      [userId, this.sourceAccountId]
    );

    return result.rows;
  }

  async getUserPermissions(userId: string): Promise<PermissionRecord[]> {
    // Get all permissions through role hierarchy
    const result = await this.query<PermissionRecord>(
      `WITH RECURSIVE role_tree AS (
         -- Start with user's direct roles
         SELECT r.id, r.parent_role_id, r.level
         FROM acl_roles r
         JOIN acl_user_roles ur ON ur.role_id = r.id
         WHERE ur.user_id = $1
           AND ur.source_account_id = $2
           AND (ur.expires_at IS NULL OR ur.expires_at > NOW())

         UNION

         -- Add parent roles recursively
         SELECT r.id, r.parent_role_id, r.level
         FROM acl_roles r
         JOIN role_tree rt ON rt.parent_role_id = r.id
       )
       SELECT DISTINCT p.*
       FROM acl_permissions p
       JOIN acl_role_permissions rp ON rp.permission_id = p.id
       JOIN role_tree rt ON rt.id = rp.role_id
       WHERE rp.granted = true
       ORDER BY p.resource ASC, p.action ASC`,
      [userId, this.sourceAccountId]
    );

    return result.rows;
  }

  // =========================================================================
  // Policies Operations
  // =========================================================================

  async createPolicy(input: CreatePolicyInput): Promise<PolicyRecord> {
    const result = await this.query<PolicyRecord>(
      `INSERT INTO acl_policies
       (source_account_id, name, description, effect, principal_type, principal_value,
        resource_pattern, action_pattern, conditions, priority, enabled)
       VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
       RETURNING *`,
      [
        this.sourceAccountId,
        input.name,
        input.description ?? null,
        input.effect,
        input.principal_type,
        input.principal_value,
        input.resource_pattern,
        input.action_pattern,
        JSON.stringify(input.conditions ?? {}),
        input.priority ?? 0,
        input.enabled ?? true,
      ]
    );

    return result.rows[0];
  }

  async getPolicy(id: string): Promise<PolicyRecord | null> {
    const result = await this.query<PolicyRecord>(
      'SELECT * FROM acl_policies WHERE id = $1 AND source_account_id = $2',
      [id, this.sourceAccountId]
    );

    return result.rows[0] ?? null;
  }

  async listPolicies(limit = 100, offset = 0): Promise<PolicyRecord[]> {
    const result = await this.query<PolicyRecord>(
      `SELECT * FROM acl_policies
       WHERE source_account_id = $1
       ORDER BY priority DESC, name ASC
       LIMIT $2 OFFSET $3`,
      [this.sourceAccountId, limit, offset]
    );

    return result.rows;
  }

  async countPolicies(): Promise<number> {
    const result = await this.query<{ count: string }>(
      'SELECT COUNT(*) as count FROM acl_policies WHERE source_account_id = $1',
      [this.sourceAccountId]
    );

    return parseInt(result.rows[0].count, 10);
  }

  async updatePolicy(id: string, input: UpdatePolicyInput): Promise<PolicyRecord | null> {
    const updates: string[] = [];
    const params: unknown[] = [id, this.sourceAccountId];
    let paramIndex = 3;

    if (input.name !== undefined) {
      updates.push(`name = $${paramIndex++}`);
      params.push(input.name);
    }

    if (input.description !== undefined) {
      updates.push(`description = $${paramIndex++}`);
      params.push(input.description);
    }

    if (input.effect !== undefined) {
      updates.push(`effect = $${paramIndex++}`);
      params.push(input.effect);
    }

    if (input.principal_value !== undefined) {
      updates.push(`principal_value = $${paramIndex++}`);
      params.push(input.principal_value);
    }

    if (input.resource_pattern !== undefined) {
      updates.push(`resource_pattern = $${paramIndex++}`);
      params.push(input.resource_pattern);
    }

    if (input.action_pattern !== undefined) {
      updates.push(`action_pattern = $${paramIndex++}`);
      params.push(input.action_pattern);
    }

    if (input.conditions !== undefined) {
      updates.push(`conditions = $${paramIndex++}`);
      params.push(JSON.stringify(input.conditions));
    }

    if (input.priority !== undefined) {
      updates.push(`priority = $${paramIndex++}`);
      params.push(input.priority);
    }

    if (input.enabled !== undefined) {
      updates.push(`enabled = $${paramIndex++}`);
      params.push(input.enabled);
    }

    updates.push(`updated_at = NOW()`);

    if (updates.length === 1) {
      return this.getPolicy(id);
    }

    const result = await this.query<PolicyRecord>(
      `UPDATE acl_policies SET ${updates.join(', ')}
       WHERE id = $1 AND source_account_id = $2
       RETURNING *`,
      params
    );

    return result.rows[0] ?? null;
  }

  async deletePolicy(id: string): Promise<boolean> {
    const result = await this.execute(
      'DELETE FROM acl_policies WHERE id = $1 AND source_account_id = $2',
      [id, this.sourceAccountId]
    );

    return result > 0;
  }

  async getApplicablePolicies(userId: string, userRoles: string[]): Promise<PolicyRecord[]> {
    const result = await this.query<PolicyRecord>(
      `SELECT * FROM acl_policies
       WHERE source_account_id = $1
         AND enabled = true
         AND (
           (principal_type = 'user' AND principal_value = $2)
           OR (principal_type = 'role' AND principal_value = ANY($3))
         )
       ORDER BY priority DESC, created_at ASC`,
      [this.sourceAccountId, userId, userRoles]
    );

    return result.rows;
  }

  // =========================================================================
  // Webhook Events Operations
  // =========================================================================

  async insertWebhookEvent(id: string, eventType: string, payload: Record<string, unknown>): Promise<void> {
    await this.execute(
      `INSERT INTO acl_webhook_events (id, source_account_id, event_type, payload)
       VALUES ($1, $2, $3, $4)
       ON CONFLICT (id) DO NOTHING`,
      [id, this.sourceAccountId, eventType, JSON.stringify(payload)]
    );
  }

  async markEventProcessed(id: string, error?: string): Promise<void> {
    await this.execute(
      `UPDATE acl_webhook_events
       SET processed = true, processed_at = NOW(), error = $2
       WHERE id = $1`,
      [id, error ?? null]
    );
  }

  // =========================================================================
  // Statistics
  // =========================================================================

  async getStats(): Promise<ACLStats> {
    const [roles, permissions, rolePermissions, userRoles, policies] = await Promise.all([
      this.countRoles(),
      this.countPermissions(),
      this.query<{ count: string }>(
        'SELECT COUNT(*) as count FROM acl_role_permissions'
      ).then(r => parseInt(r.rows[0].count, 10)),
      this.query<{ count: string }>(
        'SELECT COUNT(*) as count FROM acl_user_roles WHERE source_account_id = $1',
        [this.sourceAccountId]
      ).then(r => parseInt(r.rows[0].count, 10)),
      this.countPolicies(),
    ]);

    return {
      roles,
      permissions,
      role_permissions: rolePermissions,
      user_roles: userRoles,
      policies,
    };
  }
}
