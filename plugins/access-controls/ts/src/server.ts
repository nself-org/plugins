/**
 * Access Controls Plugin Server
 * HTTP server for ACL API endpoints
 */

import Fastify from 'fastify';
import cors from '@fastify/cors';
import { createLogger, ApiRateLimiter, createAuthHook, createRateLimitHook, getAppContext } from '@nself/plugin-utils';
import { ACLDatabase } from './database.js';
import { AuthorizationEngine } from './authz.js';
import { loadConfig, type Config } from './config.js';
import type {
  CreateRoleInput,
  UpdateRoleInput,
  CreatePermissionInput,
  AssignPermissionInput,
  AssignUserRoleInput,
  CreatePolicyInput,
  UpdatePolicyInput,
  AuthorizationRequest,
  BatchAuthorizationRequest,
} from './types.js';

const logger = createLogger('acl:server');

export async function createServer(config?: Partial<Config>) {
  const fullConfig = loadConfig(config);

  // Initialize components
  const db = new ACLDatabase();
  await db.connect();
  await db.initializeSchema();

  const authzEngine = new AuthorizationEngine(
    db,
    fullConfig.cacheTtlSeconds,
    fullConfig.maxRoleDepth,
    fullConfig.defaultDeny
  );

  // Create Fastify server
  const app = Fastify({
    logger: false,
    bodyLimit: 10 * 1024 * 1024,
  });

  // Register CORS
  await app.register(cors, {
    origin: true,
    credentials: true,
  });

  // Security middleware
  const rateLimiter = new ApiRateLimiter(
    fullConfig.security.rateLimitMax ?? 200,
    fullConfig.security.rateLimitWindowMs ?? 60000
  );

  // Add rate limiting to all requests
  app.addHook('preHandler', createRateLimitHook(rateLimiter) as never);

  // Add API key authentication (skips health check endpoints)
  if (fullConfig.security.apiKey) {
    app.addHook('preHandler', createAuthHook(fullConfig.security.apiKey) as never);
    logger.info('API key authentication enabled');
  }

  // Multi-app context: resolve source_account_id per request and create scoped DB
  app.decorateRequest('scopedDb', null);
  app.decorateRequest('scopedAuthz', null);

  app.addHook('onRequest', async (request) => {
    const ctx = getAppContext(request);
    const scopedDb = db.forSourceAccount(ctx.sourceAccountId);
    const scopedAuthz = new AuthorizationEngine(
      scopedDb,
      fullConfig.cacheTtlSeconds,
      fullConfig.maxRoleDepth,
      fullConfig.defaultDeny
    );

    (request as unknown as Record<string, unknown>).scopedDb = scopedDb;
    (request as unknown as Record<string, unknown>).scopedAuthz = scopedAuthz;
  });

  /** Extract scoped database from request */
  function scopedDb(request: unknown): ACLDatabase {
    return (request as Record<string, unknown>).scopedDb as ACLDatabase;
  }

  /** Extract scoped authorization engine from request */
  function scopedAuthz(request: unknown): AuthorizationEngine {
    return (request as Record<string, unknown>).scopedAuthz as AuthorizationEngine;
  }

  // =========================================================================
  // Health Check Endpoints
  // =========================================================================

  app.get('/health', async () => {
    return { status: 'ok', plugin: 'access-controls', timestamp: new Date().toISOString() };
  });

  app.get('/ready', async (_request, reply) => {
    try {
      await db.query('SELECT 1');
      return { ready: true, plugin: 'access-controls', timestamp: new Date().toISOString() };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Database unavailable';
      logger.error('Readiness check failed', { error: message });
      return reply.status(503).send({
        ready: false,
        plugin: 'access-controls',
        error: 'Database unavailable',
        timestamp: new Date().toISOString(),
      });
    }
  });

  app.get('/live', async (request) => {
    const stats = await scopedDb(request).getStats();
    const cacheStats = scopedAuthz(request).getCacheStats();

    return {
      alive: true,
      plugin: 'access-controls',
      version: '1.0.0',
      uptime: process.uptime(),
      memory: process.memoryUsage(),
      stats,
      cache: { size: cacheStats.size },
      timestamp: new Date().toISOString(),
    };
  });

  // =========================================================================
  // Status Endpoint
  // =========================================================================

  app.get('/status', async (request) => {
    const stats = await scopedDb(request).getStats();
    const cacheStats = scopedAuthz(request).getCacheStats();

    return {
      plugin: 'access-controls',
      version: '1.0.0',
      status: 'running',
      config: {
        cacheTtlSeconds: fullConfig.cacheTtlSeconds,
        maxRoleDepth: fullConfig.maxRoleDepth,
        defaultDeny: fullConfig.defaultDeny,
      },
      stats,
      cache: cacheStats,
      timestamp: new Date().toISOString(),
    };
  });

  // =========================================================================
  // Roles Endpoints
  // =========================================================================

  app.post('/v1/roles', async (request, reply) => {
    try {
      const input = request.body as CreateRoleInput;
      const role = await scopedDb(request).createRole(input);
      return reply.status(201).send(role);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to create role', { error: message });
      return reply.status(400).send({ error: message });
    }
  });

  app.get('/v1/roles', async (request) => {
    const { limit = 100, offset = 0 } = request.query as { limit?: number; offset?: number };
    const roles = await scopedDb(request).listRoles(limit, offset);
    const total = await scopedDb(request).countRoles();
    return { data: roles, total, limit, offset };
  });

  app.get('/v1/roles/hierarchy', async (request) => {
    const { role_id } = request.query as { role_id?: string };
    const hierarchy = await scopedDb(request).getRoleHierarchy(role_id);
    return { data: hierarchy };
  });

  app.get('/v1/roles/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const role = await scopedDb(request).getRole(id);

    if (!role) {
      return reply.status(404).send({ error: 'Role not found' });
    }

    const permissions = await scopedDb(request).getRolePermissions(id);
    return { ...role, permissions };
  });

  app.put('/v1/roles/:id', async (request, reply) => {
    try {
      const { id } = request.params as { id: string };
      const input = request.body as UpdateRoleInput;
      const role = await scopedDb(request).updateRole(id, input);

      if (!role) {
        return reply.status(404).send({ error: 'Role not found' });
      }

      return role;
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to update role', { error: message });
      return reply.status(400).send({ error: message });
    }
  });

  app.delete('/v1/roles/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const deleted = await scopedDb(request).deleteRole(id);

    if (!deleted) {
      return reply.status(404).send({ error: 'Role not found' });
    }

    return { success: true };
  });

  // =========================================================================
  // Permissions Endpoints
  // =========================================================================

  app.post('/v1/permissions', async (request, reply) => {
    try {
      const input = request.body as CreatePermissionInput;
      const permission = await scopedDb(request).createPermission(input);
      return reply.status(201).send(permission);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to create permission', { error: message });
      return reply.status(400).send({ error: message });
    }
  });

  app.get('/v1/permissions', async (request) => {
    const { limit = 100, offset = 0 } = request.query as { limit?: number; offset?: number };
    const permissions = await scopedDb(request).listPermissions(limit, offset);
    const total = await scopedDb(request).countPermissions();
    return { data: permissions, total, limit, offset };
  });

  app.get('/v1/permissions/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const permission = await scopedDb(request).getPermission(id);

    if (!permission) {
      return reply.status(404).send({ error: 'Permission not found' });
    }

    return permission;
  });

  app.delete('/v1/permissions/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const deleted = await scopedDb(request).deletePermission(id);

    if (!deleted) {
      return reply.status(404).send({ error: 'Permission not found' });
    }

    return { success: true };
  });

  // =========================================================================
  // Role Permissions Endpoints
  // =========================================================================

  app.post('/v1/roles/:id/permissions', async (request, reply) => {
    try {
      const { id } = request.params as { id: string };
      const input = request.body as AssignPermissionInput;
      const rolePermission = await scopedDb(request).assignPermissionToRole(id, input);

      // Invalidate cache for users with this role
      scopedAuthz(request).clearCache();

      return reply.status(201).send(rolePermission);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to assign permission to role', { error: message });
      return reply.status(400).send({ error: message });
    }
  });

  app.delete('/v1/roles/:roleId/permissions/:permId', async (request, reply) => {
    const { roleId, permId } = request.params as { roleId: string; permId: string };
    const removed = await scopedDb(request).removePermissionFromRole(roleId, permId);

    if (!removed) {
      return reply.status(404).send({ error: 'Role permission mapping not found' });
    }

    // Invalidate cache
    scopedAuthz(request).clearCache();

    return { success: true };
  });

  // =========================================================================
  // User Roles Endpoints
  // =========================================================================

  app.post('/v1/users/:userId/roles', async (request, reply) => {
    try {
      const { userId } = request.params as { userId: string };
      const input = request.body as AssignUserRoleInput;
      const userRole = await scopedDb(request).assignRoleToUser(userId, input);

      // Invalidate user's cache
      scopedAuthz(request).invalidateUserCache(userId);

      return reply.status(201).send(userRole);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to assign role to user', { error: message });
      return reply.status(400).send({ error: message });
    }
  });

  app.get('/v1/users/:userId/roles', async (request) => {
    const { userId } = request.params as { userId: string };
    const roles = await scopedDb(request).getUserRoles(userId);
    return { data: roles };
  });

  app.delete('/v1/users/:userId/roles/:roleId', async (request, reply) => {
    const { userId, roleId } = request.params as { userId: string; roleId: string };
    const { scope, scope_id } = request.query as { scope?: string; scope_id?: string };

    const removed = await scopedDb(request).removeRoleFromUser(userId, roleId, scope, scope_id);

    if (!removed) {
      return reply.status(404).send({ error: 'User role assignment not found' });
    }

    // Invalidate user's cache
    scopedAuthz(request).invalidateUserCache(userId);

    return { success: true };
  });

  app.get('/v1/users/:userId/permissions', async (request) => {
    const { userId } = request.params as { userId: string };
    const permissions = await scopedDb(request).getUserPermissions(userId);
    return { data: permissions };
  });

  // =========================================================================
  // Authorization Endpoints
  // =========================================================================

  app.post('/v1/authorize', async (request, reply) => {
    try {
      const authRequest = request.body as AuthorizationRequest;
      const result = await scopedAuthz(request).authorize(authRequest);
      return result;
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Authorization check failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  app.post('/v1/authorize/batch', async (request, reply) => {
    try {
      const { requests } = request.body as BatchAuthorizationRequest;
      const results = await scopedAuthz(request).batchAuthorize(requests);
      return { results: results.map((result, i) => ({ ...result, request: requests[i] })) };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Batch authorization check failed', { error: message });
      return reply.status(500).send({ error: message });
    }
  });

  // =========================================================================
  // Policies Endpoints
  // =========================================================================

  app.post('/v1/policies', async (request, reply) => {
    try {
      const input = request.body as CreatePolicyInput;
      const policy = await scopedDb(request).createPolicy(input);
      return reply.status(201).send(policy);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to create policy', { error: message });
      return reply.status(400).send({ error: message });
    }
  });

  app.get('/v1/policies', async (request) => {
    const { limit = 100, offset = 0 } = request.query as { limit?: number; offset?: number };
    const policies = await scopedDb(request).listPolicies(limit, offset);
    const total = await scopedDb(request).countPolicies();
    return { data: policies, total, limit, offset };
  });

  app.get('/v1/policies/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const policy = await scopedDb(request).getPolicy(id);

    if (!policy) {
      return reply.status(404).send({ error: 'Policy not found' });
    }

    return policy;
  });

  app.put('/v1/policies/:id', async (request, reply) => {
    try {
      const { id } = request.params as { id: string };
      const input = request.body as UpdatePolicyInput;
      const policy = await scopedDb(request).updatePolicy(id, input);

      if (!policy) {
        return reply.status(404).send({ error: 'Policy not found' });
      }

      return policy;
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to update policy', { error: message });
      return reply.status(400).send({ error: message });
    }
  });

  app.delete('/v1/policies/:id', async (request, reply) => {
    const { id } = request.params as { id: string };
    const deleted = await scopedDb(request).deletePolicy(id);

    if (!deleted) {
      return reply.status(404).send({ error: 'Policy not found' });
    }

    return { success: true };
  });

  // =========================================================================
  // Cache Management Endpoints
  // =========================================================================

  app.post('/v1/cache/invalidate', async (request) => {
    const { user_id } = request.body as { user_id?: string };

    if (user_id) {
      scopedAuthz(request).invalidateUserCache(user_id);
      return { success: true, message: `Cache invalidated for user ${user_id}` };
    } else {
      scopedAuthz(request).clearCache();
      return { success: true, message: 'All cache cleared' };
    }
  });

  app.get('/v1/cache/stats', async (request) => {
    return scopedAuthz(request).getCacheStats();
  });

  // Graceful shutdown
  const shutdown = async () => {
    logger.info('Shutting down...');
    await app.close();
    await db.disconnect();
    process.exit(0);
  };

  process.on('SIGINT', shutdown);
  process.on('SIGTERM', shutdown);

  return {
    app,
    db,
    authzEngine,
    start: async () => {
      await app.listen({ port: fullConfig.port, host: fullConfig.host });
      logger.success(`Access Controls plugin server running on http://${fullConfig.host}:${fullConfig.port}`);
    },
    stop: shutdown,
  };
}

// Start server if run directly
const isMainModule = import.meta.url === `file://${process.argv[1]}`;
if (isMainModule) {
  createServer()
    .then(server => server.start())
    .catch(error => {
      logger.error('Failed to start server', { error: error.message });
      process.exit(1);
    });
}
