/**
 * VPN Checker
 * Verifies VPN connection status before allowing torrent downloads
 */

import axios from 'axios';
import { createLogger } from '@nself/plugin-utils';
import type { VPNStatus } from './types.js';

const logger = createLogger('torrent-manager:vpn-checker');

export class VPNChecker {
  private vpnManagerUrl: string;
  private checkIntervalMs: number = 30000; // 30 seconds
  private isChecking: boolean = false;

  constructor(vpnManagerUrl: string) {
    this.vpnManagerUrl = vpnManagerUrl;
  }

  /**
   * Check if VPN is currently active
   */
  async isVPNActive(): Promise<boolean> {
    try {
      const status = await this.getVPNStatus();
      return status.connected;
    } catch (error) {
      logger.error('Failed to check VPN status', { error: error instanceof Error ? error.message : String(error) });
      return false;
    }
  }

  /**
   * Get detailed VPN status from VPN Manager
   */
  async getVPNStatus(): Promise<VPNStatus> {
    try {
      const response = await axios.get(`${this.vpnManagerUrl}/api/status`, {
        timeout: 5000,
      });

      if (response.data && response.data.connected !== undefined) {
        return {
          connected: response.data.connected,
          provider: response.data.provider,
          server: response.data.server,
          ip: response.data.vpn_ip,
          interface: response.data.interface,
        };
      }

      throw new Error('Invalid VPN status response');
    } catch (error) {
      logger.error('Failed to get VPN status from VPN Manager', { error: error instanceof Error ? error.message : String(error) });
      throw error;
    }
  }

  /**
   * Wait for VPN to become active
   * @param timeoutSeconds Maximum time to wait
   * @returns true if VPN becomes active, false if timeout
   */
  async waitForVPN(timeoutSeconds: number): Promise<boolean> {
    const startTime = Date.now();
    const timeoutMs = timeoutSeconds * 1000;

    logger.info('Waiting for VPN connection', { timeout: timeoutSeconds });

    while (Date.now() - startTime < timeoutMs) {
      const isActive = await this.isVPNActive();
      if (isActive) {
        logger.info('VPN is active');
        return true;
      }

      // Wait 5 seconds before next check
      await new Promise((resolve) => setTimeout(resolve, 5000));
    }

    logger.warn('Timeout waiting for VPN');
    return false;
  }

  /**
   * Subscribe to VPN events (webhook-based)
   * Would integrate with VPN Manager's webhook system
   */
  async subscribeToVPNEvents(): Promise<void> {
    // NOTE: VPN webhook integration requires bidirectional plugin communication
    // Integration requirements:
    // 1. VPN Manager must expose POST /api/webhooks/subscribe endpoint
    // 2. This service must expose POST /webhooks/vpn-events endpoint to receive callbacks
    // 3. Webhook payload should include: event_type (connected|disconnected), provider, server, timestamp
    // 4. On disconnect event: pause all active torrents, wait for reconnection
    // 5. On reconnect event: verify IP changed, resume paused torrents
    //
    // Example implementation:
    // await axios.post(`${this.vpnManagerUrl}/api/webhooks/subscribe`, {
    //   url: `${this.torrentManagerUrl}/webhooks/vpn-events`,
    //   events: ['vpn.disconnected', 'vpn.connected']
    // });
    logger.info('VPN event subscription would be configured here');
  }

  /**
   * Start monitoring VPN connection
   * Continuously checks VPN status and can trigger actions on disconnect
   */
  startMonitoring(onDisconnect?: () => void): void {
    if (this.isChecking) {
      logger.warn('VPN monitoring already active');
      return;
    }

    this.isChecking = true;
    logger.info('Starting VPN monitoring');

    const checkLoop = async () => {
      while (this.isChecking) {
        const isActive = await this.isVPNActive();

        if (!isActive && onDisconnect) {
          logger.warn('VPN disconnected! Triggering callback');
          onDisconnect();
        }

        await new Promise((resolve) => setTimeout(resolve, this.checkIntervalMs));
      }
    };

    checkLoop().catch((error: unknown) => {
      logger.error('VPN monitoring error', { error: error instanceof Error ? error.message : String(error) });
      this.isChecking = false;
    });
  }

  /**
   * Stop monitoring VPN connection
   */
  stopMonitoring(): void {
    this.isChecking = false;
    logger.info('Stopped VPN monitoring');
  }
}
