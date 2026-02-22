/**
 * Windscribe VPN Provider Implementation
 * Full support for Windscribe CLI and API
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

const logger = createLogger('vpn:windscribe');

interface WindscribeServer {
  hostname: string;
  name: string;
  ip: string;
  ip2?: string;
  ip3?: string;
  cnt: number; // current connections
  load: number;
  wg_pubkey?: string;
  tags?: string[];
  pro?: number; // 1 = pro only
}

interface WindscribeRegion {
  name: string;
  country_code: string;
  short_name: string;
  nodes: WindscribeServer[];
  p2p?: number; // 1 = P2P allowed
  premium_only?: number; // 1 = premium only
}

interface WindscribeServerListResponse {
  data: {
    regions: WindscribeRegion[];
    timestamp: number;
  };
}

interface WindscribeSessionResponse {
  data: {
    session_auth_hash: string;
    username: string;
    is_premium: boolean;
  };
}

export class WindscribeProvider extends BaseVPNProvider {
  readonly name = 'windscribe' as const;
  readonly displayName = 'Windscribe';
  readonly cliAvailable = true;
  readonly apiAvailable = true;
  readonly portForwardingSupported = true;
  readonly p2pAllServers = false;

  private readonly apiEndpoint = 'https://api.windscribe.com';
  private readonly cliCommand = 'windscribe';
  private sessionAuthHash: string | null = null;
  private forwardedPort: number | null = null;

  // ============================================================================
  // Initialization
  // ============================================================================

  protected async checkCLIInstalled(): Promise<void> {
    try {
      await this.executeCommand(`${this.cliCommand} --version`);
      logger.info('Windscribe CLI is installed');
    } catch (error) {
      throw new Error(
        'Windscribe CLI is not installed. Install from: https://windscribe.com/download'
      );
    }
  }

  protected async performAuthentication(credentials: VPNCredentialRecord): Promise<boolean> {
    try {
      // Check if already logged in
      try {
        const accountResult = await this.executeCommand(`${this.cliCommand} account`);
        if (
          accountResult.stdout.includes('Username:') &&
          !accountResult.stdout.includes('Not logged in')
        ) {
          logger.info('Already logged in to Windscribe');
          return true;
        }
      } catch {
        // Not logged in — proceed
      }

      if (!credentials.username || !credentials.password_encrypted) {
        throw new Error('Windscribe requires username and password');
      }

      // Login using CLI
      const result = await this.executeCommand(
        `${this.cliCommand} login ${credentials.username} ${credentials.password_encrypted}`,
        30000
      );

      if (
        result.stdout.toLowerCase().includes('logged in') ||
        result.stdout.toLowerCase().includes('successfully')
      ) {
        logger.info('Successfully logged in to Windscribe');
        return true;
      }

      // Attempt API login to get session token
      await this.apiLogin(credentials);
      return true;
    } catch (error) {
      logger.error('Windscribe authentication failed', {
        error: error instanceof Error ? error.message : String(error),
      });
      return false;
    }
  }

  private async apiLogin(credentials: VPNCredentialRecord): Promise<void> {
    const response = await fetch(`${this.apiEndpoint}/Session`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        username: credentials.username,
        password: credentials.password_encrypted,
      }),
    });

    if (!response.ok) {
      const errText = await response.text();
      throw new Error(`API login failed: ${response.status} ${errText}`);
    }

    const data: WindscribeSessionResponse = await response.json();
    this.sessionAuthHash = data.data.session_auth_hash;
    logger.info('Windscribe API session established');
  }

  // ============================================================================
  // Server Management
  // ============================================================================

  async fetchServers(): Promise<VPNServerRecord[]> {
    logger.info('Fetching Windscribe server list from API');

    try {
      const url = this.sessionAuthHash
        ? `${this.apiEndpoint}/ServerList?session_auth_hash=${this.sessionAuthHash}`
        : `${this.apiEndpoint}/ServerList`;

      const response = await fetch(url);

      if (!response.ok) {
        throw new Error(`API request failed: ${response.statusText}`);
      }

      const data: WindscribeServerListResponse = await response.json();
      const regions = data.data?.regions ?? [];
      logger.info(`Fetched ${regions.length} regions from Windscribe`);

      const servers: VPNServerRecord[] = [];

      for (const region of regions) {
        for (const node of region.nodes ?? []) {
          servers.push(this.mapServer(node, region));
        }
      }

      return servers;
    } catch (error) {
      logger.error('Failed to fetch Windscribe servers', {
        error: error instanceof Error ? error.message : String(error),
      });
      throw error;
    }
  }

  private mapServer(node: WindscribeServer, region: WindscribeRegion): VPNServerRecord {
    const protocols: VPNProtocol[] = ['wireguard', 'openvpn_udp', 'openvpn_tcp', 'ikev2'];
    const isP2P = (region.p2p ?? 0) === 1;
    const isPremium = (region.premium_only ?? 0) === 1 || (node.pro ?? 0) === 1;

    const features: string[] = [];
    if (isP2P) features.push('p2p');
    if (isPremium) features.push('premium_only');

    return {
      id: `windscribe-${node.hostname}`,
      provider_id: 'windscribe',
      hostname: node.hostname,
      ip_address: node.ip,
      country_code: (region.country_code ?? '').toUpperCase(),
      country_name: region.name,
      city: region.short_name,
      p2p_supported: isP2P,
      port_forwarding_supported: isP2P, // Port forwarding only on P2P servers
      protocols,
      load: node.load,
      status: 'online',
      features,
      public_key: node.wg_pubkey,
      owned: true,
      metadata: {
        server_name: node.name,
        active_connections: node.cnt,
        premium_only: isPremium,
        additional_ips: [node.ip2, node.ip3].filter(Boolean),
      },
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

    logger.info('Connecting to Windscribe', {
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
      // Specific server hostname
      connectCmd += ` ${request.server}`;
    } else if (request.region) {
      // Country code or short name (e.g., "US", "US-East")
      connectCmd += ` ${request.region}`;
    }
    // No args = connect to best available

    // Request port forwarding if needed
    if (request.port_forwarding) {
      await this.enablePortForwarding();
    }

    try {
      const result = await this.executeCommand(connectCmd, 60000);
      logger.info('Windscribe connection established', { output: result.stdout });

      // Wait for tunnel to stabilize
      await new Promise((resolve) => setTimeout(resolve, 2000));

      const status = await this.getStatus();

      if (request.port_forwarding) {
        this.forwardedPort = await this.getForwardedPort();
      }

      const connection: VPNConnectionRecord = {
        id: `windscribe-${Date.now()}`,
        provider_id: 'windscribe',
        server_id: undefined,
        protocol: (request.protocol || 'wireguard') as VPNProtocol,
        status: 'connected',
        local_ip: undefined,
        vpn_ip: status.vpn_ip,
        interface_name: status.interface,
        dns_servers: [],
        connected_at: new Date(),
        kill_switch_enabled: request.kill_switch !== false,
        port_forwarded: this.forwardedPort ?? undefined,
        requested_by: request.requested_by,
        metadata: {},
        created_at: new Date(),
      };

      return connection;
    } catch (error) {
      logger.error('Windscribe connection failed', {
        error: error instanceof Error ? error.message : String(error),
      });
      throw new Error(`Failed to connect to Windscribe: ${error}`);
    }
  }

  async disconnect(connectionId: string): Promise<void> {
    logger.info('Disconnecting from Windscribe', { connectionId });

    try {
      await this.executeCommand(`${this.cliCommand} disconnect`);
      this.forwardedPort = null;
      logger.info('Successfully disconnected from Windscribe');
    } catch (error) {
      logger.error('Failed to disconnect from Windscribe', {
        error: error instanceof Error ? error.message : String(error),
      });
      throw error;
    }
  }

  async getStatus(): Promise<VPNStatus> {
    try {
      const result = await this.executeCommand(`${this.cliCommand} status`);
      return this.parseWindscribeStatus(result.stdout);
    } catch (error) {
      logger.error('Failed to get Windscribe status', {
        error: error instanceof Error ? error.message : String(error),
      });
      return { connected: false };
    }
  }

  private parseWindscribeStatus(output: string): VPNStatus {
    const lines = output.split('\n');
    const status: VPNStatus = { connected: false };

    for (const line of lines) {
      const trimmed = line.trim();

      if (trimmed.toLowerCase().includes('connected')) {
        status.connected = true;
      } else if (trimmed.toLowerCase().includes('disconnected') || trimmed.toLowerCase().includes('not connected')) {
        status.connected = false;
      } else if (trimmed.startsWith('Server:') || trimmed.startsWith('Location:')) {
        status.server = trimmed.split(':').slice(1).join(':').trim();
      } else if (trimmed.startsWith('IP:') || trimmed.startsWith('Your IP:')) {
        status.vpn_ip = trimmed.split(':').slice(1).join(':').trim();
      } else if (trimmed.startsWith('Protocol:')) {
        status.protocol = trimmed.split(':')[1]?.trim().toLowerCase();
      }
    }

    if (status.connected) {
      const proto = status.protocol ?? '';
      status.interface = proto.includes('wireguard') ? 'wg0' : 'tun0';
      status.port_forwarded = this.forwardedPort ?? undefined;
    }

    return status;
  }

  // ============================================================================
  // Port Forwarding
  // ============================================================================

  async getForwardedPort(): Promise<number | null> {
    try {
      const result = await this.executeCommand(`${this.cliCommand} port-forwarding`);
      const output = result.stdout.trim();

      // Output format: "Port forwarding is active on port: 12345"
      const match = output.match(/port:\s*(\d+)/i);
      if (match) {
        const port = parseInt(match[1], 10);
        logger.info(`Windscribe forwarded port: ${port}`);
        return port;
      }

      return null;
    } catch (error) {
      logger.warn('Failed to get Windscribe forwarded port', {
        error: error instanceof Error ? error.message : String(error),
      });
      return null;
    }
  }

  // ============================================================================
  // Kill Switch
  // ============================================================================

  async enableKillSwitch(): Promise<void> {
    logger.info('Enabling Windscribe firewall (kill switch)');

    try {
      await this.executeCommand(`${this.cliCommand} firewall on`);
      logger.info('Windscribe firewall enabled');
    } catch (error) {
      logger.error('Failed to enable Windscribe firewall', {
        error: error instanceof Error ? error.message : String(error),
      });
      throw error;
    }
  }

  async disableKillSwitch(): Promise<void> {
    logger.info('Disabling Windscribe firewall');

    try {
      await this.executeCommand(`${this.cliCommand} firewall off`);
      logger.info('Windscribe firewall disabled');
    } catch (error) {
      logger.error('Failed to disable Windscribe firewall', {
        error: error instanceof Error ? error.message : String(error),
      });
      throw error;
    }
  }

  // ============================================================================
  // Additional Methods
  // ============================================================================

  /**
   * Map generic protocol to Windscribe CLI protocol string
   */
  private mapProtocol(protocol: VPNProtocol): string {
    const map: Record<string, string> = {
      wireguard: 'wireguard',
      openvpn_udp: 'openvpn-udp',
      openvpn_tcp: 'openvpn-tcp',
      ikev2: 'ikev2',
    };
    return map[protocol] ?? 'wireguard';
  }

  /**
   * Enable port forwarding (Windscribe Ephemeral Port)
   */
  async enablePortForwarding(): Promise<void> {
    await this.executeCommand(`${this.cliCommand} port-forwarding on`);
  }

  /**
   * Disable port forwarding
   */
  async disablePortForwarding(): Promise<void> {
    await this.executeCommand(`${this.cliCommand} port-forwarding off`);
    this.forwardedPort = null;
  }

  /**
   * Get list of available locations
   */
  async getLocations(): Promise<string[]> {
    const result = await this.executeCommand(`${this.cliCommand} locations`);
    return result.stdout
      .split('\n')
      .map((line) => line.trim())
      .filter((line) => line.length > 0);
  }

  /**
   * View account details (data remaining, plan, etc.)
   */
  async getAccountDetails(): Promise<string> {
    const result = await this.executeCommand(`${this.cliCommand} account`);
    return result.stdout;
  }

  /**
   * Enable split tunneling for a specific app
   */
  async addSplitTunnelApp(appName: string): Promise<void> {
    await this.executeCommand(`${this.cliCommand} split-tunnel add "${appName}"`);
  }
}
