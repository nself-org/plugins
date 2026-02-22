/**
 * ExpressVPN Provider Implementation
 * Full support for ExpressVPN CLI and API
 */

import { BaseVPNProvider } from './base.js';
import { createLogger } from '@nself/plugin-utils';
import type {
  ConnectVPNRequest,
  VPNConnectionRecord,
  VPNCredentialRecord,
  VPNServerRecord,
  VPNStatus,
  VPNProtocol,
} from '../types.js';

const logger = createLogger('vpn:expressvpn');

interface ExpressVPNLocation {
  id: string;
  name: string;
  country: string;
  country_code: string;
  region?: string;
  recommended: boolean;
}

interface ExpressVPNServerListResponse {
  locations: ExpressVPNLocation[];
}

export class ExpressVPNProvider extends BaseVPNProvider {
  readonly name = 'expressvpn' as const;
  readonly displayName = 'ExpressVPN';
  readonly cliAvailable = true;
  readonly apiAvailable = true;
  readonly portForwardingSupported = false;
  readonly p2pAllServers = true;

  private readonly apiEndpoint = 'https://api.expressvpn.com/v1';
  private readonly cliCommand = 'expressvpn';

  // ============================================================================
  // Initialization
  // ============================================================================

  protected async checkCLIInstalled(): Promise<void> {
    try {
      await this.executeCommand(`${this.cliCommand} --version`);
      logger.info('ExpressVPN CLI is installed');
    } catch (error) {
      throw new Error(
        'ExpressVPN CLI is not installed. Install from: https://www.expressvpn.com/support/vpn-setup/app-for-linux/'
      );
    }
  }

  protected async performAuthentication(credentials: VPNCredentialRecord): Promise<boolean> {
    try {
      // Check if already activated
      try {
        const statusResult = await this.executeCommand(`${this.cliCommand} status`);
        if (statusResult.stdout.includes('Activated') || statusResult.stdout.includes('Connected')) {
          logger.info('ExpressVPN is already activated');
          return true;
        }
      } catch {
        // Not authenticated yet — proceed
      }

      if (!credentials.api_token_encrypted) {
        throw new Error('ExpressVPN requires an activation code (api_token_encrypted)');
      }

      // Activate with activation code
      await this.executeCommand(
        `${this.cliCommand} activate --code ${credentials.api_token_encrypted}`,
        30000
      );

      // Verify activation
      const checkResult = await this.executeCommand(`${this.cliCommand} status`);
      if (!checkResult.stdout.toLowerCase().includes('activated')) {
        throw new Error('Activation verification failed');
      }

      logger.info('Successfully activated ExpressVPN');
      return true;
    } catch (error) {
      logger.error('ExpressVPN authentication failed', {
        error: error instanceof Error ? error.message : String(error),
      });
      return false;
    }
  }

  // ============================================================================
  // Server Management
  // ============================================================================

  async fetchServers(): Promise<VPNServerRecord[]> {
    logger.info('Fetching ExpressVPN server locations from API');

    try {
      const response = await fetch(`${this.apiEndpoint}/vpn_locations`, {
        headers: { 'Content-Type': 'application/json' },
      });

      if (!response.ok) {
        // Fall back to CLI location list
        logger.warn('API unavailable, falling back to CLI location list');
        return this.fetchServersFromCLI();
      }

      const data: ExpressVPNServerListResponse = await response.json();
      const locations = data.locations ?? [];
      logger.info(`Fetched ${locations.length} locations from ExpressVPN`);

      return locations.map((loc) => this.mapLocation(loc));
    } catch (error) {
      logger.warn('Failed to fetch from API, falling back to CLI', {
        error: error instanceof Error ? error.message : String(error),
      });
      return this.fetchServersFromCLI();
    }
  }

  private async fetchServersFromCLI(): Promise<VPNServerRecord[]> {
    try {
      const result = await this.executeCommand(`${this.cliCommand} list all`);
      const lines = result.stdout
        .split('\n')
        .map((l) => l.trim())
        .filter((l) => l.length > 0 && !l.startsWith('ALIAS'));

      const servers: VPNServerRecord[] = [];

      for (const line of lines) {
        // Format: ALIAS    LOCATION        COUNTRY
        const parts = line.split(/\s{2,}/);
        if (parts.length < 2) continue;

        const alias = parts[0]?.trim() ?? '';
        const location = parts[1]?.trim() ?? alias;
        const country = parts[2]?.trim() ?? location;

        // Derive a rough country code from alias (e.g., "usnc" -> "US")
        const countryCode = alias.replace(/\d+/g, '').substring(0, 2).toUpperCase();

        servers.push({
          id: `expressvpn-${alias}`,
          provider_id: 'expressvpn',
          hostname: `${alias}.expressvpn.com`,
          ip_address: '',
          country_code: countryCode,
          country_name: country,
          city: location,
          p2p_supported: true,
          port_forwarding_supported: false,
          protocols: ['lightway', 'openvpn_udp', 'openvpn_tcp', 'ikev2'],
          status: 'online',
          features: ['p2p'],
          owned: true,
          metadata: { alias },
          last_seen: new Date(),
          created_at: new Date(),
          updated_at: new Date(),
        } as VPNServerRecord);
      }

      return servers;
    } catch (error) {
      logger.error('Failed to fetch ExpressVPN servers from CLI', {
        error: error instanceof Error ? error.message : String(error),
      });
      throw error;
    }
  }

  private mapLocation(loc: ExpressVPNLocation): VPNServerRecord {
    return {
      id: `expressvpn-${loc.id}`,
      provider_id: 'expressvpn',
      hostname: `${loc.id}.expressvpn.com`,
      ip_address: '',
      country_code: (loc.country_code ?? '').toUpperCase(),
      country_name: loc.country,
      city: loc.name,
      region: loc.region,
      p2p_supported: true,
      port_forwarding_supported: false,
      protocols: ['lightway', 'openvpn_udp', 'openvpn_tcp', 'ikev2'],
      status: 'online',
      features: loc.recommended ? ['p2p', 'recommended'] : ['p2p'],
      owned: true,
      metadata: { recommended: loc.recommended },
      last_seen: new Date(),
      created_at: new Date(),
      updated_at: new Date(),
    } as VPNServerRecord;
  }

  // ============================================================================
  // Connection Management
  // ============================================================================

  async connect(request: ConnectVPNRequest, credentials: VPNCredentialRecord): Promise<VPNConnectionRecord> {
    this.ensureAuthenticated();

    logger.info('Connecting to ExpressVPN', {
      region: request.region,
      server: request.server,
      protocol: request.protocol,
    });

    // Set protocol if specified
    if (request.protocol) {
      const proto = this.mapProtocol(request.protocol);
      await this.executeCommand(`${this.cliCommand} protocol ${proto}`);
    }

    // Build connect command
    let connectCmd = `${this.cliCommand} connect`;

    if (request.server) {
      // Specific server alias
      connectCmd += ` ${request.server}`;
    } else if (request.region) {
      // Country or location
      connectCmd += ` ${request.region}`;
    } else {
      // Smart Location (recommended)
      connectCmd += ' smart';
    }

    try {
      const result = await this.executeCommand(connectCmd, 60000);
      logger.info('ExpressVPN connection established', { output: result.stdout });

      const status = await this.getStatus();

      const connection: VPNConnectionRecord = {
        id: `expressvpn-${Date.now()}`,
        provider_id: 'expressvpn',
        server_id: undefined,
        protocol: (request.protocol || 'lightway') as VPNProtocol,
        status: 'connected',
        local_ip: undefined,
        vpn_ip: status.vpn_ip,
        interface_name: status.interface,
        dns_servers: [],
        connected_at: new Date(),
        kill_switch_enabled: request.kill_switch !== false,
        requested_by: request.requested_by,
        metadata: {},
        created_at: new Date(),
      };

      return connection;
    } catch (error) {
      logger.error('ExpressVPN connection failed', {
        error: error instanceof Error ? error.message : String(error),
      });
      throw new Error(`Failed to connect to ExpressVPN: ${error}`);
    }
  }

  async disconnect(connectionId: string): Promise<void> {
    logger.info('Disconnecting from ExpressVPN', { connectionId });

    try {
      await this.executeCommand(`${this.cliCommand} disconnect`);
      logger.info('Successfully disconnected from ExpressVPN');
    } catch (error) {
      logger.error('Failed to disconnect from ExpressVPN', {
        error: error instanceof Error ? error.message : String(error),
      });
      throw error;
    }
  }

  async getStatus(): Promise<VPNStatus> {
    try {
      const result = await this.executeCommand(`${this.cliCommand} status`);
      return this.parseExpressVPNStatus(result.stdout);
    } catch (error) {
      logger.error('Failed to get ExpressVPN status', {
        error: error instanceof Error ? error.message : String(error),
      });
      return { connected: false };
    }
  }

  private parseExpressVPNStatus(output: string): VPNStatus {
    const lines = output.split('\n');
    const status: VPNStatus = { connected: false };

    for (const line of lines) {
      const trimmed = line.trim();

      if (trimmed.toLowerCase().includes('connected to')) {
        status.connected = true;
        // Extract server name: "Connected to United States - New York"
        const match = trimmed.match(/connected to (.+)/i);
        if (match) {
          status.server = match[1].trim();
        }
      } else if (trimmed.startsWith('IP:') || trimmed.startsWith('Your IP:')) {
        status.vpn_ip = trimmed.split(':').slice(1).join(':').trim();
      } else if (trimmed.startsWith('Protocol:')) {
        status.protocol = trimmed.split(':')[1]?.trim().toLowerCase();
      } else if (trimmed.toLowerCase().includes('not connected') || trimmed.toLowerCase().includes('disconnected')) {
        status.connected = false;
      }
    }

    if (status.connected) {
      const proto = status.protocol ?? '';
      status.interface = proto.includes('lightway') || proto.includes('wireguard') ? 'lwip0' : 'tun0';
    }

    return status;
  }

  // ============================================================================
  // Kill Switch
  // ============================================================================

  async enableKillSwitch(): Promise<void> {
    logger.info('Enabling ExpressVPN network lock (kill switch)');

    try {
      await this.executeCommand(`${this.cliCommand} preferences set network_lock default`);
      logger.info('Network lock enabled');
    } catch (error) {
      logger.error('Failed to enable network lock', {
        error: error instanceof Error ? error.message : String(error),
      });
      throw error;
    }
  }

  async disableKillSwitch(): Promise<void> {
    logger.info('Disabling ExpressVPN network lock');

    try {
      await this.executeCommand(`${this.cliCommand} preferences set network_lock off`);
      logger.info('Network lock disabled');
    } catch (error) {
      logger.error('Failed to disable network lock', {
        error: error instanceof Error ? error.message : String(error),
      });
      throw error;
    }
  }

  // ============================================================================
  // Additional Methods
  // ============================================================================

  /**
   * Map generic protocol name to ExpressVPN CLI protocol string
   */
  private mapProtocol(protocol: VPNProtocol): string {
    const map: Record<string, string> = {
      lightway: 'lightway_udp',
      wireguard: 'lightway_udp', // ExpressVPN's Lightway is WireGuard-based
      openvpn_udp: 'udp',
      openvpn_tcp: 'tcp',
      ikev2: 'ikev2',
    };
    return map[protocol] ?? 'lightway_udp';
  }

  /**
   * Get recommended (Smart Location) server
   */
  async getRecommendedLocation(): Promise<string> {
    const result = await this.executeCommand(`${this.cliCommand} list recommended`);
    const lines = result.stdout
      .split('\n')
      .map((l) => l.trim())
      .filter((l) => l.length > 0);
    return lines[0] ?? 'smart';
  }

  /**
   * List all available locations
   */
  async listLocations(): Promise<string[]> {
    const result = await this.executeCommand(`${this.cliCommand} list all`);
    return result.stdout
      .split('\n')
      .map((line) => line.trim())
      .filter((line) => line.length > 0);
  }

  /**
   * Set split tunneling app bypass
   */
  async addSplitTunnelApp(appPath: string): Promise<void> {
    await this.executeCommand(`${this.cliCommand} split-tunnel add "${appPath}"`);
  }
}
