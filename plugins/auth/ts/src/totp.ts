/**
 * TOTP 2FA Implementation
 * Using otplib for TOTP generation and verification
 */

import { TOTP, generateSecret, NobleCryptoPlugin, ScureBase32Plugin } from 'otplib';
import QRCode from 'qrcode';
import { createLogger } from '@nself/plugin-utils';
import type { AuthConfig } from './types.js';

const logger = createLogger('auth:totp');

export interface TotpEnrollmentData {
  secret: string;
  qrCode: string;
  backupCodes: string[];
}

export interface TotpVerifyResult {
  valid: boolean;
  usedBackupCode?: boolean;
}

export class TotpService {
  private config: AuthConfig;
  private totp: TOTP;

  constructor(config: AuthConfig) {
    this.config = config;

    // Configure TOTP instance with plugins
    this.totp = new TOTP({
      digits: config.totp.digits,
      period: config.totp.period,
      algorithm: config.totp.algorithm.toLowerCase() as 'sha1' | 'sha256' | 'sha512',
      crypto: new NobleCryptoPlugin(),
      base32: new ScureBase32Plugin(),
    });
  }

  /**
   * Generate new TOTP secret and QR code for enrollment
   */
  async generateSecret(userId: string, email?: string): Promise<TotpEnrollmentData> {
    // Generate random secret
    const secret = generateSecret();

    // Generate OTP Auth URL
    const issuer = this.config.totp.issuer || 'nself';
    const label = email || userId;
    const otpauth = `otpauth://totp/${encodeURIComponent(issuer)}:${encodeURIComponent(label)}?secret=${secret}&issuer=${encodeURIComponent(issuer)}&algorithm=${this.config.totp.algorithm.toUpperCase()}&digits=${this.config.totp.digits}&period=${this.config.totp.period}`;

    // Generate QR code
    const qrCode = await QRCode.toDataURL(otpauth);

    // Generate backup codes
    const backupCodes = this.generateBackupCodes();

    logger.info('Generated TOTP secret', { userId });

    return {
      secret,
      qrCode,
      backupCodes,
    };
  }

  /**
   * Verify TOTP code
   */
  async verifyToken(secret: string, token: string): Promise<boolean> {
    try {
      const result = await this.totp.verify(token, { secret });

      // otplib v13 returns VerifyResult object with { valid: boolean }
      const isValid = typeof result === 'object' && result !== null && 'valid' in result
        ? (result as { valid: boolean }).valid
        : Boolean(result);

      logger.debug('TOTP verification', { isValid });
      return isValid;
    } catch (error) {
      logger.error('TOTP verification error', { error });
      return false;
    }
  }

  /**
   * Verify backup code
   */
  verifyBackupCode(providedCode: string, storedCodes: string[]): { valid: boolean; remainingCodes: string[] } {
    const normalizedProvided = providedCode.trim().toLowerCase();

    const codeIndex = storedCodes.findIndex(
      code => code.trim().toLowerCase() === normalizedProvided
    );

    if (codeIndex === -1) {
      return { valid: false, remainingCodes: storedCodes };
    }

    // Remove used backup code
    const remainingCodes = storedCodes.filter((_, i) => i !== codeIndex);

    logger.info('Backup code used', { remainingCount: remainingCodes.length });

    return { valid: true, remainingCodes };
  }

  /**
   * Generate backup codes for account recovery
   */
  private generateBackupCodes(): string[] {
    const count = this.config.totp.backupCodeCount || 10;
    const codes: string[] = [];

    for (let i = 0; i < count; i++) {
      // Generate 8-character alphanumeric code
      const code = this.generateRandomCode(8);
      codes.push(code);
    }

    return codes;
  }

  /**
   * Generate random alphanumeric code
   */
  private generateRandomCode(length: number): string {
    const chars = 'ABCDEFGHJKLMNPQRSTUVWXYZ23456789'; // Exclude confusing chars (0,O,1,I)
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
   * Get current TOTP token (for testing purposes)
   */
  async getCurrentToken(secret: string): Promise<string> {
    return await this.totp.generate({ secret });
  }
}
