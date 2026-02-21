#!/usr/bin/env node
/**
 * VPN Plugin CLI
 * Complete command-line interface for VPN management
 */

import { Command } from 'commander';
import { createLogger } from '@nself/plugin-utils';
import chalk from 'chalk';
import ora from 'ora';
import { VPNDatabase } from './database.js';
import { getProvider, getSupportedProviders, providerMetadata } from './providers/index.js';
import { config } from './config.js';
import type { VPNProvider } from './types.js';

const logger = createLogger('vpn:cli');
const program = new Command();

// Initialize database
const db = new VPNDatabase(config.database_url);

// Encryption key for credentials (must be set via environment variable)
const ENCRYPTION_KEY = process.env.ENCRYPTION_KEY;
if (!ENCRYPTION_KEY) {
  throw new Error('ENCRYPTION_KEY environment variable is required. Set a strong random key for credential encryption.');
}

// ============================================================================
// Helper Functions
// ============================================================================

function formatBytes(bytes: bigint | number | string): string {
  const num = typeof bytes === 'string' ? BigInt(bytes) : BigInt(bytes);
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let value = Number(num);
  let unitIndex = 0;

  while (value >= 1024 && unitIndex < units.length - 1) {
    value /= 1024;
    unitIndex++;
  }

  return `${value.toFixed(2)} ${units[unitIndex]}`;
}

function formatDuration(seconds: number): string {
  const hours = Math.floor(seconds / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  const secs = seconds % 60;

  if (hours > 0) {
    return `${hours}h ${minutes}m ${secs}s`;
  } else if (minutes > 0) {
    return `${minutes}m ${secs}s`;
  } else {
    return `${secs}s`;
  }
}

// ============================================================================
// Commands
// ============================================================================

/**
 * Initialize plugin
 */
program
  .command('init')
  .description('Initialize VPN plugin and database schema')
  .action(async () => {
    const spinner = ora('Initializing VPN plugin...').start();

    try {
      // Initialize database schema
      await db.initializeSchema();
      spinner.succeed('Database schema initialized');

      // Initialize providers
      spinner.start('Initializing providers...');
      const providers = getSupportedProviders();

      for (const providerName of providers) {
        const metadata = providerMetadata[providerName];
        await db.upsertProvider({
          id: providerName,
          name: providerName,
          display_name: metadata.name,
          cli_available: metadata.cliRequired,
          api_available: false, // Will be set by provider
          port_forwarding_supported: metadata.portForwarding,
          p2p_all_servers: metadata.p2pServers.includes('All'),
          p2p_server_count: 0, // Will be updated on sync
          total_servers: 0,
          total_countries: 0,
          wireguard_supported: true,
          openvpn_supported: true,
          kill_switch_available: true,
          split_tunneling_available: false,
          config: {},
        });
      }

      spinner.succeed(`Initialized ${providers.length} providers`);

      console.log(chalk.green('\n✓ VPN plugin initialized successfully!'));
      console.log(chalk.gray('\nNext steps:'));
      console.log(chalk.gray('  1. Add provider credentials: npx tsx src/cli.ts providers add <provider> --token <token>'));
      console.log(chalk.gray('  2. Connect to VPN: npx tsx src/cli.ts connect <provider>'));
      console.log(chalk.gray('  3. Check status: npx tsx src/cli.ts status'));
    } catch (error) {
      spinner.fail('Initialization failed');
      console.error(chalk.red('\nError:'), error instanceof Error ? error.message : error);
      process.exit(1);
    }
  });

/**
 * Provider management commands
 */
const providersCmd = program.command('providers').description('Manage VPN provider credentials');

providersCmd
  .command('list')
  .description('List all supported VPN providers')
  .action(async () => {
    try {
      const providers = await db.getAllProviders();

      console.log(chalk.bold('\n📡 Supported VPN Providers\n'));

      for (const provider of providers) {
        const hasCredentials = await db.getCredentials(provider.id, ENCRYPTION_KEY);
        const status = hasCredentials ? chalk.green('✓ Configured') : chalk.gray('○ Not configured');
        const metadata = providerMetadata[provider.name as VPNProvider];

        console.log(chalk.bold(provider.display_name));
        console.log(`  Status: ${status}`);
        console.log(`  P2P Servers: ${chalk.cyan(metadata.p2pServers)}`);
        console.log(`  Port Forwarding: ${metadata.portForwarding ? chalk.green('✓ Yes') : chalk.gray('✗ No')}`);
        console.log(`  Notes: ${chalk.gray(metadata.notes)}`);
        console.log('');
      }
    } catch (error) {
      console.error(chalk.red('Error:'), error instanceof Error ? error.message : error);
      process.exit(1);
    }
  });

providersCmd
  .command('add <provider>')
  .description('Add provider credentials')
  .option('-t, --token <token>', 'Access token (NordVPN, ProtonVPN)')
  .option('-u, --username <username>', 'Username')
  .option('-p, --password <password>', 'Password')
  .option('-a, --account <account>', 'Account number (Mullvad)')
  .option('-k, --api-key <key>', 'API key')
  .action(async (providerName: string, options) => {
    const spinner = ora(`Adding credentials for ${providerName}...`).start();

    try {
      // Validate provider
      const provider = await db.getProvider(providerName);
      if (!provider) {
        throw new Error(`Provider '${providerName}' not found`);
      }

      // Store credentials
      await db.upsertCredentials(
        {
          provider_id: providerName,
          username: options.username,
          password_encrypted: options.password || '',
          api_token_encrypted: options.token || '',
          account_number: options.account,
          api_key_encrypted: options.apiKey || '',
        },
        ENCRYPTION_KEY
      );

      spinner.succeed(`Credentials added for ${provider.display_name}`);
      console.log(chalk.green('\n✓ Credentials stored securely (encrypted)'));
    } catch (error) {
      spinner.fail('Failed to add credentials');
      console.error(chalk.red('Error:'), error instanceof Error ? error.message : error);
      process.exit(1);
    }
  });

/**
 * Connect to VPN
 */
program
  .command('connect <provider>')
  .description('Connect to VPN provider')
  .option('-r, --region <region>', 'Region/country code (e.g., us, uk, nl)')
  .option('-c, --city <city>', 'City name')
  .option('-s, --server <server>', 'Specific server hostname')
  .option('-p, --protocol <protocol>', 'Protocol (wireguard, openvpn_udp, openvpn_tcp)')
  .option('--p2p', 'Connect to best P2P server')
  .option('--no-kill-switch', 'Disable kill switch')
  .option('--port-forwarding', 'Enable port forwarding (if supported)')
  .action(async (providerName: string, options) => {
    const spinner = ora(`Connecting to ${providerName}...`).start();

    try {
      // Get provider instance
      const provider = getProvider(providerName as VPNProvider);

      // Get credentials
      const credentials = await db.getCredentials(providerName, ENCRYPTION_KEY);
      if (!credentials) {
        throw new Error(`No credentials found for ${providerName}. Run: providers add ${providerName}`);
      }

      // Initialize and authenticate
      await provider.initialize();
      await provider.authenticate(credentials);

      // Connect
      const connection = await provider.connect(
        {
          provider: providerName as VPNProvider,
          region: options.region,
          city: options.city,
          server: options.server,
          protocol: options.protocol,
          kill_switch: options.killSwitch !== false,
          port_forwarding: options.portForwarding,
          requested_by: 'cli',
        },
        credentials
      );

      // Store connection in database
      await db.createConnection(connection);

      spinner.succeed(`Connected to ${providerName}`);

      // Display connection info
      console.log(chalk.green('\n✓ VPN Connected!\n'));
      console.log(chalk.bold('Connection Details:'));
      console.log(`  Provider: ${chalk.cyan(providerName)}`);
      if (connection.vpn_ip) console.log(`  VPN IP: ${chalk.cyan(connection.vpn_ip)}`);
      if (connection.interface_name) console.log(`  Interface: ${chalk.cyan(connection.interface_name)}`);
      if (connection.protocol) console.log(`  Protocol: ${chalk.cyan(connection.protocol)}`);
      console.log(`  Kill Switch: ${connection.kill_switch_enabled ? chalk.green('✓ Enabled') : chalk.gray('✗ Disabled')}`);
      if (connection.port_forwarded) console.log(`  Port Forwarded: ${chalk.green(connection.port_forwarded)}`);
    } catch (error) {
      spinner.fail('Connection failed');
      console.error(chalk.red('\nError:'), error instanceof Error ? error.message : error);
      process.exit(1);
    }
  });

/**
 * Disconnect from VPN
 */
program
  .command('disconnect')
  .description('Disconnect from current VPN connection')
  .action(async () => {
    const spinner = ora('Disconnecting...').start();

    try {
      // Get active connection
      const connection = await db.getActiveConnection();
      if (!connection) {
        spinner.info('No active VPN connection');
        return;
      }

      // Get provider and disconnect
      const provider = getProvider(connection.provider_id as VPNProvider);
      await provider.initialize();
      await provider.disconnect(connection.id);

      // Update database
      await db.updateConnection(connection.id, {
        status: 'disconnected',
        disconnected_at: new Date(),
        duration_seconds: Math.floor((Date.now() - connection.connected_at!.getTime()) / 1000),
      });

      spinner.succeed('Disconnected from VPN');
      console.log(chalk.green('\n✓ VPN disconnected'));
    } catch (error) {
      spinner.fail('Disconnect failed');
      console.error(chalk.red('\nError:'), error instanceof Error ? error.message : error);
      process.exit(1);
    }
  });

/**
 * Get connection status
 */
program
  .command('status')
  .description('Show current VPN connection status')
  .option('-v, --verbose', 'Show detailed information')
  .action(async (options) => {
    try {
      // Get active connection from database
      const connection = await db.getActiveConnection();

      if (!connection) {
        console.log(chalk.yellow('\n⚠ Not connected to VPN\n'));
        return;
      }

      // Get live status from provider
      const provider = getProvider(connection.provider_id as VPNProvider);
      await provider.initialize();
      const status = await provider.getStatus();

      console.log(chalk.bold('\n🔒 VPN Status\n'));
      console.log(`  Status: ${status.connected ? chalk.green('✓ Connected') : chalk.red('✗ Disconnected')}`);
      console.log(`  Provider: ${chalk.cyan(connection.provider_id)}`);
      if (status.server) console.log(`  Server: ${chalk.cyan(status.server)}`);
      if (status.vpn_ip) console.log(`  VPN IP: ${chalk.cyan(status.vpn_ip)}`);
      if (status.interface) console.log(`  Interface: ${chalk.cyan(status.interface)}`);
      if (status.protocol) console.log(`  Protocol: ${chalk.cyan(status.protocol)}`);
      if (status.uptime_seconds) console.log(`  Uptime: ${chalk.cyan(formatDuration(status.uptime_seconds))}`);

      if (status.bytes_sent) {
        console.log(`  Sent: ${chalk.cyan(formatBytes(status.bytes_sent))}`);
      }
      if (status.bytes_received) {
        console.log(`  Received: ${chalk.cyan(formatBytes(status.bytes_received))}`);
      }

      console.log(`  Kill Switch: ${status.kill_switch_enabled ? chalk.green('✓ Enabled') : chalk.gray('✗ Disabled')}`);
      if (status.port_forwarded) {
        console.log(`  Port Forwarded: ${chalk.green(status.port_forwarded)}`);
      }

      console.log('');
    } catch (error) {
      console.error(chalk.red('Error:'), error instanceof Error ? error.message : error);
      process.exit(1);
    }
  });

/**
 * List servers
 */
program
  .command('servers')
  .description('List available VPN servers')
  .option('-p, --provider <provider>', 'Filter by provider')
  .option('-c, --country <country>', 'Filter by country code')
  .option('--p2p', 'Show only P2P servers')
  .option('--port-forwarding', 'Show only servers with port forwarding')
  .option('-l, --limit <number>', 'Limit number of results', '20')
  .action(async (options) => {
    try {
      const servers = await db.getServers({
        provider: options.provider,
        country: options.country,
        p2p_only: options.p2p,
        port_forwarding: options.portForwarding,
        limit: parseInt(options.limit),
      });

      if (servers.length === 0) {
        console.log(chalk.yellow('\nNo servers found matching criteria\n'));
        console.log(chalk.gray('Try running: npx tsx src/cli.ts sync <provider>'));
        return;
      }

      console.log(chalk.bold(`\n📡 VPN Servers (${servers.length} results)\n`));

      for (const server of servers) {
        const p2pBadge = server.p2p_supported ? chalk.green('P2P') : '';
        const pfBadge = server.port_forwarding_supported ? chalk.blue('PF') : '';
        const loadColor = server.load ? (server.load < 30 ? chalk.green : server.load < 70 ? chalk.yellow : chalk.red) : chalk.gray;
        const load = server.load ? loadColor(`${server.load}%`) : chalk.gray('N/A');

        console.log(`${chalk.cyan(server.hostname)} ${p2pBadge} ${pfBadge}`);
        console.log(`  ${server.country_name} ${server.city ? `(${server.city})` : ''} - Load: ${load}`);
        console.log(`  IP: ${server.ip_address}`);
        console.log('');
      }
    } catch (error) {
      console.error(chalk.red('Error:'), error instanceof Error ? error.message : error);
      process.exit(1);
    }
  });

/**
 * Sync server list
 */
program
  .command('sync <provider>')
  .description('Sync server list from provider')
  .action(async (providerName: string) => {
    const spinner = ora(`Syncing servers from ${providerName}...`).start();

    try {
      const provider = getProvider(providerName as VPNProvider);

      // Get credentials if needed
      const credentials = await db.getCredentials(providerName, ENCRYPTION_KEY);

      // Initialize provider
      await provider.initialize();
      if (credentials) {
        await provider.authenticate(credentials);
      }

      // Fetch servers
      const servers = await provider.fetchServers();

      // Store in database
      let synced = 0;
      for (const server of servers) {
        await db.upsertServer(server);
        synced++;
      }

      spinner.succeed(`Synced ${synced} servers from ${providerName}`);
      console.log(chalk.green(`\n✓ ${synced} servers synced successfully`));
    } catch (error) {
      spinner.fail('Sync failed');
      console.error(chalk.red('\nError:'), error instanceof Error ? error.message : error);
      process.exit(1);
    }
  });

/**
 * Download via torrent
 */
program
  .command('download <magnet>')
  .description('Download file via torrent over VPN')
  .option('-p, --provider <provider>', 'VPN provider to use')
  .option('-r, --region <region>', 'Region/country code')
  .option('-d, --destination <path>', 'Download destination path')
  .action(async (magnetLink: string, options) => {
    console.log(chalk.yellow('\n⚠ Torrent download feature requires server to be running'));
    console.log(chalk.gray('Start server with: npx tsx src/index.ts'));
    console.log(chalk.gray('Then use: curl -X POST http://localhost:3200/api/download -d \'{"magnet_link":"..."}\''));
    console.log('');
  });

/**
 * Test for leaks
 */
program
  .command('test')
  .description('Test for DNS/IP leaks')
  .action(async () => {
    const spinner = ora('Testing for leaks...').start();

    try {
      // Get active connection
      const connection = await db.getActiveConnection();
      if (!connection) {
        spinner.fail('No active VPN connection');
        console.log(chalk.yellow('\n⚠ Connect to VPN first: npx tsx src/cli.ts connect <provider>\n'));
        return;
      }

      // Get provider and run leak test
      const provider = getProvider(connection.provider_id as VPNProvider);
      await provider.initialize();
      const result = await provider.testLeaks();

      if (result.passed) {
        spinner.succeed('No leaks detected');
        console.log(chalk.green('\n✓ All tests passed!\n'));
      } else {
        spinner.fail('Leaks detected');
        console.log(chalk.red('\n✗ Leaks detected!\n'));
      }

      // Show detailed results
      console.log(chalk.bold('Test Results:\n'));
      console.log(`  DNS Leak: ${result.tests.dns.passed ? chalk.green('✓ Pass') : chalk.red('✗ Fail')}`);
      if (result.tests.dns.actual) console.log(chalk.gray(`    Detected DNS: ${result.tests.dns.actual}`));

      console.log(`  IP Leak: ${result.tests.ip.passed ? chalk.green('✓ Pass') : chalk.red('✗ Fail')}`);
      if (result.tests.ip.actual) console.log(chalk.gray(`    Detected IP: ${result.tests.ip.actual}`));

      console.log(`  IPv6 Leak: ${result.tests.ipv6.passed ? chalk.green('✓ Pass') : chalk.red('✗ Fail')}`);
      if (result.tests.ipv6.leaked_ip) console.log(chalk.gray(`    Leaked IPv6: ${result.tests.ipv6.leaked_ip}`));

      console.log(`  WebRTC Leak: ${result.tests.webrtc.passed ? chalk.green('✓ Pass') : chalk.red('✗ Fail')}`);

      console.log('');

      // Store result in database
      await db.query(
        `INSERT INTO vpn_leak_tests (connection_id, test_type, passed, expected_value, actual_value)
         VALUES ($1, $2, $3, $4, $5)`,
        [connection.id, 'comprehensive', result.passed, 'no leaks', JSON.stringify(result.tests)]
      );
    } catch (error) {
      spinner.fail('Leak test failed');
      console.error(chalk.red('\nError:'), error instanceof Error ? error.message : error);
      process.exit(1);
    }
  });

/**
 * Show statistics
 */
program
  .command('stats')
  .description('Show VPN usage statistics')
  .action(async () => {
    try {
      const stats = await db.getStatistics();

      console.log(chalk.bold('\n📊 VPN Statistics\n'));
      console.log(chalk.bold('Overview:'));
      console.log(`  Total Connections: ${chalk.cyan(stats.total_connections)}`);
      console.log(`  Active Connections: ${chalk.green(stats.active_connections)}`);
      console.log(`  Total Downloads: ${chalk.cyan(stats.total_downloads)}`);
      console.log(`  Active Downloads: ${chalk.green(stats.active_downloads)}`);
      console.log(`  Total Data: ${chalk.cyan(formatBytes(stats.total_bytes_downloaded))}`);

      if (stats.providers.length > 0) {
        console.log(chalk.bold('\nProvider Usage:'));
        for (const provider of stats.providers.slice(0, 5)) {
          console.log(`  ${provider.provider}: ${chalk.cyan(provider.connections)} connections`);
        }
      }

      if (stats.top_servers.length > 0) {
        console.log(chalk.bold('\nTop Servers:'));
        for (const server of stats.top_servers.slice(0, 5)) {
          console.log(`  ${server.server} (${server.country}): ${chalk.cyan(server.connections)} connections`);
        }
      }

      console.log('');
    } catch (error) {
      console.error(chalk.red('Error:'), error instanceof Error ? error.message : error);
      process.exit(1);
    }
  });

// ============================================================================
// Program Configuration
// ============================================================================

program
  .name('vpn')
  .description('VPN Plugin for nself - Multi-provider VPN management')
  .version('1.0.0');

// Parse arguments
program.parse();
