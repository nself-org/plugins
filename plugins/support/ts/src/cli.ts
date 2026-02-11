#!/usr/bin/env node
/**
 * Support Plugin CLI
 * Command-line interface for helpdesk, ticketing, SLA, canned responses, knowledge base
 */

import { Command } from 'commander';
import { createLogger } from '@nself/plugin-utils';
import { loadConfig } from './config.js';
import { SupportDatabase } from './database.js';
import { startServer } from './server.js';

const logger = createLogger('support:cli');

const program = new Command();

program
  .name('nself-support')
  .description('Support plugin for nself - Helpdesk, ticketing, SLA, knowledge base')
  .version('1.0.0');

// =========================================================================
// Init Command
// =========================================================================

program
  .command('init')
  .description('Initialize support plugin schema')
  .action(async () => {
    try {
      logger.info('Initializing support schema...');
      const db = new SupportDatabase();
      await db.connect();
      await db.initializeSchema();
      console.log('✓ Support schema initialized successfully');
      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Initialization failed', { error: message });
      console.error(`✗ Error: ${message}`);
      process.exit(1);
    }
  });

// =========================================================================
// Server Command
// =========================================================================

program
  .command('server')
  .description('Start support plugin server')
  .option('-p, --port <port>', 'Server port', '3709')
  .action(async (options) => {
    try {
      logger.info('Starting support server...');
      await startServer({ port: parseInt(options.port, 10) });
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Server failed', { error: message });
      process.exit(1);
    }
  });

// =========================================================================
// Status Command
// =========================================================================

program
  .command('status')
  .description('Show support plugin status')
  .action(async () => {
    try {
      const config = loadConfig();
      const db = new SupportDatabase();
      await db.connect();

      const stats = await db.getStats();

      console.log('\nSupport Plugin Status');
      console.log('=======================');
      console.log(`Port:               ${config.port}`);
      console.log(`CSAT Enabled:       ${config.csatEnabled}`);
      console.log(`KB Enabled:         ${config.kbEnabled}`);
      console.log(`Auto Assignment:    ${config.autoAssignment}`);
      console.log(`Assignment Method:  ${config.assignmentMethod}`);
      console.log('');
      console.log('Ticket Statistics');
      console.log('-----------------');
      console.log(`Total Tickets:      ${stats.totalTickets}`);
      console.log(`Open:               ${stats.openTickets}`);
      console.log(`Pending:            ${stats.pendingTickets}`);
      console.log(`Resolved:           ${stats.resolvedTickets}`);
      console.log('');
      console.log('Resources');
      console.log('---------');
      console.log(`Teams:              ${stats.totalTeams}`);
      console.log(`Agents:             ${stats.totalAgents}`);
      console.log(`SLA Policies:       ${stats.totalSlaPolicies}`);
      console.log(`Canned Responses:   ${stats.totalCannedResponses}`);
      console.log(`KB Articles:        ${stats.totalKbArticles} (${stats.publishedKbArticles} published)`);

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Status failed', { error: message });
      console.error(`✗ Error: ${message}`);
      process.exit(1);
    }
  });

// =========================================================================
// Ticket Commands
// =========================================================================

program
  .command('ticket:create')
  .description('Create a new support ticket')
  .argument('<subject>', 'Ticket subject')
  .option('--description <description>', 'Ticket description')
  .option('--priority <priority>', 'Priority: low, medium, high, urgent', 'medium')
  .option('--assign <userId>', 'Assign to user')
  .option('--team <teamId>', 'Assign to team')
  .option('--category <category>', 'Ticket category')
  .option('--source <source>', 'Source: chat, email, api, web_form', 'chat')
  .action(async (subject, options) => {
    try {
      const db = new SupportDatabase();
      await db.connect();

      const ticket = await db.createTicket({
        subject,
        description: options.description ?? subject,
        priority: options.priority,
        assignedTo: options.assign,
        teamId: options.team,
        category: options.category,
        source: options.source,
      });

      console.log(`✓ Ticket created: ${ticket.ticket_number}`);
      console.log(`  ID:       ${ticket.id}`);
      console.log(`  Status:   ${ticket.status}`);
      console.log(`  Priority: ${ticket.priority}`);
      if (ticket.first_response_due_at) {
        console.log(`  First response due: ${new Date(ticket.first_response_due_at).toISOString()}`);
      }
      if (ticket.resolution_due_at) {
        console.log(`  Resolution due:     ${new Date(ticket.resolution_due_at).toISOString()}`);
      }

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to create ticket', { error: message });
      console.error(`✗ Error: ${message}`);
      process.exit(1);
    }
  });

program
  .command('tickets:list')
  .description('List support tickets')
  .option('--status <status>', 'Filter by status')
  .option('--priority <priority>', 'Filter by priority')
  .option('--assigned-to <userId>', 'Filter by assigned agent')
  .option('--team <teamId>', 'Filter by team')
  .option('--search <query>', 'Search subject/description')
  .option('-l, --limit <limit>', 'Result limit', '20')
  .action(async (options) => {
    try {
      const db = new SupportDatabase();
      await db.connect();

      const tickets = await db.listTickets({
        status: options.status,
        priority: options.priority,
        assignedTo: options.assignedTo,
        teamId: options.team,
        search: options.search,
        limit: parseInt(options.limit, 10),
      });

      console.log(`\nSupport Tickets (${tickets.length}):`);
      console.log('='.repeat(80));

      if (tickets.length === 0) {
        console.log('No tickets found.');
      } else {
        for (const t of tickets) {
          const breachFlag = t.first_response_breached || t.resolution_breached ? ' [SLA BREACH]' : '';
          console.log(`  ${t.ticket_number} [${t.status}] [${t.priority}] ${t.subject}${breachFlag}`);
          console.log(`    Created: ${new Date(t.created_at).toISOString()} | Assigned: ${t.assigned_to ?? 'Unassigned'}`);
        }
      }

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to list tickets', { error: message });
      console.error(`✗ Error: ${message}`);
      process.exit(1);
    }
  });

program
  .command('ticket:info')
  .description('Get ticket details')
  .argument('<ticketId>', 'Ticket ID or number')
  .action(async (ticketId) => {
    try {
      const db = new SupportDatabase();
      await db.connect();

      let ticket = await db.getTicket(ticketId);
      if (!ticket) ticket = await db.getTicketByNumber(ticketId);
      if (!ticket) {
        console.error(`✗ Ticket not found: ${ticketId}`);
        process.exit(1);
      }

      console.log(`\nTicket: ${ticket.ticket_number}`);
      console.log('='.repeat(60));
      console.log(`ID:              ${ticket.id}`);
      console.log(`Subject:         ${ticket.subject}`);
      console.log(`Status:          ${ticket.status}`);
      console.log(`Priority:        ${ticket.priority}`);
      console.log(`Source:          ${ticket.source}`);
      console.log(`Category:        ${ticket.category ?? 'N/A'}`);
      console.log(`Tags:            ${ticket.tags.length > 0 ? ticket.tags.join(', ') : 'None'}`);
      console.log(`Assigned To:     ${ticket.assigned_to ?? 'Unassigned'}`);
      console.log(`Team:            ${ticket.team_id ?? 'None'}`);
      console.log(`Customer:        ${ticket.customer_name ?? ticket.customer_email ?? ticket.customer_id ?? 'Unknown'}`);
      console.log('');
      console.log('SLA Tracking');
      console.log('------------');
      console.log(`First Response Due:  ${ticket.first_response_due_at ? new Date(ticket.first_response_due_at).toISOString() : 'N/A'}`);
      console.log(`First Response At:   ${ticket.first_response_at ? new Date(ticket.first_response_at).toISOString() : 'Not yet'}`);
      console.log(`First Response Breach: ${ticket.first_response_breached ? 'YES' : 'No'}`);
      console.log(`Resolution Due:      ${ticket.resolution_due_at ? new Date(ticket.resolution_due_at).toISOString() : 'N/A'}`);
      console.log(`Resolved At:         ${ticket.resolved_at ? new Date(ticket.resolved_at).toISOString() : 'Not yet'}`);
      console.log(`Resolution Breach:   ${ticket.resolution_breached ? 'YES' : 'No'}`);
      if (ticket.satisfaction_rating) {
        console.log('');
        console.log(`Satisfaction Rating: ${ticket.satisfaction_rating}/5`);
        if (ticket.satisfaction_comment) console.log(`Comment: ${ticket.satisfaction_comment}`);
      }
      console.log('');
      console.log(`Created:  ${new Date(ticket.created_at).toISOString()}`);
      console.log(`Updated:  ${new Date(ticket.updated_at).toISOString()}`);
      if (ticket.closed_at) console.log(`Closed:   ${new Date(ticket.closed_at).toISOString()}`);

      // List messages
      const messages = await db.listTicketMessages(ticket.id);
      if (messages.length > 0) {
        console.log(`\nMessages (${messages.length}):`);
        console.log('-'.repeat(40));
        for (const msg of messages) {
          const prefix = msg.is_internal ? '[INTERNAL] ' : msg.is_system ? '[SYSTEM] ' : '';
          console.log(`  ${prefix}${msg.user_id ?? 'System'} (${new Date(msg.created_at).toISOString()}):`);
          console.log(`    ${msg.content.substring(0, 200)}${msg.content.length > 200 ? '...' : ''}`);
        }
      }

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to get ticket info', { error: message });
      console.error(`✗ Error: ${message}`);
      process.exit(1);
    }
  });

program
  .command('ticket:update')
  .description('Update a ticket')
  .argument('<ticketId>', 'Ticket ID')
  .option('--status <status>', 'New status')
  .option('--priority <priority>', 'New priority')
  .option('--assign <userId>', 'Assign to user')
  .option('--team <teamId>', 'Move to team')
  .option('--category <category>', 'Set category')
  .action(async (ticketId, options) => {
    try {
      const db = new SupportDatabase();
      await db.connect();

      const ticket = await db.updateTicket(ticketId, {
        status: options.status,
        priority: options.priority,
        assignedTo: options.assign,
        teamId: options.team,
        category: options.category,
      });

      if (!ticket) {
        console.error(`✗ Ticket not found: ${ticketId}`);
        process.exit(1);
      }

      console.log(`✓ Ticket updated: ${ticket.ticket_number}`);
      console.log(`  Status:   ${ticket.status}`);
      console.log(`  Priority: ${ticket.priority}`);

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to update ticket', { error: message });
      console.error(`✗ Error: ${message}`);
      process.exit(1);
    }
  });

program
  .command('ticket:close')
  .description('Close a ticket')
  .argument('<ticketId>', 'Ticket ID')
  .action(async (ticketId) => {
    try {
      const db = new SupportDatabase();
      await db.connect();

      const ticket = await db.updateTicket(ticketId, { status: 'closed' });
      if (!ticket) {
        console.error(`✗ Ticket not found: ${ticketId}`);
        process.exit(1);
      }

      console.log(`✓ Ticket closed: ${ticket.ticket_number}`);

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to close ticket', { error: message });
      console.error(`✗ Error: ${message}`);
      process.exit(1);
    }
  });

program
  .command('ticket:comment')
  .description('Add a comment to a ticket')
  .argument('<ticketId>', 'Ticket ID')
  .argument('<message>', 'Comment text')
  .option('--internal', 'Mark as internal note')
  .option('--user <userId>', 'User ID of commenter')
  .action(async (ticketId, content, options) => {
    try {
      const db = new SupportDatabase();
      await db.connect();

      const msg = await db.createTicketMessage({
        ticketId,
        content,
        userId: options.user,
        isInternal: options.internal ?? false,
      });

      const prefix = msg.is_internal ? 'Internal note' : 'Comment';
      console.log(`✓ ${prefix} added to ticket`);

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to add comment', { error: message });
      console.error(`✗ Error: ${message}`);
      process.exit(1);
    }
  });

// =========================================================================
// Team Commands
// =========================================================================

program
  .command('team:create')
  .description('Create a support team')
  .argument('<name>', 'Team name')
  .option('--email <email>', 'Team email')
  .option('--timezone <tz>', 'Team timezone', 'UTC')
  .option('--assignment <method>', 'Assignment method: round_robin, load_balanced, skill_based', 'round_robin')
  .action(async (name, options) => {
    try {
      const db = new SupportDatabase();
      await db.connect();

      const team = await db.createTeam({
        name,
        email: options.email,
        timezone: options.timezone,
        assignmentMethod: options.assignment,
      });

      console.log(`✓ Team created: ${team.name}`);
      console.log(`  ID: ${team.id}`);

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to create team', { error: message });
      console.error(`✗ Error: ${message}`);
      process.exit(1);
    }
  });

program
  .command('teams:list')
  .description('List support teams')
  .action(async () => {
    try {
      const db = new SupportDatabase();
      await db.connect();

      const teams = await db.listTeams();

      console.log(`\nSupport Teams (${teams.length}):`);
      console.log('='.repeat(60));

      if (teams.length === 0) {
        console.log('No teams found.');
      } else {
        for (const t of teams) {
          const status = t.is_active ? '✓' : '✗';
          console.log(`  ${status} ${t.name} (${t.member_count} members, ${t.open_tickets_count} open tickets)`);
          console.log(`    Assignment: ${t.assignment_method} | Timezone: ${t.timezone}`);
          if (t.email) console.log(`    Email: ${t.email}`);
        }
      }

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to list teams', { error: message });
      console.error(`✗ Error: ${message}`);
      process.exit(1);
    }
  });

program
  .command('team:add-member')
  .description('Add a member to a team')
  .argument('<teamId>', 'Team ID')
  .argument('<userId>', 'User ID')
  .option('--role <role>', 'Role: agent, lead, manager', 'agent')
  .option('--skills <skills>', 'Comma-separated skills')
  .action(async (teamId, userId, options) => {
    try {
      const db = new SupportDatabase();
      await db.connect();

      const member = await db.addTeamMember({
        teamId,
        userId,
        role: options.role,
        skills: options.skills ? options.skills.split(',').map((s: string) => s.trim()) : undefined,
      });

      console.log(`✓ Member added to team`);
      console.log(`  Member ID: ${member.id}`);
      console.log(`  Role: ${member.role}`);

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to add team member', { error: message });
      console.error(`✗ Error: ${message}`);
      process.exit(1);
    }
  });

program
  .command('agent:availability')
  .description('Update agent availability status')
  .argument('<memberId>', 'Team member ID')
  .argument('<status>', 'Status: available, busy, away, offline')
  .action(async (memberId, status) => {
    try {
      const db = new SupportDatabase();
      await db.connect();

      const isAvailable = status === 'available';
      const member = await db.updateTeamMember(memberId, {
        availabilityStatus: status,
        isAvailable,
      });

      if (!member) {
        console.error(`✗ Team member not found: ${memberId}`);
        process.exit(1);
      }

      console.log(`✓ Agent availability updated: ${status}`);

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to update availability', { error: message });
      console.error(`✗ Error: ${message}`);
      process.exit(1);
    }
  });

// =========================================================================
// SLA Commands
// =========================================================================

program
  .command('sla:create')
  .description('Create an SLA policy')
  .argument('<name>', 'Policy name')
  .option('--urgent-response <min>', 'Urgent first response minutes', '15')
  .option('--urgent-resolution <min>', 'Urgent resolution minutes', '240')
  .option('--high-response <min>', 'High first response minutes', '60')
  .option('--high-resolution <min>', 'High resolution minutes', '480')
  .option('--default', 'Set as default policy')
  .action(async (name, options) => {
    try {
      const db = new SupportDatabase();
      await db.connect();

      const policy = await db.createSlaPolicy({
        name,
        urgentFirstResponseMinutes: parseInt(options.urgentResponse, 10),
        urgentResolutionMinutes: parseInt(options.urgentResolution, 10),
        highFirstResponseMinutes: parseInt(options.highResponse, 10),
        highResolutionMinutes: parseInt(options.highResolution, 10),
        isDefault: options.default ?? false,
      });

      console.log(`✓ SLA policy created: ${policy.name}`);
      console.log(`  ID: ${policy.id}`);
      console.log(`  Default: ${policy.is_default}`);

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to create SLA policy', { error: message });
      console.error(`✗ Error: ${message}`);
      process.exit(1);
    }
  });

program
  .command('sla:list')
  .description('List SLA policies')
  .action(async () => {
    try {
      const db = new SupportDatabase();
      await db.connect();

      const policies = await db.listSlaPolicies();

      console.log(`\nSLA Policies (${policies.length}):`);
      console.log('='.repeat(80));

      if (policies.length === 0) {
        console.log('No SLA policies found.');
      } else {
        for (const p of policies) {
          const defaultTag = p.is_default ? ' [DEFAULT]' : '';
          const activeTag = p.is_active ? '' : ' [INACTIVE]';
          console.log(`  ${p.name}${defaultTag}${activeTag}`);
          console.log(`    Urgent: ${p.urgent_first_response_minutes}m response, ${p.urgent_resolution_minutes}m resolution`);
          console.log(`    High:   ${p.high_first_response_minutes}m response, ${p.high_resolution_minutes}m resolution`);
          console.log(`    Medium: ${p.medium_first_response_minutes}m response, ${p.medium_resolution_minutes}m resolution`);
          console.log(`    Low:    ${p.low_first_response_minutes}m response, ${p.low_resolution_minutes}m resolution`);
        }
      }

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to list SLA policies', { error: message });
      console.error(`✗ Error: ${message}`);
      process.exit(1);
    }
  });

program
  .command('sla:set-default')
  .description('Set a policy as the default SLA')
  .argument('<policyId>', 'SLA policy ID')
  .action(async (policyId) => {
    try {
      const db = new SupportDatabase();
      await db.connect();

      const policy = await db.updateSlaPolicy(policyId, { isDefault: true });
      if (!policy) {
        console.error(`✗ SLA policy not found: ${policyId}`);
        process.exit(1);
      }

      console.log(`✓ Default SLA policy set: ${policy.name}`);

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to set default SLA', { error: message });
      console.error(`✗ Error: ${message}`);
      process.exit(1);
    }
  });

// =========================================================================
// Canned Responses Commands
// =========================================================================

program
  .command('canned:create')
  .description('Create a canned response')
  .argument('<title>', 'Response title')
  .argument('<shortcut>', 'Shortcut (e.g. /greeting)')
  .argument('<content>', 'Response content')
  .option('--category <category>', 'Category')
  .option('--user <userId>', 'Created by user ID')
  .action(async (title, shortcut, content, options) => {
    try {
      const db = new SupportDatabase();
      await db.connect();

      if (!options.user) {
        console.error('✗ --user is required');
        process.exit(1);
      }

      const response = await db.createCannedResponse({
        title,
        shortcut,
        content,
        category: options.category,
        createdBy: options.user,
      });

      console.log(`✓ Canned response created: ${response.title}`);
      console.log(`  Shortcut: ${response.shortcut}`);

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to create canned response', { error: message });
      console.error(`✗ Error: ${message}`);
      process.exit(1);
    }
  });

program
  .command('canned:list')
  .description('List canned responses')
  .option('--category <category>', 'Filter by category')
  .option('--search <query>', 'Search responses')
  .action(async (options) => {
    try {
      const db = new SupportDatabase();
      await db.connect();

      const responses = await db.listCannedResponses({
        category: options.category,
        search: options.search,
      });

      console.log(`\nCanned Responses (${responses.length}):`);
      console.log('='.repeat(60));

      if (responses.length === 0) {
        console.log('No canned responses found.');
      } else {
        for (const r of responses) {
          console.log(`  ${r.shortcut ?? '(no shortcut)'} - ${r.title} (used ${r.usage_count} times)`);
          console.log(`    ${r.content.substring(0, 100)}${r.content.length > 100 ? '...' : ''}`);
        }
      }

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to list canned responses', { error: message });
      console.error(`✗ Error: ${message}`);
      process.exit(1);
    }
  });

// =========================================================================
// Knowledge Base Commands
// =========================================================================

program
  .command('kb:create')
  .description('Create a knowledge base article')
  .argument('<title>', 'Article title')
  .option('--content <content>', 'Article content')
  .option('--category <category>', 'Article category')
  .option('--author <authorId>', 'Author user ID')
  .option('--slug <slug>', 'Custom URL slug')
  .action(async (title, options) => {
    try {
      const db = new SupportDatabase();
      await db.connect();

      if (!options.author) {
        console.error('✗ --author is required');
        process.exit(1);
      }

      const article = await db.createKbArticle({
        title,
        content: options.content ?? '',
        authorId: options.author,
        category: options.category,
        slug: options.slug,
      });

      console.log(`✓ KB article created: ${article.title}`);
      console.log(`  Slug: ${article.slug}`);
      console.log(`  Published: ${article.is_published}`);

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to create KB article', { error: message });
      console.error(`✗ Error: ${message}`);
      process.exit(1);
    }
  });

program
  .command('kb:list')
  .description('List knowledge base articles')
  .option('--category <category>', 'Filter by category')
  .option('--published', 'Show only published articles')
  .option('--search <query>', 'Search articles')
  .action(async (options) => {
    try {
      const db = new SupportDatabase();
      await db.connect();

      const articles = await db.listKbArticles({
        category: options.category,
        published: options.published ?? undefined,
        search: options.search,
      });

      console.log(`\nKB Articles (${articles.length}):`);
      console.log('='.repeat(60));

      if (articles.length === 0) {
        console.log('No articles found.');
      } else {
        for (const a of articles) {
          const pub = a.is_published ? '✓' : '✗';
          console.log(`  ${pub} ${a.title} (${a.view_count} views, ${a.helpful_count} helpful)`);
          console.log(`    /${a.slug} | Category: ${a.category ?? 'None'}`);
        }
      }

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to list KB articles', { error: message });
      console.error(`✗ Error: ${message}`);
      process.exit(1);
    }
  });

program
  .command('kb:publish')
  .description('Publish a knowledge base article')
  .argument('<articleId>', 'Article ID')
  .action(async (articleId) => {
    try {
      const db = new SupportDatabase();
      await db.connect();

      const article = await db.updateKbArticle(articleId, { isPublished: true });
      if (!article) {
        console.error(`✗ Article not found: ${articleId}`);
        process.exit(1);
      }

      console.log(`✓ Article published: ${article.title}`);

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to publish article', { error: message });
      console.error(`✗ Error: ${message}`);
      process.exit(1);
    }
  });

program
  .command('kb:search')
  .description('Search knowledge base articles')
  .argument('<query>', 'Search query')
  .option('-l, --limit <limit>', 'Result limit', '10')
  .action(async (query, options) => {
    try {
      const db = new SupportDatabase();
      await db.connect();

      const articles = await db.listKbArticles({
        search: query,
        published: true,
        limit: parseInt(options.limit, 10),
      });

      console.log(`\nKB Search Results (${articles.length}):`);
      console.log('='.repeat(60));

      if (articles.length === 0) {
        console.log('No articles found.');
      } else {
        for (const a of articles) {
          console.log(`  ${a.title}`);
          console.log(`    /${a.slug} | ${a.view_count} views`);
          if (a.summary) console.log(`    ${a.summary.substring(0, 120)}${a.summary.length > 120 ? '...' : ''}`);
        }
      }

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to search KB', { error: message });
      console.error(`✗ Error: ${message}`);
      process.exit(1);
    }
  });

// =========================================================================
// Analytics Commands
// =========================================================================

program
  .command('dashboard')
  .description('View support dashboard')
  .action(async () => {
    try {
      const db = new SupportDatabase();
      await db.connect();

      const analytics = await db.getAnalyticsOverview();
      const stats = await db.getStats();

      console.log('\nSupport Dashboard');
      console.log('==================');
      console.log(`Open Tickets:          ${analytics.openTickets}`);
      console.log(`Avg First Response:    ${analytics.avgFirstResponseTime > 0 ? `${(analytics.avgFirstResponseTime / 60).toFixed(1)} min` : 'N/A'}`);
      console.log(`Avg Resolution Time:   ${analytics.avgResolutionTime > 0 ? `${(analytics.avgResolutionTime / 3600).toFixed(1)} hrs` : 'N/A'}`);
      console.log(`SLA Compliance:        ${analytics.slaCompliance > 0 ? `${(analytics.slaCompliance * 100).toFixed(1)}%` : 'N/A'}`);
      console.log(`Customer Satisfaction: ${analytics.customerSatisfaction > 0 ? `${analytics.customerSatisfaction.toFixed(1)}/5` : 'N/A'}`);
      console.log('');
      console.log('Tickets by Status:');
      for (const [status, count] of Object.entries(analytics.ticketsByStatus)) {
        console.log(`  ${status}: ${count}`);
      }
      console.log('');
      console.log('Tickets by Priority:');
      for (const [priority, count] of Object.entries(analytics.ticketsByPriority)) {
        console.log(`  ${priority}: ${count}`);
      }
      console.log('');
      console.log(`Teams: ${stats.totalTeams} | Agents: ${stats.totalAgents} | KB Articles: ${stats.publishedKbArticles}`);

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to load dashboard', { error: message });
      console.error(`✗ Error: ${message}`);
      process.exit(1);
    }
  });

program
  .command('leaderboard')
  .description('View agent leaderboard')
  .option('-l, --limit <limit>', 'Number of agents to show', '10')
  .action(async (options) => {
    try {
      const db = new SupportDatabase();
      await db.connect();

      const agents = await db.getAgentPerformance();
      const limit = parseInt(options.limit, 10);
      const top = agents.slice(0, limit);

      console.log(`\nAgent Leaderboard (top ${Math.min(limit, top.length)}):`);
      console.log('='.repeat(80));

      if (top.length === 0) {
        console.log('No agents found.');
      } else {
        for (let i = 0; i < top.length; i++) {
          const a = top[i];
          console.log(`  ${i + 1}. User ${a.userId}`);
          console.log(`     Tickets: ${a.ticketsHandled} | Current: ${a.currentTickets} | Satisfaction: ${a.satisfactionAvg > 0 ? `${a.satisfactionAvg.toFixed(1)}/5` : 'N/A'}`);
        }
      }

      await db.disconnect();
      process.exit(0);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error('Failed to load leaderboard', { error: message });
      console.error(`✗ Error: ${message}`);
      process.exit(1);
    }
  });

// =========================================================================
// Health Command
// =========================================================================

program
  .command('health')
  .description('Check support plugin health')
  .option('-p, --port <port>', 'Server port', '3709')
  .action(async (options) => {
    try {
      const response = await fetch(`http://localhost:${options.port}/health`);
      const data = await response.json() as Record<string, unknown>;
      console.log('Health:', JSON.stringify(data, null, 2));
      process.exit(response.ok ? 0 : 1);
    } catch {
      console.error('✗ Support server is not reachable');
      process.exit(1);
    }
  });

program.parse();
