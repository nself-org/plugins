/**
 * KeepSolid VPN Unlimited Provider Implementation
 * API-only (no official Linux CLI)
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

const logger = createLogger('vpn:keepsolid');

interface KeepSolidServer {
  id: string;
  name: string;
  country_code: string;
  country_name: string;
  city: string;
  ip: string;
  load: number;
  available: boolean;
  protocols: string[];
  tags: string[];
}

interface KeepSolidServerListResponse {
  servers: KeepSolidServer[];
}

interface KeepSolidAuthResponse {
  access_token: string;
  expires_in: number;
  token_type: string;
}

interface KeepSolidConnectResponse {
  server_ip: string;
  server_hostname: string;
  vpn_ip: string;
  wg_config?: string;
  ovpn_config?: string;
  connection_id: string;
}

export class KeepSolidProvider extends BaseVPNProvider {
  readonly name = 'keepsolid' as const;
  readonly displayName = 'KeepSolid VPN Unlimited';
  readonly cliAvailable = false;
  readonly apiAvailable = true;
  readonly portForwardingSupported = false;
  readonly p2pAllServers = false;

  private readonly apiEndpoint = 'https://api.keepsolid.com/vpnunlimited';
  private accessToken: string | null = null;
  private activeConnectionId: string | null = null;
  private activeVpnIp: string | null = null;
  private wgInterface: string | null = null;

  // ============================================================================
  // Initialization
  // ============================================================================

  protected async checkCLIInstalled(): Promise<void> {
    // KeepSolid VPN Unlimited has no official Linux CLI.
    // All operations are conducted through the REST API.
    logger.info('KeepSolid VPN Unlimited operates API-only — no CLI required');
  }

  protected async performAuthentication(credentials: VPNCredentialRecord): Promise<boolean> {
    try {
      if (!credentials.username || !credentials.password_encrypted) {
        throw new Error('KeepSolid requires username and password');
      }

      const response = await fetch(`${this.apiEndpoint}/auth/login`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          login: credentials.username,
          password: credentials.password_encrypted,
        }),
      });

      if (!response.ok) {
        const err = await response.text();
        throw new Error(`Authentication failed: ${response.status} ${err}`);
      }

      const data: KeepSolidAuthResponse = await response.json();
      this.accessToken = data.access_token;

      logger.info('Successfully authenticated with KeepSolid VPN Unlimited');
      return true;
    } catch (error) {
      logger.error('KeepSolid authentication failed', {
        error: error instanceof Error ? error.message : String(error),
      });
      return false;
    }
  }

  // ============================================================================
  // Server Management
  // ============================================================================

  async fetchServers(): Promise<VPNServerRecord[]> {
    logger.info('Fetching KeepSolid server list from API');

    try {
      const response = await fetch(`${this.apiEndpoint}/servers`, {
        headers: this.authHeaders(),
      });

      if (!response.ok) {
        throw new Error(`API request failed: ${response.statusText}`);
      }

      const data: KeepSolidServerListResponse = await response.json();
      const servers = data.servers ?? [];
      logger.info(`Fetched ${servers.length} servers from KeepSolid`);

      return servers.map((server) => this.mapServer(server));
    } catch (error) {
      logger.error('Failed to fetch KeepSolid servers', {
        error: error instanceof Error ? error.message : String(error),
      });
      throw error;
    }
  }

  private mapServer(server: KeepSolidServer): VPNServerRecord {
    const protocols: VPNProtocol[] = [];
    for (const p of server.protocols ?? []) {
      if (p === 'wireguard') protocols.push('wireguard');
      else if (p === 'openvpn_udp') protocols.push('openvpn_udp');
      else if (p === 'openvpn_tcp') protocols.push('openvpn_tcp');
      else if (p === 'ikev2') protocols.push('ikev2');
    }
    if (protocols.length === 0) {
      protocols.push('wireguard', 'openvpn_udp', 'openvpn_tcp');
    }

    const isP2P = server.tags?.includes('p2p');

    return {
      id: `keepsolid-${server.id}`,
      provider_id: 'keepsolid',
      hostname: server.ip,
      ip_address: server.ip,
      country_code: (server.country_code ?? '').toUpperCase(),
      country_name: server.country_name,
      city: server.city,
      p2p_supported: isP2P ?? false,
      port_forwarding_supported: false,
      protocols,
      load: server.load,
      status: server.available ? 'online' : 'offline',
      features: server.tags ?? [],
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

    logger.info('Connecting to KeepSolid VPN Unlimited via API', {
      region: request.region,
      server: request.server,
      protocol: request.protocol,
    });

    const protocol = request.protocol ?? 'wireguard';

    const payload: Record<string, unknown> = {
      protocol: this.mapProtocol(protocol as VPNProtocol),
    };

    if (request.server) {
      payload['server_id'] = request.server;
    } else if (request.region) {
      payload['country_code'] = request.region.toUpperCase();
    }

    try {
      const response = await fetch(`${this.apiEndpoint}/connect`, {
        method: 'POST',
        headers: { ...this.authHeaders(), 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
      });

      if (!response.ok) {
        const err = await response.text();
        throw new Error(`Connect API failed: ${response.status} ${err}`);
      }

      const data: KeepSolidConnectResponse = await response.json();
      this.activeConnectionId = data.connection_id;
      this.activeVpnIp = data.vpn_ip;

      // If WireGuard config was returned, apply it
      if (data.wg_config) {
        await this.applyWireGuardConfig(data.wg_config);
        this.wgInterface = 'wg-keepsolid';
      } else if (data.ovpn_config) {
        await this.applyOpenVPNConfig(data.ovpn_config);
      }

      logger.info('KeepSolid connection established', {
        connection_id: data.connection_id,
        vpn_ip: data.vpn_ip,
      });

      const connection: VPNConnectionRecord = {
        id: `keepsolid-${Date.now()}`,
        provider_id: 'keepsolid',
        server_id: undefined,
        protocol: protocol as VPNProtocol,
        status: 'connected',
        local_ip: undefined,
        vpn_ip: data.vpn_ip,
        interface_name: this.wgInterface ?? 'tun0',
        dns_servers: [],
        connected_at: new Date(),
        kill_switch_enabled: request.kill_switch !== false,
        requested_by: request.requested_by,
        metadata: { api_connection_id: data.connection_id },
        created_at: new Date(),
      };

      return connection;
    } catch (error) {
      logger.error('KeepSolid connection failed', {
        error: error instanceof Error ? error.message : String(error),
      });
      throw new Error(`Failed to connect to KeepSolid VPN Unlimited: ${error}`);
    }
  }

  async disconnect(connectionId: string): Promise<void> {
    logger.info('Disconnecting from KeepSolid VPN Unlimited', { connectionId });

    try {
      if (this.activeConnectionId) {
        await fetch(`${this.apiEndpoint}/disconnect`, {
          method: 'POST',
          headers: { ...this.authHeaders(), 'Content-Type': 'application/json' },
          body: JSON.stringify({ connection_id: this.activeConnectionId }),
        });
      }

      // Bring down WireGuard interface if we brought it up
      if (this.wgInterface) {
        await this.executeCommand(`wg-quick down ${this.wgInterface}`).catch(() => {});
        this.wgInterface = null;
      } else {
        // Attempt to bring down OpenVPN tunnel
        await this.executeCommand('pkill -f openvpn').catch(() => {});
      }

      this.activeConnectionId = null;
      this.activeVpnIp = null;

      logger.info('Successfully disconnected from KeepSolid VPN Unlimited');
    } catch (error) {
      logger.error('Failed to disconnect from KeepSolid VPN Unlimited', {
        error: error instanceof Error ? error.message : String(error),
      });
      throw error;
    }
  }

  async getStatus(): Promise<VPNStatus> {
    if (!this.activeConnectionId) {
      return { connected: false };
    }

    try {
      const response = await fetch(
        `${this.apiEndpoint}/status/${this.activeConnectionId}`,
        { headers: this.authHeaders() }
      );

      if (!response.ok) {
        return { connected: false };
      }

      const data: { connected: boolean; vpn_ip?: string; server?: string } = await response.json();

      return {
        connected: data.connected,
        vpn_ip: data.vpn_ip ?? this.activeVpnIp ?? undefined,
        server: data.server,
        interface: this.wgInterface ?? 'tun0',
        protocol: 'wireguard',
      };
    } catch (error) {
      logger.error('Failed to get KeepSolid status', {
        error: error instanceof Error ? error.message : String(error),
      });
      return { connected: false };
    }
  }

  // ============================================================================
  // Kill Switch (iptables-based — API-only providers manage this manually)
  // ============================================================================

  async enableKillSwitch(): Promise<void> {
    logger.info('Enabling iptables kill switch for KeepSolid');

    try {
      // Block all traffic except through VPN interface
      const iface = this.wgInterface ?? 'tun0';
      await this.executeCommand(
        `iptables -I OUTPUT ! -o ${iface} -m mark ! --mark $(wg show ${iface} fwmark 2>/dev/null || echo 0) -j DROP`
      );
      logger.info('Kill switch enabled via iptables');
    } catch (error) {
      logger.error('Failed to enable kill switch', {
        error: error instanceof Error ? error.message : String(error),
      });
      throw error;
    }
  }

  async disableKillSwitch(): Promise<void> {
    logger.info('Disabling iptables kill switch for KeepSolid');

    try {
      const iface = this.wgInterface ?? 'tun0';
      await this.executeCommand(
        `iptables -D OUTPUT ! -o ${iface} -m mark ! --mark $(wg show ${iface} fwmark 2>/dev/null || echo 0) -j DROP`
      ).catch(() => {});
      logger.info('Kill switch disabled');
    } catch (error) {
      logger.error('Failed to disable kill switch', {
        error: error instanceof Error ? error.message : String(error),
      });
      throw error;
    }
  }

  // ============================================================================
  // Private Helpers
  // ============================================================================

  private authHeaders(): Record<string, string> {
    const headers: Record<string, string> = {
      'Content-Type': 'application/json',
    };
    if (this.accessToken) {
      headers['Authorization'] = `Bearer ${this.accessToken}`;
    }
    return headers;
  }

  private mapProtocol(protocol: VPNProtocol): string {
    const map: Record<string, string> = {
      wireguard: 'wireguard',
      openvpn_udp: 'openvpn-udp',
      openvpn_tcp: 'openvpn-tcp',
      ikev2: 'ikev2',
    };
    return map[protocol] ?? 'wireguard';
  }

  private async applyWireGuardConfig(config: string): Promise<void> {
    const fs = await import('fs/promises');
    const configPath = '/etc/wireguard/wg-keepsolid.conf';
    await fs.writeFile(configPath, config, { mode: 0o600 });
    await this.executeCommand('wg-quick up wg-keepsolid', 30000);
  }

  private async applyOpenVPNConfig(config: string): Promise<void> {
    const fs = await import('fs/promises');
    const configPath = '/tmp/keepsolid.ovpn';
    await fs.writeFile(configPath, config, { mode: 0o600 });
    // Start OpenVPN in background
    this.executeCommand(`openvpn --config ${configPath} --daemon`, 5000).catch(() => {});
    // Allow time for tunnel to establish
    await new Promise((resolve) => setTimeout(resolve, 5000));
  }

  /**
   * Refresh access token using stored credentials
   */
  async refreshToken(credentials: VPNCredentialRecord): Promise<void> {
    this.authenticated = false;
    await this.authenticate(credentials);
  }

  /**
   * Get account subscription info
   */
  async getAccountInfo(): Promise<Record<string, unknown>> {
    const response = await fetch(`${this.apiEndpoint}/account`, {
      headers: this.authHeaders(),
    });

    if (!response.ok) {
      throw new Error(`Failed to fetch account info: ${response.statusText}`);
    }

    return response.json();
  }
}
