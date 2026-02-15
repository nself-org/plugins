/**
 * Magic Links Service
 * Secure token generation and validation for magic link authentication
 */

import { randomBytes, createHash } from 'crypto';
import { createLogger } from '@nself/plugin-utils';
import type { AuthConfig } from './types.js';

const logger = createLogger('auth:magic-links');

export interface MagicLinkData {
  token: string;
  url: string;
  expiresAt: Date;
}

export class MagicLinkService {
  private config: AuthConfig;

  constructor(config: AuthConfig) {
    this.config = config;
  }

  /**
   * Generate a magic link token and URL
   */
  async generateMagicLink(email: string, purpose: 'login' | 'verify' | 'reset'): Promise<MagicLinkData> {
    // Generate cryptographically secure random token (64 hex chars = 256 bits)
    const token = randomBytes(32).toString('hex');

    // Calculate expiration
    const expiresAt = new Date();
    expiresAt.setSeconds(expiresAt.getSeconds() + this.config.magicLink.expirySeconds);

    // Generate URL
    const baseUrl = this.config.magicLink.baseUrl;
    const url = `${baseUrl}/auth/magic-link/verify?token=${token}&purpose=${purpose}`;

    logger.info('Generated magic link', { email, purpose, expiresAt });

    return {
      token,
      url,
      expiresAt,
    };
  }

  /**
   * Hash token for storage (SHA-256)
   * We never store the raw token, only the hash
   */
  hashToken(token: string): string {
    return createHash('sha256').update(token).digest('hex');
  }

  /**
   * Verify token hasn't expired
   */
  isExpired(expiresAt: Date): boolean {
    return new Date() > expiresAt;
  }
}
