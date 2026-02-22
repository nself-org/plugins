/**
 * AirVPN Provider Implementation
 * Config-file-based (OpenVPN/WireGuard) with API-assisted server list
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

const logger = createLogger('vpn:airvpn');

interface AirVPNServer {
  public_name: string;
  hostname: string;
  ip: string;
  country_code: string;
  country_name: string;
  city_name?: string;
  continent_name?: string;
  load: number;
  bw: number; // bandwidth in MB/s
  users: number;
  bw_net: number;
  wireguard_port?: number;
  wireguard_key?: string;
  openvpn_port_udp?: number;
  openvpn_port_tcp?: number;
}

interface AirVPNStatusResponse {
  servers: AirVPNServer[];
  code: number;
  message?: string;
}

interface AirVPNConfigResponse {
  config: string;
  format: 'openvpn' | 'wireguard';
}

export class AirVPNProvider extends BaseVPNProvider {
  readonly name = 'airvpn' as const;
  readonly displayName = 'AirVPN';
  readonly cliAvailable = false;
  readonly apiAvailable = true;
  readonly portForwardingSupported = true;
  readonly p2pAllServers = true;

  private readonly apiEndpoint = 'https://airvpn.org/api';
  private apiKey: string | null = null;
  private activeInterface: string | null = null;
  private activeVpnIp: string | null = null;
  private forwardedPort: number | null = null;

  // ============================================================================
  // Initialization
  // ============================================================================

  protected async checkCLIInstalled(): Promise<void> {
    // AirVPN has no official CLI for Linux. Connections use OpenVPN or WireGuard
    // config files. Check that at least one VPN daemon tool is available.
    let hasOpenVPN = false;
    let hasWireGuard = false;

    try {
      await this.executeCommand('which openvpn');
      hasOpenVPN = true;
      logger.info('AirVPN: OpenVPN is available');
    } catch {
      logger.warn('AirVPN: openvpn not found');
    }

    try {
      await this.executeCommand('which wg-quick');
      hasWireGuard = true;
      logger.info('AirVPN: WireGuard (wg-quick) is available');
    } catch {
      logger.warn('AirVPN: wg-quick not found');
    }

    if (!hasOpenVPN && !hasWireGuard) {
      throw new Error(
        'AirVPN requires openvpn or wg-quick to be installed. ' +
          'Install via: apt install openvpn  or  apt install wireguard'
      );
    }
  }

  protected async performAuthentication(credentials: VPNCredentialRecord): Promise<boolean> {
    try {
      const key = credentials.api_key_encrypted ?? credentials.api_token_encrypted;
      if (!key) {
        throw new Error(
          'AirVPN requires an API key (api_key_encrypted or api_token_encrypted). ' +
            'Generate one at: https://airvpn.org/api'
        );
      }

      // Verify the API key by requesting the server status endpoint
      const response = await fetch(`${this.apiEndpoint}/status`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ key }),
      });

      if (!response.ok) {
        const errText = await response.text();
        throw new Error(`API key verification failed: ${response.status} ${errText}`);
      }

      const data: { code: number } = await response.json();
      if (data.code !== 1) {
        throw new Error(`AirVPN API key is invalid or insufficient permissions`);
      }

      this.apiKey = key;
      logger.info('Successfully authenticated with AirVPN');
      return true;
    } catch (error) {
      logger.error('AirVPN authentication failed', {
        error: error instanceof Error ? error.message : String(error),
      });
      return false;
    }
  }

  // ============================================================================
  // Server Management
  // ============================================================================

  async fetchServers(): Promise<VPNServerRecord[]> {
    logger.info('Fetching AirVPN server list from API');

    try {
      const response = await fetch(`${this.apiEndpoint}/status`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ key: this.apiKey, format: 'json' }),
      });

      if (!response.ok) {
        throw new Error(`API request failed: ${response.statusText}`);
      }

      const data: AirVPNStatusResponse = await response.json();

      if (data.code !== 1) {
        throw new Error(`AirVPN API error: ${data.message ?? 'Unknown error'}`);
      }

      const servers = data.servers ?? [];
      logger.info(`Fetched ${servers.length} servers from AirVPN`);

      return servers.map((server) => this.mapServer(server));
    } catch (error) {
      logger.error('Failed to fetch AirVPN servers', {
        error: error instanceof Error ? error.message : String(error),
      });
      throw error;
    }
  }

  private mapServer(server: AirVPNServer): VPNServerRecord {
    const protocols: VPNProtocol[] = [];
    if (server.wireguard_port) protocols.push('wireguard');
    if (server.openvpn_port_udp) protocols.push('openvpn_udp');
    if (server.openvpn_port_tcp) protocols.push('openvpn_tcp');
    if (protocols.length === 0) protocols.push('openvpn_udp', 'openvpn_tcp');

    // Calculate approximate load percentage (users / capacity)
    const loadPercent = server.load ?? Math.min(Math.floor((server.users / 10) * 100), 100);

    return {
      id: `airvpn-${server.public_name}`,
      provider_id: 'airvpn',
      hostname: server.hostname,
      ip_address: server.ip,
      country_code: (server.country_code ?? '').toUpperCase(),
      country_name: server.country_name,
      city: server.city_name,
      region: server.continent_name,
      p2p_supported: true, // AirVPN allows P2P on all servers
      port_forwarding_supported: true,
      protocols,
      load: loadPercent,
      status: 'online',
      features: ['p2p', 'port_forwarding'],
      public_key: server.wireguard_key,
      endpoint_port: server.wireguard_port ?? server.openvpn_port_udp,
      owned: true,
      metadata: {
        bandwidth_mbps: server.bw,
        active_users: server.users,
        network_bandwidth: server.bw_net,
        openvpn_port_udp: server.openvpn_port_udp,
        openvpn_port_tcp: server.openvpn_port_tcp,
        wireguard_port: server.wireguard_port,
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

    logger.info('Connecting to AirVPN via config file', {
      region: request.region,
      server: request.server,
      protocol: request.protocol,
    });

    const protocol = (request.protocol ?? 'openvpn_udp') as VPNProtocol;

    // Request a generated config from the AirVPN API
    const configPayload: Record<string, unknown> = {
      key: this.apiKey,
      format: 'json',
      protocol: this.mapProtocol(protocol),
    };

    if (request.server) {
      configPayload['server'] = request.server;
    } else if (request.region) {
      configPayload['country'] = request.region.toUpperCase();
    }

    if (request.port_forwarding) {
      configPayload['port_forwarding'] = true;
    }

    const configResponse = await fetch(`${this.apiEndpoint}/generator`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(configPayload),
    });

    if (!configResponse.ok) {
      const errText = await configResponse.text();
      throw new Error(`AirVPN config generation failed: ${configResponse.status} ${errText}`);
    }

    const configData: AirVPNConfigResponse = await configResponse.json();

    try {
      if (configData.format === 'wireguard') {
        await this.applyWireGuardConfig(configData.config);
        this.activeInterface = 'wg-airvpn';
      } else {
        await this.applyOpenVPNConfig(configData.config);
        this.activeInterface = 'tun0';
      }

      // Wait and fetch the assigned VPN IP
      const vpnIp = await this.getInterfaceIP(this.activeInterface);
      this.activeVpnIp = vpnIp;

      // Get forwarded port if port forwarding was requested
      if (request.port_forwarding) {
        this.forwardedPort = await this.getForwardedPort();
      }

      logger.info('AirVPN connection established', {
        interface: this.activeInterface,
        vpn_ip: vpnIp,
        port_forwarded: this.forwardedPort,
      });

      const connection: VPNConnectionRecord = {
        id: `airvpn-${Date.now()}`,
        provider_id: 'airvpn',
        server_id: undefined,
        protocol,
        status: 'connected',
        local_ip: undefined,
        vpn_ip: vpnIp ?? undefined,
        interface_name: this.activeInterface,
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
      logger.error('AirVPN connection failed', {
        error: error instanceof Error ? error.message : String(error),
      });
      throw new Error(`Failed to connect to AirVPN: ${error}`);
    }
  }

  async disconnect(connectionId: string): Promise<void> {
    logger.info('Disconnecting from AirVPN', { connectionId });

    try {
      if (this.activeInterface?.startsWith('wg')) {
        await this.executeCommand(`wg-quick down ${this.activeInterface}`);
      } else {
        await this.executeCommand('pkill -f openvpn').catch(() => {});
      }

      this.activeInterface = null;
      this.activeVpnIp = null;
      this.forwardedPort = null;

      logger.info('Successfully disconnected from AirVPN');
    } catch (error) {
      logger.error('Failed to disconnect from AirVPN', {
        error: error instanceof Error ? error.message : String(error),
      });
      throw error;
    }
  }

  async getStatus(): Promise<VPNStatus> {
    if (!this.activeInterface) {
      return { connected: false };
    }

    try {
      const ip = await this.getInterfaceIP(this.activeInterface);
      const connected = ip !== null;

      return {
        connected,
        vpn_ip: ip ?? this.activeVpnIp ?? undefined,
        interface: this.activeInterface,
        protocol: this.activeInterface.startsWith('wg') ? 'wireguard' : 'openvpn',
        port_forwarded: this.forwardedPort ?? undefined,
      };
    } catch (error) {
      logger.error('Failed to get AirVPN status', {
        error: error instanceof Error ? error.message : String(error),
      });
      return { connected: false };
    }
  }

  // ============================================================================
  // Port Forwarding
  // ============================================================================

  async getForwardedPort(): Promise<number | null> {
    if (!this.apiKey || !this.activeVpnIp) {
      return null;
    }

    try {
      const response = await fetch(`${this.apiEndpoint}/portforwarding`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ key: this.apiKey, ip: this.activeVpnIp }),
      });

      if (!response.ok) {
        logger.warn('AirVPN port forwarding request failed', { status: response.status });
        return null;
      }

      const data: { code: number; ports?: number[] } = await response.json();
      if (data.code === 1 && data.ports && data.ports.length > 0) {
        const port = data.ports[0];
        logger.info(`AirVPN forwarded port: ${port}`);
        return port ?? null;
      }

      return null;
    } catch (error) {
      logger.error('Failed to get AirVPN forwarded port', {
        error: error instanceof Error ? error.message : String(error),
      });
      return null;
    }
  }

  // ============================================================================
  // Kill Switch
  // ============================================================================

  async enableKillSwitch(): Promise<void> {
    logger.info('Enabling iptables kill switch for AirVPN');

    try {
      const iface = this.activeInterface ?? 'tun0';
      // Allow established/related traffic, then block everything not on VPN interface
      await this.executeCommand(
        `iptables -I OUTPUT ! -o ${iface} -m state --state NEW -j DROP`
      );
      logger.info('Kill switch enabled');
    } catch (error) {
      logger.error('Failed to enable kill switch', {
        error: error instanceof Error ? error.message : String(error),
      });
      throw error;
    }
  }

  async disableKillSwitch(): Promise<void> {
    logger.info('Disabling iptables kill switch for AirVPN');

    try {
      const iface = this.activeInterface ?? 'tun0';
      await this.executeCommand(
        `iptables -D OUTPUT ! -o ${iface} -m state --state NEW -j DROP`
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

  private mapProtocol(protocol: VPNProtocol): string {
    const map: Record<string, string> = {
      wireguard: 'wireguard',
      openvpn_udp: 'udp',
      openvpn_tcp: 'tcp',
    };
    return map[protocol] ?? 'udp';
  }

  private async applyWireGuardConfig(config: string): Promise<void> {
    const fs = await import('fs/promises');
    await fs.writeFile('/etc/wireguard/wg-airvpn.conf', config, { mode: 0o600 });
    await this.executeCommand('wg-quick up wg-airvpn', 30000);
  }

  private async applyOpenVPNConfig(config: string): Promise<void> {
    const fs = await import('fs/promises');
    const configPath = '/tmp/airvpn.ovpn';
    await fs.writeFile(configPath, config, { mode: 0o600 });
    this.executeCommand(`openvpn --config ${configPath} --daemon`, 5000).catch(() => {});
    await new Promise((resolve) => setTimeout(resolve, 5000));
  }

  /**
   * Get user session status and account info from the API
   */
  async getAccountInfo(): Promise<Record<string, unknown>> {
    const response = await fetch(`${this.apiEndpoint}/userinfo`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ key: this.apiKey }),
    });

    if (!response.ok) {
      throw new Error(`Failed to fetch account info: ${response.statusText}`);
    }

    return response.json();
  }

  /**
   * Request an additional forwarded port (AirVPN allows multiple per session)
   */
  async requestAdditionalPort(): Promise<number | null> {
    return this.getForwardedPort();
  }
}
