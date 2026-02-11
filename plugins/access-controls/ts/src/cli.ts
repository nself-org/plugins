#!/usr/bin/env node
/**
 * Access Controls Plugin CLI
 * Command-line interface for the ACL plugin
 */

import { Command } from 'commander';
import { createLogger } from '@nself/plugin-utils';
import { loadConfig } from './config.js';
import { ACLDatabase } from './database.js';
import { AuthorizationEngine } from './authz.js';
import { createServer } from './server.js';

const logger = createLogger('acl:cli');

const program = new Command();

program
  .name('nself-acl')
  .description('Access Controls plugin for nself - RBAC + ABAC authorization')
  .version('1.0.0');

// Init command
program
  .command('init')
  .description('Initialize database schema')
  .action(async () => {
    try {
      loadConfig(); // Validate config

      const db = new ACLDatabase();
      await db.connect();
      await db.initializeSchema();
      await db.disconnect();

      logger.success('Database schema initialized');
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Init failed', { error: message });
      process.exit(1);
    }
  });

// Server command
program
  .command('server')
  .description('Start the ACL API server')
  .option('-p, --port <port>', 'Server port', '3027')
  .option('-h, --host <host>', 'Server host', '0.0.0.0')
  .action(async (options) => {
    try {
      const config = loadConfig({
        port: parseInt(options.port, 10),
        host: options.host,
      });

      const server = await createServer(config);
      await server.start();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Server failed', { error: message });
      process.exit(1);
    }
  });

// Status command
program
  .command('status')
  .description('Show ACL status and statistics')
  .action(async () => {
    try {
      loadConfig();

      const db = new ACLDatabase();
      await db.connect();

      const stats = await db.getStats();

      console.log('\nAccess Controls Plugin Status');
      console.log('==============================');
      console.log(`Roles:            ${stats.roles}`);
      console.log(`Permissions:      ${stats.permissions}`);
      console.log(`Role Permissions: ${stats.role_permissions}`);
      console.log(`User Roles:       ${stats.user_roles}`);
      console.log(`Policies:         ${stats.policies}`);

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Status check failed', { error: message });
      process.exit(1);
    }
  });

// Roles command
program
  .command('roles')
  .description('Manage roles')
  .argument('[action]', 'Action: list, create, show, delete', 'list')
  .argument('[name]', 'Role name (for create/show/delete)')
  .option('-d, --display-name <name>', 'Display name')
  .option('--description <desc>', 'Role description')
  .option('-p, --parent <parent>', 'Parent role name')
  .option('-l, --limit <limit>', 'Number of records', '50')
  .action(async (action, name, options) => {
    try {
      const db = new ACLDatabase();
      await db.connect();

      switch (action) {
        case 'list': {
          const roles = await db.listRoles(parseInt(options.limit, 10));
          console.log('\nRoles:');
          console.log('-'.repeat(100));
          roles.forEach(r => {
            const parent = r.parent_role_id ? `→ ${r.parent_role_id}` : '';
            console.log(`${r.name.padEnd(30)} | Level ${r.level} ${parent} | ${r.description ?? 'No description'}`);
          });
          console.log(`\nTotal: ${await db.countRoles()}`);
          break;
        }

        case 'create': {
          if (!name) {
            logger.error('Role name required');
            process.exit(1);
          }

          let parentRoleId: string | undefined;
          if (options.parent) {
            const parent = await db.getRoleByName(options.parent);
            if (!parent) {
              logger.error(`Parent role "${options.parent}" not found`);
              process.exit(1);
            }
            parentRoleId = parent.id;
          }

          const role = await db.createRole({
            name,
            display_name: options.displayName,
            description: options.description,
            parent_role_id: parentRoleId,
          });

          logger.success(`Created role: ${role.name} (${role.id})`);
          break;
        }

        case 'show': {
          if (!name) {
            logger.error('Role name required');
            process.exit(1);
          }

          const role = await db.getRoleByName(name);
          if (!role) {
            logger.error('Role not found');
            process.exit(1);
          }

          const permissions = await db.getRolePermissions(role.id);

          console.log('\nRole Details:');
          console.log('=============');
          console.log(`ID:           ${role.id}`);
          console.log(`Name:         ${role.name}`);
          console.log(`Display Name: ${role.display_name ?? 'N/A'}`);
          console.log(`Description:  ${role.description ?? 'N/A'}`);
          console.log(`Level:        ${role.level}`);
          console.log(`Parent:       ${role.parent_role_id ?? 'None'}`);
          console.log(`System:       ${role.is_system}`);
          console.log(`\nPermissions (${permissions.length}):`);
          permissions.forEach(p => {
            console.log(`  - ${p.resource}:${p.action}`);
          });
          break;
        }

        case 'delete': {
          if (!name) {
            logger.error('Role name required');
            process.exit(1);
          }

          const role = await db.getRoleByName(name);
          if (!role) {
            logger.error('Role not found');
            process.exit(1);
          }

          await db.deleteRole(role.id);
          logger.success(`Deleted role: ${name}`);
          break;
        }

        default:
          logger.error(`Unknown action: ${action}`);
          process.exit(1);
      }

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Command failed', { error: message });
      process.exit(1);
    }
  });

// Permissions command
program
  .command('permissions')
  .description('Manage permissions')
  .argument('[action]', 'Action: list, create, delete', 'list')
  .option('-r, --resource <resource>', 'Resource pattern')
  .option('-a, --action <action>', 'Action name')
  .option('--description <desc>', 'Permission description')
  .option('-l, --limit <limit>', 'Number of records', '50')
  .action(async (action, options) => {
    try {
      const db = new ACLDatabase();
      await db.connect();

      switch (action) {
        case 'list': {
          const permissions = await db.listPermissions(parseInt(options.limit, 10));
          console.log('\nPermissions:');
          console.log('-'.repeat(100));
          permissions.forEach(p => {
            console.log(`${p.resource.padEnd(40)} | ${p.action.padEnd(20)} | ${p.description ?? 'No description'}`);
          });
          console.log(`\nTotal: ${await db.countPermissions()}`);
          break;
        }

        case 'create': {
          if (!options.resource || !options.action) {
            logger.error('--resource and --action required');
            process.exit(1);
          }

          const permission = await db.createPermission({
            resource: options.resource,
            action: options.action,
            description: options.description,
          });

          logger.success(`Created permission: ${permission.resource}:${permission.action} (${permission.id})`);
          break;
        }

        case 'delete': {
          if (!options.resource || !options.action) {
            logger.error('--resource and --action required');
            process.exit(1);
          }

          const permissions = await db.listPermissions(1000, 0);
          const permission = permissions.find(p => p.resource === options.resource && p.action === options.action);

          if (!permission) {
            logger.error('Permission not found');
            process.exit(1);
          }

          await db.deletePermission(permission.id);
          logger.success(`Deleted permission: ${options.resource}:${options.action}`);
          break;
        }

        default:
          logger.error(`Unknown action: ${action}`);
          process.exit(1);
      }

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Command failed', { error: message });
      process.exit(1);
    }
  });

// Users command
program
  .command('users')
  .description('Manage user roles')
  .argument('<user_id>', 'User ID')
  .argument('[action]', 'Action: list, assign, remove', 'list')
  .option('-r, --role <role>', 'Role name')
  .option('--scope <scope>', 'Role scope')
  .option('--scope-id <id>', 'Role scope ID')
  .action(async (userId, action, options) => {
    try {
      const db = new ACLDatabase();
      await db.connect();

      switch (action) {
        case 'list': {
          const roles = await db.getUserRoles(userId);
          console.log(`\nRoles for user ${userId}:`);
          console.log('-'.repeat(80));
          roles.forEach(r => {
            console.log(`  - ${r.name} (Level ${r.level})`);
          });

          const permissions = await db.getUserPermissions(userId);
          console.log(`\nEffective Permissions (${permissions.length}):`);
          permissions.forEach(p => {
            console.log(`  - ${p.resource}:${p.action}`);
          });
          break;
        }

        case 'assign': {
          if (!options.role) {
            logger.error('--role required');
            process.exit(1);
          }

          const role = await db.getRoleByName(options.role);
          if (!role) {
            logger.error(`Role "${options.role}" not found`);
            process.exit(1);
          }

          await db.assignRoleToUser(userId, {
            role_id: role.id,
            scope: options.scope,
            scope_id: options.scopeId,
          });

          logger.success(`Assigned role "${options.role}" to user ${userId}`);
          break;
        }

        case 'remove': {
          if (!options.role) {
            logger.error('--role required');
            process.exit(1);
          }

          const role = await db.getRoleByName(options.role);
          if (!role) {
            logger.error(`Role "${options.role}" not found`);
            process.exit(1);
          }

          await db.removeRoleFromUser(userId, role.id, options.scope, options.scopeId);
          logger.success(`Removed role "${options.role}" from user ${userId}`);
          break;
        }

        default:
          logger.error(`Unknown action: ${action}`);
          process.exit(1);
      }

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Command failed', { error: message });
      process.exit(1);
    }
  });

// Authorize command
program
  .command('authorize')
  .description('Check authorization')
  .argument('<user_id>', 'User ID')
  .argument('<resource>', 'Resource')
  .argument('<action>', 'Action')
  .option('-c, --context <json>', 'Context JSON')
  .action(async (userId, resource, action, options) => {
    try {
      const config = loadConfig();
      const db = new ACLDatabase();
      await db.connect();

      const authzEngine = new AuthorizationEngine(
        db,
        config.cacheTtlSeconds,
        config.maxRoleDepth,
        config.defaultDeny
      );

      let context: Record<string, unknown> = {};
      if (options.context) {
        try {
          context = JSON.parse(options.context);
        } catch {
          logger.error('Invalid context JSON');
          process.exit(1);
        }
      }

      const result = await authzEngine.authorize({
        user_id: userId,
        resource,
        action,
        context,
      });

      console.log('\nAuthorization Result:');
      console.log('=====================');
      console.log(`User:     ${userId}`);
      console.log(`Resource: ${resource}`);
      console.log(`Action:   ${action}`);
      console.log(`Allowed:  ${result.allowed ? 'YES' : 'NO'}`);
      console.log(`Reason:   ${result.reason}`);

      if (result.matched_permissions && result.matched_permissions.length > 0) {
        console.log(`Matched Permissions: ${result.matched_permissions.join(', ')}`);
      }

      if (result.matched_policies && result.matched_policies.length > 0) {
        console.log(`Matched Policies: ${result.matched_policies.join(', ')}`);
      }

      await db.disconnect();
      process.exit(result.allowed ? 0 : 1);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Authorization check failed', { error: message });
      process.exit(1);
    }
  });

// Policies command
program
  .command('policies')
  .description('Manage ABAC policies')
  .argument('[action]', 'Action: list, create, delete', 'list')
  .option('-n, --name <name>', 'Policy name')
  .option('-e, --effect <effect>', 'Effect: allow or deny')
  .option('-t, --type <type>', 'Principal type: role, user, group')
  .option('-v, --value <value>', 'Principal value')
  .option('-r, --resource <pattern>', 'Resource pattern')
  .option('-a, --action <pattern>', 'Action pattern')
  .option('-p, --priority <number>', 'Priority')
  .option('-l, --limit <limit>', 'Number of records', '50')
  .action(async (action, options) => {
    try {
      const db = new ACLDatabase();
      await db.connect();

      switch (action) {
        case 'list': {
          const policies = await db.listPolicies(parseInt(options.limit, 10));
          console.log('\nPolicies:');
          console.log('-'.repeat(120));
          policies.forEach(p => {
            const enabled = p.enabled ? '✓' : '✗';
            console.log(`${enabled} ${p.name.padEnd(30)} | ${p.effect.padEnd(5)} | ${p.principal_type}:${p.principal_value.padEnd(20)} | ${p.resource_pattern}:${p.action_pattern} | Priority: ${p.priority}`);
          });
          console.log(`\nTotal: ${await db.countPolicies()}`);
          break;
        }

        case 'create': {
          if (!options.name || !options.effect || !options.type || !options.value || !options.resource || !options.action) {
            logger.error('Required: --name, --effect, --type, --value, --resource, --action');
            process.exit(1);
          }

          if (!['allow', 'deny'].includes(options.effect)) {
            logger.error('Effect must be "allow" or "deny"');
            process.exit(1);
          }

          if (!['role', 'user', 'group'].includes(options.type)) {
            logger.error('Type must be "role", "user", or "group"');
            process.exit(1);
          }

          const policy = await db.createPolicy({
            name: options.name,
            effect: options.effect as 'allow' | 'deny',
            principal_type: options.type as 'role' | 'user' | 'group',
            principal_value: options.value,
            resource_pattern: options.resource,
            action_pattern: options.action,
            priority: options.priority ? parseInt(options.priority, 10) : 0,
          });

          logger.success(`Created policy: ${policy.name} (${policy.id})`);
          break;
        }

        case 'delete': {
          if (!options.name) {
            logger.error('--name required');
            process.exit(1);
          }

          const policies = await db.listPolicies(1000, 0);
          const policy = policies.find(p => p.name === options.name);

          if (!policy) {
            logger.error('Policy not found');
            process.exit(1);
          }

          await db.deletePolicy(policy.id);
          logger.success(`Deleted policy: ${options.name}`);
          break;
        }

        default:
          logger.error(`Unknown action: ${action}`);
          process.exit(1);
      }

      await db.disconnect();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Command failed', { error: message });
      process.exit(1);
    }
  });

program.parse();
