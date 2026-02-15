/**
 * Device Code Service
 * OAuth 2.0 Device Code Flow implementation (RFC 8628)
 */

import { randomBytes } from 'crypto';
import { createLogger } from '@nself/plugin-utils';
import type { AuthConfig } from './types.js';

const logger = createLogger('auth:device-code');

export interface DeviceCodeData {
  deviceCode: string;
  userCode: string;
  verificationUri: string;
  expiresAt: Date;
  interval: number;
}

export class DeviceCodeService {
  private config: AuthConfig;

  constructor(config: AuthConfig) {
    this.config = config;
  }

  /**
   * Generate user-friendly device code
   * Excludes confusing characters (0, O, 1, I, L)
   */
  generateUserCode(): string {
    const chars = 'ABCDEFGHJKLMNPQRSTUVWXYZ23456789'; // Exclude confusing chars
    const length = this.config.deviceCode.codeLength || 8;
    let code = '';

    for (let i = 0; i < length; i++) {
      const randomIndex = Math.floor(Math.random() * chars.length);
      code += chars[randomIndex];
    }

    // Format as XXXX-XXXX for readability
    if (length === 8) {
      return `${code.slice(0, 4)}-${code.slice(4)}`;
    }

    return code;
  }

  /**
   * Generate device code (internal tracking)
   */
  generateDeviceCode(): string {
    return randomBytes(32).toString('hex');
  }

  /**
   * Initiate device code flow
   */
  async initiate(clientId?: string, scope?: string): Promise<DeviceCodeData> {
    const deviceCode = this.generateDeviceCode();
    const userCode = this.generateUserCode();

    const expiresAt = new Date();
    expiresAt.setSeconds(expiresAt.getSeconds() + this.config.deviceCode.expirySeconds);

    const verificationUri = `${this.config.host}:${this.config.port}/auth/device`;

    logger.info('Device code initiated', { userCode, expiresAt });

    return {
      deviceCode,
      userCode,
      verificationUri,
      expiresAt,
      interval: this.config.deviceCode.pollInterval || 5,
    };
  }

  /**
   * Check if device code is expired
   */
  isExpired(expiresAt: Date): boolean {
    return new Date() > expiresAt;
  }
}
