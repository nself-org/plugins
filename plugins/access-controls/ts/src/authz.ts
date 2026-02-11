/**
 * Authorization Engine
 * RBAC + ABAC authorization with caching and pattern matching
 */

import { createLogger } from '@nself/plugin-utils';
import { ACLDatabase } from './database.js';
import type {
  AuthorizationRequest,
  AuthorizationResult,
  PolicyRecord,
  UserPermissionsCache,
  EffectivePermission,
} from './types.js';

const logger = createLogger('acl:authz');

export class AuthorizationEngine {
  private db: ACLDatabase;
  private cacheTtlSeconds: number;
  private defaultDeny: boolean;
  private cache: Map<string, UserPermissionsCache>;

  constructor(
    db: ACLDatabase,
    cacheTtlSeconds = 300,
    _maxRoleDepth = 10,
    defaultDeny = true
  ) {
    this.db = db;
    this.cacheTtlSeconds = cacheTtlSeconds;
    this.defaultDeny = defaultDeny;
    this.cache = new Map();
  }

  /**
   * Main authorization check
   */
  async authorize(request: AuthorizationRequest): Promise<AuthorizationResult> {
    const startTime = Date.now();

    try {
      // Get user's effective permissions (cached)
      const userPerms = await this.getUserEffectivePermissions(request.user_id);

      // Check RBAC permissions first
      const rbacResult = this.checkRBACPermissions(
        userPerms.permissions,
        request.resource,
        request.action,
        request.context
      );

      if (rbacResult.matched) {
        logger.debug('RBAC allowed', {
          userId: request.user_id,
          resource: request.resource,
          action: request.action,
          duration: Date.now() - startTime,
        });

        return {
          allowed: true,
          reason: 'RBAC permission granted',
          matched_permissions: rbacResult.permissionIds,
          cached: userPerms.cached_at < new Date(),
        };
      }

      // Check ABAC policies
      const policies = await this.db.getApplicablePolicies(
        request.user_id,
        userPerms.roles
      );

      const policyResult = this.evaluatePolicies(
        policies,
        request.resource,
        request.action,
        request.context
      );

      if (policyResult.decision !== null) {
        logger.debug('ABAC policy decision', {
          userId: request.user_id,
          resource: request.resource,
          action: request.action,
          decision: policyResult.decision,
          duration: Date.now() - startTime,
        });

        return {
          allowed: policyResult.decision,
          reason: policyResult.reason,
          matched_policies: policyResult.policyIds,
          cached: false,
        };
      }

      // Default decision
      logger.debug('Default decision', {
        userId: request.user_id,
        resource: request.resource,
        action: request.action,
        defaultDeny: this.defaultDeny,
        duration: Date.now() - startTime,
      });

      return {
        allowed: !this.defaultDeny,
        reason: this.defaultDeny ? 'No matching permissions or policies (default deny)' : 'No restrictions (default allow)',
        cached: false,
      };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Authorization check failed', {
        error: message,
        userId: request.user_id,
        resource: request.resource,
        action: request.action,
      });

      // Fail secure: deny on error
      return {
        allowed: false,
        reason: `Authorization error: ${message}`,
        cached: false,
      };
    }
  }

  /**
   * Batch authorization check
   */
  async batchAuthorize(requests: AuthorizationRequest[]): Promise<AuthorizationResult[]> {
    return Promise.all(requests.map(req => this.authorize(req)));
  }

  /**
   * Get user's effective permissions with caching
   */
  private async getUserEffectivePermissions(userId: string): Promise<UserPermissionsCache> {
    const cacheKey = `${this.db.getCurrentSourceAccountId()}:${userId}`;
    const cached = this.cache.get(cacheKey);

    if (cached && cached.expires_at > new Date()) {
      return cached;
    }

    // Fetch fresh permissions
    const [roles, permissions] = await Promise.all([
      this.db.getUserRoles(userId),
      this.db.getUserPermissions(userId),
    ]);

    const effectivePermissions: EffectivePermission[] = permissions.map(p => ({
      resource: p.resource,
      action: p.action,
      granted: true,
      source: 'role' as const,
      source_id: p.id,
      conditions: p.conditions,
    }));

    const now = new Date();
    const expiresAt = new Date(now.getTime() + this.cacheTtlSeconds * 1000);

    const cache: UserPermissionsCache = {
      user_id: userId,
      source_account_id: this.db.getCurrentSourceAccountId(),
      permissions: effectivePermissions,
      roles: roles.map(r => r.id),
      cached_at: now,
      expires_at: expiresAt,
    };

    this.cache.set(cacheKey, cache);

    return cache;
  }

  /**
   * Check RBAC permissions
   */
  private checkRBACPermissions(
    permissions: EffectivePermission[],
    resource: string,
    action: string,
    context?: Record<string, unknown>
  ): { matched: boolean; permissionIds: string[] } {
    const matchedIds: string[] = [];

    for (const perm of permissions) {
      if (this.matchPattern(perm.resource, resource) && this.matchPattern(perm.action, action)) {
        // Check conditions if present
        if (Object.keys(perm.conditions).length > 0) {
          if (!this.evaluateConditions(perm.conditions, context ?? {})) {
            continue;
          }
        }

        matchedIds.push(perm.source_id);
      }
    }

    return {
      matched: matchedIds.length > 0,
      permissionIds: matchedIds,
    };
  }

  /**
   * Evaluate ABAC policies
   */
  private evaluatePolicies(
    policies: PolicyRecord[],
    resource: string,
    action: string,
    context?: Record<string, unknown>
  ): { decision: boolean | null; reason: string; policyIds: string[] } {
    // Policies are ordered by priority DESC
    for (const policy of policies) {
      // Check pattern match
      if (!this.matchPattern(policy.resource_pattern, resource)) {
        continue;
      }

      if (!this.matchPattern(policy.action_pattern, action)) {
        continue;
      }

      // Check conditions
      if (Object.keys(policy.conditions).length > 0) {
        if (!this.evaluateConditions(policy.conditions, context ?? {})) {
          continue;
        }
      }

      // Policy matched - return decision
      const allowed = policy.effect === 'allow';
      return {
        decision: allowed,
        reason: `Policy "${policy.name}" (${policy.effect})`,
        policyIds: [policy.id],
      };
    }

    // No policy matched
    return {
      decision: null,
      reason: 'No applicable policies',
      policyIds: [],
    };
  }

  /**
   * Pattern matching with wildcards
   */
  private matchPattern(pattern: string, value: string): boolean {
    // Exact match
    if (pattern === value) {
      return true;
    }

    // Wildcard match (supports * for any sequence)
    if (pattern.includes('*')) {
      const regexPattern = pattern
        .replace(/[.+?^${}()|[\]\\]/g, '\\$&') // Escape special chars
        .replace(/\*/g, '.*'); // Convert * to .*

      const regex = new RegExp(`^${regexPattern}$`);
      return regex.test(value);
    }

    return false;
  }

  /**
   * Evaluate conditions (simple key-value equality for now)
   */
  private evaluateConditions(
    conditions: Record<string, unknown>,
    context: Record<string, unknown>
  ): boolean {
    for (const [key, expectedValue] of Object.entries(conditions)) {
      const actualValue = context[key];

      // Handle different condition operators
      if (typeof expectedValue === 'object' && expectedValue !== null) {
        const condObj = expectedValue as Record<string, unknown>;

        // $eq operator
        if ('$eq' in condObj) {
          if (actualValue !== condObj.$eq) {
            return false;
          }
        }

        // $ne operator
        if ('$ne' in condObj) {
          if (actualValue === condObj.$ne) {
            return false;
          }
        }

        // $in operator
        if ('$in' in condObj && Array.isArray(condObj.$in)) {
          if (!condObj.$in.includes(actualValue)) {
            return false;
          }
        }

        // $nin operator
        if ('$nin' in condObj && Array.isArray(condObj.$nin)) {
          if (condObj.$nin.includes(actualValue)) {
            return false;
          }
        }

        // $gt operator
        if ('$gt' in condObj) {
          if (typeof actualValue !== 'number' || actualValue <= (condObj.$gt as number)) {
            return false;
          }
        }

        // $gte operator
        if ('$gte' in condObj) {
          if (typeof actualValue !== 'number' || actualValue < (condObj.$gte as number)) {
            return false;
          }
        }

        // $lt operator
        if ('$lt' in condObj) {
          if (typeof actualValue !== 'number' || actualValue >= (condObj.$lt as number)) {
            return false;
          }
        }

        // $lte operator
        if ('$lte' in condObj) {
          if (typeof actualValue !== 'number' || actualValue > (condObj.$lte as number)) {
            return false;
          }
        }
      } else {
        // Simple equality check
        if (actualValue !== expectedValue) {
          return false;
        }
      }
    }

    return true;
  }

  /**
   * Invalidate user's cached permissions
   */
  invalidateUserCache(userId: string): void {
    const cacheKey = `${this.db.getCurrentSourceAccountId()}:${userId}`;
    this.cache.delete(cacheKey);
    logger.debug('Invalidated cache', { userId });
  }

  /**
   * Clear all cache
   */
  clearCache(): void {
    this.cache.clear();
    logger.debug('Cleared all cache');
  }

  /**
   * Get cache statistics
   */
  getCacheStats(): { size: number; keys: string[] } {
    return {
      size: this.cache.size,
      keys: Array.from(this.cache.keys()),
    };
  }
}
