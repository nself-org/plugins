/**
 * NordVPN Provider Implementation
 * Full support for NordVPN CLI and API
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

const logger = createLogger('vpn:nordvpn');

interface NordVPNServer {
  id: number;
  name: string;
  hostname: string;
  station: string; // IP address
  load: number;
  status: string;
  locations: Array<{
    country: {
      id: number;
      name: string;
      code: string;
    };
    city: {
      id: number;
      name: string;
    };
  }>;
  technologies: Array<{
    id: number;
    name: string;
    identifier: string;
  }>;
  groups: Array<{
    id: number;
    title: string;
    identifier: string;
  }>;
}

export class NordVPNProvider extends BaseVPNProvider {
  readonly name = 'nordvpn' as const;
  readonly displayName = 'NordVPN';
  readonly cliAvailable = true;
  readonly apiAvailable = true;
  readonly portForwardingSupported = false;
  readonly p2pAllServers = true;

  private readonly apiEndpoint = 'https://api.nordvpn.com/v1';
  private readonly cliCommand = 'nordvpn';

  // ============================================================================
  // Initialization
  // ============================================================================

  protected async checkCLIInstalled(): Promise<void> {
    try {
      await this.executeCommand(`${this.cliCommand} --version`);
      logger.info('NordVPN CLI is installed');
    } catch (error) {
      throw new Error(
        'NordVPN CLI is not installed. Install from: https://nordvpn.com/download/linux/'
      );
    }
  }

  protected async performAuthentication(credentials: VPNCredentialRecord): Promise<boolean> {
    try {
      // Check if already logged in
      const accountResult = await this.executeCommand(`${this.cliCommand} account`);
      if (accountResult.stdout.includes('You are logged in')) {
        logger.info('Already logged in to NordVPN');
        return true;
      }

      // Login with token
      if (credentials.api_token_encrypted) {
        const token = credentials.api_token_encrypted; // Already decrypted by database
        await this.executeCommand(`${this.cliCommand} login --token ${token}`);
        logger.info('Successfully logged in with token');
        return true;
      }

      throw new Error('NordVPN requires access token for authentication');
    } catch (error) {
      logger.error('NordVPN authentication failed', { error: error instanceof Error ? error.message : String(error) });
      return false;
    }
  }

  // ============================================================================
  // Server Management
  // ============================================================================

  async fetchServers(): Promise<VPNServerRecord[]> {
    logger.info('Fetching NordVPN server list from API');

    try {
      // Fetch P2P servers via API
      const url = `${this.apiEndpoint}/servers/recommendations?filters[servers_groups][identifier]=legacy_p2p&limit=1000`;
      const response = await fetch(url);

      if (!response.ok) {
        throw new Error(`API request failed: ${response.statusText}`);
      }

      const servers: NordVPNServer[] = await response.json();
      logger.info(`Fetched ${servers.length} P2P servers from NordVPN`);

      return servers.map((server) => this.mapServer(server));
    } catch (error) {
      logger.error('Failed to fetch NordVPN servers', { error: error instanceof Error ? error.message : String(error) });
      throw error;
    }
  }

  private mapServer(server: NordVPNServer): VPNServerRecord {
    const location = server.locations[0];
    const protocols: VPNProtocol[] = [];

    server.technologies.forEach((tech) => {
      if (tech.identifier === 'wireguard_udp' || tech.identifier === 'nordlynx') {
        protocols.push('nordlynx');
      } else if (tech.identifier === 'openvpn_udp') {
        protocols.push('openvpn_udp');
      } else if (tech.identifier === 'openvpn_tcp') {
        protocols.push('openvpn_tcp');
      }
    });

    const isP2P = server.groups.some((g) => g.identifier === 'legacy_p2p');
    const features = server.groups.map((g) => g.identifier);

    return {
      id: `nordvpn-${server.id}`,
      provider_id: 'nordvpn',
      hostname: server.hostname,
      ip_address: server.station,
      country_code: location.country.code,
      country_name: location.country.name,
      city: location.city.name,
      p2p_supported: isP2P,
      port_forwarding_supported: false,
      protocols,
      load: server.load,
      status: server.status === 'online' ? 'online' : 'offline',
      features,
      owned: true,
      metadata: {},
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

    logger.info('Connecting to NordVPN', {
      region: request.region,
      server: request.server,
      protocol: request.protocol,
    });

    // Set protocol
    if (request.protocol) {
      const tech = request.protocol === 'wireguard' || request.protocol === 'nordlynx' ? 'nordlynx' : 'openvpn';
      await this.executeCommand(`${this.cliCommand} set technology ${tech}`);
    }

    // Enable kill switch if requested
    if (request.kill_switch !== false) {
      await this.enableKillSwitch();
    }

    // Build connect command
    let connectCmd = `${this.cliCommand} connect`;

    if (request.server) {
      // Specific server
      connectCmd += ` ${request.server}`;
    } else if (request.region) {
      // Specific region/country
      connectCmd += ` ${request.region}`;
    } else {
      // Connect to best P2P server
      connectCmd += ' --group p2p';
    }

    // Execute connection
    try {
      const result = await this.executeCommand(connectCmd, 60000); // 60 second timeout
      logger.info('NordVPN connection established', { output: result.stdout });

      // Get connection details
      const status = await this.getStatus();

      // Create connection record
      const connection: VPNConnectionRecord = {
        id: `nordvpn-${Date.now()}`,
        provider_id: 'nordvpn',
        server_id: undefined, // Would need to look up from status
        protocol: (request.protocol || 'nordlynx') as VPNProtocol,
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
      logger.error('NordVPN connection failed', { error: error instanceof Error ? error.message : String(error) });
      throw new Error(`Failed to connect to NordVPN: ${error}`);
    }
  }

  async disconnect(connectionId: string): Promise<void> {
    logger.info('Disconnecting from NordVPN', { connectionId });

    try {
      await this.executeCommand(`${this.cliCommand} disconnect`);
      logger.info('Successfully disconnected from NordVPN');
    } catch (error) {
      logger.error('Failed to disconnect from NordVPN', { error: error instanceof Error ? error.message : String(error) });
      throw error;
    }
  }

  async getStatus(): Promise<VPNStatus> {
    try {
      const result = await this.executeCommand(`${this.cliCommand} status`);
      return this.parseNordVPNStatus(result.stdout);
    } catch (error) {
      logger.error('Failed to get NordVPN status', { error: error instanceof Error ? error.message : String(error) });
      return { connected: false };
    }
  }

  private parseNordVPNStatus(output: string): VPNStatus {
    const lines = output.split('\n');
    const status: VPNStatus = {
      connected: false,
    };

    for (const line of lines) {
      const trimmed = line.trim();

      if (trimmed.startsWith('Status:')) {
        status.connected = trimmed.includes('Connected');
      } else if (trimmed.startsWith('Server IP:')) {
        status.vpn_ip = trimmed.split(':')[1]?.trim();
      } else if (trimmed.startsWith('Hostname:')) {
        status.server = trimmed.split(':')[1]?.trim();
      } else if (trimmed.startsWith('Technology:')) {
        status.protocol = trimmed.split(':')[1]?.trim().toLowerCase();
      } else if (trimmed.startsWith('Protocol:')) {
        const proto = trimmed.split(':')[1]?.trim().toLowerCase();
        if (proto) {
          status.protocol = status.protocol ? `${status.protocol}_${proto}` : proto;
        }
      } else if (trimmed.startsWith('Transfer:')) {
        // Parse transfer data (e.g., "Transfer: 10.5 MiB received, 2.3 MiB sent")
        const match = trimmed.match(/(\d+\.?\d*)\s*(\w+)\s*received.*?(\d+\.?\d*)\s*(\w+)\s*sent/i);
        if (match) {
          status.bytes_received = this.convertToBytes(parseFloat(match[1]), match[2]);
          status.bytes_sent = this.convertToBytes(parseFloat(match[3]), match[4]);
        }
      } else if (trimmed.startsWith('Uptime:')) {
        // Parse uptime (e.g., "Uptime: 1 hour 23 minutes")
        const uptime = trimmed.split(':')[1]?.trim();
        if (uptime) {
          status.uptime_seconds = this.parseUptime(uptime);
        }
      }
    }

    // Detect interface (NordVPN uses nordlynx or tun0)
    if (status.connected && status.protocol?.includes('nordlynx')) {
      status.interface = 'nordlynx';
    } else if (status.connected) {
      status.interface = 'tun0';
    }

    return status;
  }

  private convertToBytes(value: number, unit: string): string {
    const units: { [key: string]: number } = {
      b: 1,
      kb: 1024,
      mb: 1024 * 1024,
      mib: 1024 * 1024,
      gb: 1024 * 1024 * 1024,
      gib: 1024 * 1024 * 1024,
    };

    const multiplier = units[unit.toLowerCase()] || 1;
    return String(Math.floor(value * multiplier));
  }

  private parseUptime(uptime: string): number {
    let seconds = 0;

    const hourMatch = uptime.match(/(\d+)\s*hour/);
    if (hourMatch) {
      seconds += parseInt(hourMatch[1]) * 3600;
    }

    const minuteMatch = uptime.match(/(\d+)\s*minute/);
    if (minuteMatch) {
      seconds += parseInt(minuteMatch[1]) * 60;
    }

    const secondMatch = uptime.match(/(\d+)\s*second/);
    if (secondMatch) {
      seconds += parseInt(secondMatch[1]);
    }

    return seconds;
  }

  // ============================================================================
  // Kill Switch
  // ============================================================================

  async enableKillSwitch(): Promise<void> {
    logger.info('Enabling NordVPN kill switch');

    try {
      await this.executeCommand(`${this.cliCommand} set killswitch on`);
      logger.info('Kill switch enabled');
    } catch (error) {
      logger.error('Failed to enable kill switch', { error: error instanceof Error ? error.message : String(error) });
      throw error;
    }
  }

  async disableKillSwitch(): Promise<void> {
    logger.info('Disabling NordVPN kill switch');

    try {
      await this.executeCommand(`${this.cliCommand} set killswitch off`);
      logger.info('Kill switch disabled');
    } catch (error) {
      logger.error('Failed to disable kill switch', { error: error instanceof Error ? error.message : String(error) });
      throw error;
    }
  }

  // ============================================================================
  // Additional Commands
  // ============================================================================

  /**
   * Refresh server list cache
   */
  async refreshServers(): Promise<void> {
    logger.info('Refreshing NordVPN server cache');
    await this.executeCommand(`${this.cliCommand} refresh`);
  }

  /**
   * Get list of countries
   */
  async getCountries(): Promise<string[]> {
    const result = await this.executeCommand(`${this.cliCommand} countries`);
    return result.stdout
      .split('\n')
      .map((line) => line.trim())
      .filter((line) => line.length > 0);
  }

  /**
   * Get cities for a country
   */
  async getCities(countryCode: string): Promise<string[]> {
    const result = await this.executeCommand(`${this.cliCommand} cities ${countryCode}`);
    return result.stdout
      .split('\n')
      .map((line) => line.trim())
      .filter((line) => line.length > 0);
  }

  /**
   * Whitelist subnet (allow local network)
   */
  async whitelistSubnet(subnet: string): Promise<void> {
    await this.executeCommand(`${this.cliCommand} whitelist add subnet ${subnet}`);
  }

  /**
   * Set custom DNS
   */
  async setDNS(servers: string[]): Promise<void> {
    const dnsString = servers.join(' ');
    await this.executeCommand(`${this.cliCommand} set dns ${dnsString}`);
  }

  /**
   * Enable threat protection lite (ad blocking)
   */
  async enableThreatProtection(): Promise<void> {
    await this.executeCommand(`${this.cliCommand} set threatprotectionlite on`);
  }
}
